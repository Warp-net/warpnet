---
name: warpnet-testnet-verify
description: Use this skill whenever a Warpnet change needs to be verified by actually running it in the testnet network — any task phrased as "test this in testnet", "verify the feature works", "check it end-to-end", "does the handler actually respond", "prove the route works on a real node", "smoke-test before pushing", or when you've just added/changed a handler, DTO, or protocol path and want runtime confirmation rather than just `go test`. This skill MANDATES bringing up a real business node (`cmd/node/business`) on `--node.network=testnet` and driving it over its `/ws` bridge — that is the required verification vehicle, not an optional one. Do NOT use this skill to design a new handler (use `warpnet-add-handler`) or to diagnose a cross-layer bug whose symptom you already have (use `warpnet-debug-stack`). Use it to confirm a change is live and correct on an actual node.
---

# Verifying Warpnet changes in testnet via a business node

`go test` proves the pieces compile and the unit logic holds. It does **not** prove
that a route is registered, that the libp2p self-stream reaches the handler, that the
auth-signed envelope decodes, or that the wire payload is what the frontend will read.
The only artifact that exercises the whole request path in one process is the
**business node** (`cmd/node/business`): a single Go binary that serves the embedded
`frontend/dist` over HTTP and bridges the dashboard to the node's own handlers over a
WebSocket at `/ws`. Every non-auth call it receives is signed and routed through
`node.SelfStream(...)` — the exact code path a real client hits.

**This skill's rule: a change is not "verified in testnet" until a business node has
been built from your working tree, started on `--node.network=testnet`, logged in, and
answered the route you changed with the payload you expect.** Running the binary is
mandatory, not a nice-to-have. `go test` alone is never sufficient to close a
"verify in testnet" task.

Why the *business* node specifically (and not the member/desktop node):

- It's headless and driveable from a script — no Wails, no browser, no GUI event loop.
- Its `/ws` bridge speaks the same `event.Message` envelope the Vue frontend uses, so
  what you prove here is what the UI will get.
- One binary boots the whole stack: local BadgerDB, auth, the libp2p host, and every
  registered protocol handler. If your route is wired, this node answers it.

## The mandatory verification loop

```
1. build     go build -mod=vendor -o <scratch>/business ./cmd/node/business
2. fresh     rm -rf ~/.warpdata/testnet          # only for a clean first-run/register
3. run       <scratch>/business --node.network=testnet \
                 --node.server.password='TestPass123!' --node.server.port=4999 &
4. wait      curl -s -o /dev/null -w '%{http_code}' localhost:4999/healthz   # → 200
5. drive     ws://localhost:4999/ws :  is-first-run → login → <your route(s)>
6. assert    inspect the JSON body of the reply to your route
7. teardown  kill the node; rm -rf ~/.warpdata/testnet if you want a clean slate
```

Steps 1, 3, and 5 are non-negotiable for any "verify in testnet" task. Skipping to
`go test` and declaring success is the failure mode this skill exists to prevent.

## Step 1 — build from the working tree

```bash
SB=<your scratch dir>
go build -mod=vendor -o "$SB/business" ./cmd/node/business
```

Always `-mod=vendor` (the repo vendors everything; see `CLAUDE.md`). Build from the
branch you're verifying — a stale binary verifies nothing. The build needs the Go
toolchain pinned in `Dockerfile.business` (currently Go 1.26.x); the CI Docker image is
the source of truth for the version.

## Step 2/3 — run on testnet

```bash
"$SB/business" \
  --node.network=testnet \
  --node.server.password='TestPass123!' \
  --node.server.port=4999 \
  --logging.level=info > "$SB/business.log" 2>&1 &
```

Key flags (all from `config/config.go`, override via `--flag` or `NODE_*` env):

| flag | default | notes |
|------|---------|-------|
| `--node.network` | `warpnet` | **use `testnet`** — it auto-appends the testnet bootstrap peers and flips `IsTestnet()` |
| `--node.server.password` | *(empty → `log.Fatal`)* | required; also the AES key for `/ws` traffic. Must pass the password policy (below) |
| `--node.server.port` | `4999` | dashboard HTTP/WS port |
| `--node.port` | `4001` | libp2p listen port |
| `--node.bootstrap` | *(empty)* | comma-separated multiaddrs; testnet peers are added automatically on top |
| `--database.dir` | `storage` | DB lives at `~/.warpdata/<network>/<database.dir>` |
| `--logging.level` | `info` | `debug` shows every `protocol added: [...]` line — handy to confirm your route registered |

The node prints `NODE IS LISTENING ON 'localhost::4999'` immediately, but **the libp2p
node and its handlers do not exist until someone logs in** (see the flow below). Until
then any routed call returns `{"code":500,"message":"not attached server node"}`.

### Password policy (from `cmd/node/member/auth/auth.go:validatePassword`)

Login on a fresh DB **registers** the account, so the password must satisfy all of:

- 8–32 characters
- ≥1 uppercase, ≥1 lowercase, ≥1 digit, ≥1 special (`[\W_]`)

`TestPass123!` passes. A weaker password comes back as e.g.
`{"code":500,"message":"password must have at least one uppercase letter"}` — that error
is the node working correctly, not a bug.

The identity is **deterministic**: the ed25519 key derives from
`username + password + network` (`database/auth-repo.go`). Same creds + same network ⇒
same `node_id` every run. Change the password and you get a different account.

## Step 4 — liveness vs readiness

`/healthz` and `/readyz` both **always return 200** — they are process-liveness probes
(`cmd/node/business/handlers/auxiliary.go`), they do **not** gate on the node being
attached. Use them only to confirm the HTTP server is up. Real readiness = you logged in
and a routed call (e.g. admin stats) returned data. Don't treat `readyz=200` as "the
node is ready to serve routes".

## Step 5 — drive the `/ws` bridge

The envelope is `event.Message` (`event/event.go`):

```json
{"body": <raw json>, "message_id": "<any>", "path": "<destination>",
 "timestamp": "<rfc3339>", "version": "0.0.0", "signature": ""}
```

`path` is the destination route. The reply echoes `message_id` and `path` and carries the
result in `body`.

**Codec shortcut for testing:** the bridge's `AESCodec.Decode` tries to AES-GCM-decrypt
each frame and, *on failure, treats the frame as plaintext* and replies in plaintext
(`security/aes.go`). So a test client can send **plaintext JSON** and read plaintext
replies — no need to reimplement the AES layer the browser uses. (The browser encrypts
because it shares the password; your probe doesn't have to.)

Three destinations are handled specially by the bridge (`handlers/bridge.go::dispatch`);
everything else is signed and forwarded to `node.SelfStream`:

| `path` | meaning |
|--------|---------|
| `is-first-run` | returns `true` on an empty DB (no account yet) |
| `/private/post/login/0.0.0` | logs in / registers; **this is what boots and attaches the libp2p node** |
| `/private/post/logout/0.0.0` | closes the DB; the node keeps running |

The required order: **`is-first-run` → login → wait a few seconds for the node to attach
→ your route(s).** Login returns `domain.AuthNodeInfo` (`user_id`, `token`, `psk`,
`node_id`, `addresses`, `role":"business"`, `bootstrap_peers`, `network`). A successful
`node_id` means the host started.

### Reference probe client

Because the module vendors `gorilla/websocket`, the simplest reliable way to run a Go
probe is to drop it into a **throwaway package inside the repo** and delete it after
(never commit it):

```bash
mkdir -p cmd/wsprobe && cat > cmd/wsprobe/main.go <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Body        json.RawMessage `json:"body"`
	MessageId   string          `json:"message_id"`
	NodeId      string          `json:"node_id"`
	Destination string          `json:"path"`
	Timestamp   time.Time       `json:"timestamp"`
	Version     string          `json:"version"`
	Signature   string          `json:"signature"`
}

func main() {
	c, _, err := websocket.DefaultDialer.Dial("ws://localhost:4999/ws", nil)
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer c.Close()

	send := func(dest string, body any) {
		b, _ := json.Marshal(body)
		m := Msg{Body: b, MessageId: dest, Destination: dest, Timestamp: time.Now(), Version: "0.0.0"}
		out, _ := json.Marshal(m)
		_ = c.WriteMessage(websocket.TextMessage, out)
	}
	read := func(label string) {
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
		_, data, err := c.ReadMessage()
		if err != nil {
			fmt.Printf("[%s] read err: %v\n", label, err)
			return
		}
		fmt.Printf("[%s] %s\n", label, string(data))
	}

	send("is-first-run", nil)
	read("is-first-run")

	send("/private/post/login/0.0.0", map[string]string{"username": "demo", "password": "TestPass123!"})
	read("login")

	time.Sleep(4 * time.Second) // let the libp2p node attach

	// ---- edit below: drive the route(s) you changed ----
	send("/private/get/admin/stats/0.0.0", map[string]any{})
	read("stats")
}
EOF

go run -mod=vendor ./cmd/wsprobe
rm -rf cmd/wsprobe   # ALWAYS remove — never commit the probe
```

To verify *your* change, replace the `stats` block with your route's `path` and body.
Look at the reply's `body`:

- meaningful JSON matching your handler's return type ⇒ **verified**.
- `{"code":500,"message":"not attached server node"}` ⇒ login didn't succeed (bad
  password, or you called the route before login attached the node).
- `{"code":500,"message":"..."}` with a real message ⇒ the route ran and rejected the
  input — read the message; that's your handler talking.
- an empty/zero-value body where you expected data ⇒ likely a wire-contract issue;
  switch to `warpnet-debug-stack` (§ Silent zero-value DTO parsing).

## Step 7 — teardown & clean state

```bash
kill <node-pid>
rm -rf ~/.warpdata/testnet     # wipes the account + DB → next run is a fresh first-run
rm -rf cmd/wsprobe             # if you used the probe
```

Confirm `git status --porcelain` is clean before finishing — the probe package, any
built binary, and the `~/.warpdata` store must never end up in the commit. Nothing this
skill creates belongs in the repo.

## Gotchas

- **The node attaches only after login.** The main loop in `cmd/node/business/main.go`
  blocks on `readyChan` and constructs the libp2p node on the first successful login,
  then `AttachNode`s it. Calling a route before that ⇒ `not attached server node`.
- **`healthz`/`readyz` are liveness only** — both hard-coded to 200. Don't use them as a
  readiness gate; assert on an actual routed reply instead.
- **Isolated environments won't peer.** Behind an egress proxy the node completes
  `dht: bootstrap complete` but shows `network_state: Disconnected`, `peers_online: 0`.
  That's expected and does **not** invalidate handler verification — self-stream routes
  are answered locally by this node regardless of peer connectivity. Only cross-node
  features (fetching another user's remote data) need real peers.
- **Metrics gateway** defaults to a hardcoded push address and may be unreachable in a
  sandbox; it fails soft and does not block the node. Ignore metrics errors in the log.
- **Same creds ⇒ same node.** The deterministic key means re-running with the same
  username/password/network reuses the account. Wipe `~/.warpdata/<network>` for a true
  first-run/register test; keep it to test the returning-login path.
- **`is-first-run` flips lazily.** It's `db.IsFirstRun` queried per call; it reports
  `false` once the DB has been opened by a first login.

## Two-node local topology (when you need real peer exchange)

Some features (remote user fetch, follow across nodes, moderation handshake) only prove
out with a second node to talk to. Run a **member node** and point the business node at
it as a bootstrap peer, both on the same `--node.network=testnet` and the **same
`version` file** (the PSK derives from `network + version`, so a version mismatch =
different private network = they won't connect):

```bash
# member node on its own ports + db dir
"$SB/member" --node.network=testnet --node.port=4101 --node.server.port=4998 \
    --database.dir=storage-member --node.server.password='TestPass123!' &
# read its /ip4/.../p2p/<id> from the login response or logs, then:
"$SB/business" --node.network=testnet --node.port=4001 --node.server.port=4999 \
    --node.bootstrap='/ip4/127.0.0.1/tcp/4101/p2p/<member-node-id>' \
    --database.dir=storage-business --node.server.password='TestPass123!' &
```

Give each node a distinct `--node.port`, `--node.server.port`, and `--database.dir`.
Verify connectivity via the `peers_online` field in `/private/get/admin/stats/0.0.0`
before asserting on any cross-node route.

## When this skill does NOT apply

- **Designing/adding a new route** → `warpnet-add-handler` (come back here to verify it).
- **A known cross-layer bug with a symptom** → `warpnet-debug-stack`.
- **Pure unit logic** with no route/protocol surface → a plain `go test ./...` is enough;
  you don't need a node.
- **warpdroid/Vue UI-only changes** that don't cross the Warpnet protocol boundary → no
  node needed.
</content>
</invoke>
