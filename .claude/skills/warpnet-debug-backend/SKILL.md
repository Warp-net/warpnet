---
name: warpnet-debug-backend
description: Use this skill when a Warpnet bug lives in the Go node/backend — the fat/business/member node, its handlers, storage, or libp2p layer. Symptoms and triggers include "Transaction Conflict. Please retry" in node logs, the libp2p connection flapping every ~25-30s (yamux keep-alive), "context deadline exceeded" on a specific server RPC, a handler emitting the wrong or zero-value payload on the wire, gossip/timeline delivery failing (a followed user's tweets never arrive), CRDT stat double-counting, BadgerDB MVCC / scan-then-write conflicts, or standing up a real business node in Docker on testnet to reproduce a symptom against a live node. Also use to verify the wire contract from the server side (test/api_sync_test.go) — the handler's return statement is the ground truth for what a client will parse. Do NOT use this skill for pure client rendering/parsing/UI bugs (use warpnet-debug-frontend) or to add a new route or feature (use warpnet-add-handler).
---

# Debugging backend bugs in Warpnet (Go node)

This skill is for bugs whose cause lives in the Go node — the handlers, BadgerDB storage, the libp2p/yamux transport, or the gossip/CRDT layer — and for standing up your own node to reproduce a symptom. The visible breakage is often reported by a client (blank list, dropped connection, a slow RPC), but when the cause is server-side — the wire-contract DTO the handler actually emits, the yamux config, BadgerDB MVCC semantics — this is the skill.

If your task is "add a new feature", you want `warpnet-add-handler` instead. If the node is fine and the client parses/renders/behaves wrong, you want `warpnet-debug-frontend`.

## Triage tree

Start at the **symptom** and walk down. Each leaf names the section below that documents the failure mode and the fix pattern.

```
Symptom: "Transaction Conflict. Please retry" in the fat-node logs
                                                                          ─→ § BadgerDB scan-then-write conflict

Symptom: connection drops periodically, period matches ~30s (yamux keep-alive)
                                                                          ─→ § Yamux config for relay-tunneled traffic

Symptom: "context deadline exceeded" on a specific server RPC
                                                                          ─→ § BadgerDB scan-then-write conflict OR the backend handler

Symptom: client shows blank / zero-value data while the same node serves the
         Vue desktop correctly — the payload is wrong on the wire
                                                                          ─→ the handler's return statement is ground truth;
                                                                             verify the wire contract — § Wire contract not covered by tests
```

Frontend-only symptoms — blank rows/fields that are a client-parse issue, missing avatars, UI jank, battery drain, a stale committed `.aar`, or the dashboard behaving "logged out" after a restart — are not this skill: use `warpnet-debug-frontend`.

## § Yamux config for relay-tunneled traffic

**Symptom.** `network: event: peer ...UVdLFy connectedness updated: Limited → NotConnected → Limited` cycle every ~25-30 seconds in the fat-node logs. The cycle period matches yamux's `KeepAliveInterval`.

**Mechanism.** yamux ships with `KeepAliveInterval=30s` and `ConnectionWriteTimeout=10s`. When the connection is tunneled through a circuit-v2 relay (warpdroid → DigitalOcean relay → home router → desktop), the round-trip jitter for a keep-alive ping can spike above 10s under any congestion. When that happens, yamux concludes the peer is dead and tears the connection down. libp2p auto-reconnects via the same relay, the 30s idle starts again, and the cycle repeats.

**Where to look.**

- `warpdroid/node/node.go` libp2p options — the yamux muxer config.
- `core/node/options.go` on the fat-node side — same.
- The default `yamux.DefaultTransport.Config` only has `KeepAliveInterval=30s` / `ConnectionWriteTimeout=10s`.

**Fix pattern.** Build a custom yamux Config on both sides:

```go
ya := yamux.DefaultTransport
ya.KeepAliveInterval = 15 * time.Second        // ping more often than the cycle was
ya.ConnectionWriteTimeout = 30 * time.Second   // pong has slack to traverse relay
libp2p.Muxer(yamux.ID, ya)
```

Both sides must agree — yamux is symmetric, either party can tear down the connection.

**Anti-pattern.** Do not "fix" this by *disabling* keep-alive. Without it, a broken connection isn't detected until the next user-initiated request, which then hangs for the full stream-open timeout (~15s) before failing. Keep keep-alive on; just give pong room to traverse the relay.

## § BadgerDB scan-then-write conflict

**Symptom.** Backend logs show repeated `middleware: handling of ... failed: Transaction Conflict. Please retry` for write routes that update one record in a per-user prefix (mark-read, follow-update, similar).

**Mechanism.** Badger's SSI tracks every key the txn *reads*. If a writer scans a prefix list (~100 sibling keys) inside the same RW txn that later writes to one key, every concurrent writer that touches *any* key in that prefix is now a conflict candidate. Two concurrent mark-reads on *different* notifications both read the same prefix and write different keys — they commit-conflict on the second commit.

**Where to look.**

- `database/<feature>-repo.go` for any method matching this pattern:

```go
func (repo *FooRepo) UpdateOne(userId, fooId string) error {
    txn, _ := repo.db.NewTxn()
    defer txn.Rollback()
    for {
        items, _, _ := txn.List(prefix, ...)       // ← reads many keys
        for _, item := range items {
            if matches(item, fooId) {
                txn.SetWithTTL(item.Key, ...)      // ← writes one key
                return txn.Commit()
            }
        }
    }
}
```

**Fix pattern.** Split into two transactions:

1. **Find-key in a discardable RW txn.** `Rollback()` it. Dropping the txn drops every key from Badger's conflict table for this caller.
2. **Targeted write in a fresh RW txn.** Read just `{targetKey}` via `txn.Get`, modify, `txn.SetWithTTL`, `txn.Commit`. The read-set and write-set are both `{targetKey}` — disjoint from concurrent writers on other keys.

Concurrent writers on the **same** key still legitimately conflict — but if the update is monotonic (e.g. setting `IsRead=true` for a notification), the loser's view-after-commit matches the winner's, so the retry just observes the already-updated record.

**Real example.** `database/notification-repo.go::MarkRead` was the canonical victim. The fix is `findNotificationKey` (scan in a separate txn) → small write txn.

**Anti-pattern.** Wrapping the broken method in a `for attempt := 0; attempt < N; attempt++ {...}` retry loop. This *hides* the contention; under real concurrency it just shifts the failure to the last retry. Fix the read-set, don't wallpaper over it.

## § Wire contract not covered by tests

**Symptom.** A wire-format bug (silent zero-value parsing, see above) reaches a user-facing surface. `test/api_sync_test.go` was green.

**Mechanism.** The original `TestAPISync_Payloads` only diffs **request** bodies: it checks that the client sends keys the backend's input struct accepts. The mirror case — client *reads* keys the backend never emits — was uncovered. Most production wire bugs in this repo fall in the mirror direction.

**Fix pattern.** `TestAPISync_ResponsePayloads` (already in `test/api_sync_test.go`) walks each routed handler's body, picks out the success-path return type, resolves it via the same alias chain, and asserts the warpdroid parse-DTO's keys are a subset. Smoke-test it by temporarily reintroducing a known bug:

```bash
# Reintroduce the WarpnetNotification bug
sed -i 's|@Json(name = "user_id") val userId: String = "",|@Json(name = "from_user_id") val fromUserId: String = "",\n    @Json(name = "tweet_id") val tweetId: String? = null,|' \
    warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt

go test ./test/ -run TestAPISync_ResponsePayloads -count=1
# Should fail with: "PRIVATE_GET_NOTIFICATION: warpdroid reads keys the backend doesn't emit: [from_user_id tweet_id]"

git checkout warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt
```

**When adding a new route**, this test is the safety net — if you add a new DTO with phantom fields, the test catches it without needing a manual device test.

## Running your own fat node in Docker (testnet)

Most of the dances above start with *"log into the fat node first."* When you don't have
the user's desktop node in front of you, stand up your own headless **business node**
(`cmd/node/business` — the same binary `warpnet-testnet-verify` uses) in a Docker
container on `testnet`. It boots the whole stack (BadgerDB, auth, libp2p host, every
handler) and serves the dashboard `/ws` bridge on a port you can drive or open in a
browser. This gives you a live node to reproduce a symptom against without touching the
user's machine.

> Sandbox note: outbound egress here is limited to TCP 80/443, so a lone node stays
> `network_state: Disconnected` / `peers_online: 0` — it can't reach the real testnet
> bootstrap peers on `:4011/:4022/:4033`. That does **not** block single-node handler
> debugging: every `self-stream` route is answered locally regardless of peers. For
> cross-node reproduction use the local swarm in `warpnet-testnet-verify`.

**Rule: always debug on your own node — the `Claude` account on the `NODE_SEED=claude`
node — never on the user's node or a throwaway account.** The point is a stable, known
fixture: same `node_id`, same `Claude` user, same avatar, same data every session, so
symptoms are reproducible and you never mutate the user's real state while poking at a
bug. To make that fixture survive container restarts and image rebuilds, the node **must**
mount its **own dedicated Docker volume** for `/root/.warpdata` (the BadgerDB store where
the account, profile, avatar, tweets, and node identity live). Without a volume the DB
lives in the container's writable layer and is lost on `docker rm` — you'd re-register and
lose the avatar every rebuild. The identity is deterministic (`username+password+network`
→ same key) so a wipe still yields the same `node_id`, but the profile/avatar and any
seeded content only persist on the volume. One volume, one account, reused everywhere.

To exercise the UI against this node (notifications triage, the base UI test plan, interacting as a user), see the `warpnet-debug-frontend` skill.

### 1. Build the image from the working tree

The build uses `Dockerfile.business` (Go toolchain from `go.dev` over 443, vendored
modules, embeds `frontend/dist`). Run from the repo root:

```bash
docker build -f Dockerfile.business -t warpnet-business:claude .
```

### 2. Run the container on testnet

`deploy/docker-compose-testnet.yml` is the reference for the env vars each node takes.
The `NODE_SEED` env fixes the node's deterministic libp2p ID (`config.go`:
`node.seed` ← `NODE_SEED`); `NODE_SERVER_PASSWORD` is the dashboard `/ws` AES secret
(and is **required** — an empty one is `log.Fatal`). Use `NODE_SEED=claude`, and mount the
node's **own** named volume at `/root/.warpdata` so the `Claude` account persists:

```bash
docker volume create warpnet-claude-testnet-data   # idempotent; the node's dedicated store

docker run -d --name warpnet-claude-testnet \
  -e NODE_NETWORK=testnet \
  -e NODE_SEED=claude \
  -e NODE_SERVER_PASSWORD='Claude1234$' \
  -e NODE_SERVER_PORT=4999 \
  -e NODE_PORT=4001 \
  -e LOGGING_LEVEL=info \
  -e LOGGING_FORMAT=json \
  -v warpnet-claude-testnet-data:/root/.warpdata \
  -p 4999:4999 \
  warpnet-business:claude

# wait for the HTTP server, then confirm liveness (always 200 — see below)
until curl -sf -o /dev/null localhost:4999/healthz; do sleep 1; done
```

Because the volume is dedicated and named, you can `docker rm` / rebuild the image and
`docker run` again with the same `-v warpnet-claude-testnet-data:/root/.warpdata` — the
`Claude` account, avatar, and any seeded data come straight back. Register the account (§3)
**once**; every later session reuses it.

Or, to keep it alongside the other testnet services, add a service to
`deploy/docker-compose-testnet.yml` (build locally rather than pulling a ghcr image):

```yaml
  claude-testnet:
    container_name: claude-testnet
    build:
      context: ..
      dockerfile: Dockerfile.business
    network_mode: host
    restart: always
    environment:
      - NODE_PORT=4077
      - NODE_NETWORK=testnet
      - NODE_SEED=claude
      - NODE_SERVER_PORT=4999
      - NODE_SERVER_PASSWORD=Claude1234$
      - LOGGING_LEVEL=info
      - LOGGING_FORMAT=json
    volumes:
      - warpnet-claude-testnet-data:/root/.warpdata   # dedicated, persists the Claude account

# and at the file's top level:
volumes:
  warpnet-claude-testnet-data:
```

**Liveness ≠ readiness.** `/healthz` and `/readyz` are hard-coded to 200; the libp2p node
and its handlers don't exist until someone logs in over `/ws`. Any routed call before
login returns `{"code":500,"message":"not attached server node"}`.

### 3. Register the `Claude` account and set the avatar

The account identity is deterministic from `username + password + network`. Log in over
`/ws` to register it (first login on a fresh DB creates it):

- **username:** `Claude`
- **password:** `Claude1234$` (public; passes the policy — upper/lower/digit/special, 8–32 chars)

The avatar is a two-step flow that mirrors the Vue client
(`frontend/src/service/service.js`: `uploadImages` → `editMyProfile`):

1. `POST /private/post/image/0.0.0` with `{image1:"data:image/png;base64,<logo>", image2..4:""}`
   → returns `{key1,...}` (the handler re-encodes to JPEG and stores it; `core/handler/media.go`).
2. `POST /private/post/user/0.0.0` with `{username:"Claude", avatar_key:"<key1>"}` — sets
   `domain.User.AvatarKey` on the owner profile.

Use the repo's own mark, `cmd/node/member/icon.png`, as the logo (or any PNG/JPG).

The `/ws` bridge accepts **plaintext** JSON frames (`AESCodec.Decode` falls back to
plaintext on decrypt failure — no need to reimplement the browser's AES layer). Drive it
with a throwaway Go probe using the vendored `gorilla/websocket` (delete it after — never
commit it):

```bash
mkdir -p cmd/wsprobe && cat > cmd/wsprobe/main.go <<'EOF'
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Body        json.RawMessage `json:"body"`
	MessageId   string          `json:"message_id"`
	Destination string          `json:"path"`
	Timestamp   time.Time       `json:"timestamp"`
	Version     string          `json:"version"`
	Signature   string          `json:"signature"`
}

func main() {
	logoPath := "cmd/node/member/icon.png"
	raw, err := os.ReadFile(logoPath)
	if err != nil {
		fmt.Println("read logo:", err)
		return
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)

	c, _, err := websocket.DefaultDialer.Dial("ws://localhost:4999/ws", nil)
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer c.Close()

	call := func(dest string, body any) json.RawMessage {
		b, _ := json.Marshal(body)
		out, _ := json.Marshal(Msg{Body: b, MessageId: dest, Destination: dest, Timestamp: time.Now(), Version: "0.0.0"})
		_ = c.WriteMessage(websocket.TextMessage, out)
		_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, data, err := c.ReadMessage()
		if err != nil {
			fmt.Printf("[%s] read err: %v\n", dest, err)
			return nil
		}
		fmt.Printf("[%s] %s\n", dest, string(data))
		var m Msg
		_ = json.Unmarshal(data, &m)
		return m.Body
	}

	call("is-first-run", nil)
	call("/private/post/login/0.0.0", map[string]string{"username": "Claude", "password": "Claude1234$"})
	time.Sleep(4 * time.Second) // let the libp2p node attach

	imgResp := call("/private/post/image/0.0.0", map[string]string{"image1": dataURL})
	var img struct {
		Key1 string `json:"key1"`
	}
	_ = json.Unmarshal(imgResp, &img)
	if img.Key1 == "" {
		fmt.Println("no avatar key returned")
		return
	}
	call("/private/post/user/0.0.0", map[string]string{"username": "Claude", "avatar_key": img.Key1})
}
EOF

go run -mod=vendor ./cmd/wsprobe
rm -rf cmd/wsprobe   # ALWAYS remove — never commit the probe
```

A non-empty `key1` in the image reply and a returned user with `avatar_key` set means the
avatar is live. Open `http://localhost:4999` in the session browser and log in as
`Claude` / `Claude1234$` to see it.

### 4. Teardown

```bash
docker rm -f warpnet-claude-testnet          # stop/remove the container...
# ...but KEEP the volume — it holds the Claude account + avatar for next session:
docker volume ls | grep warpnet-claude-testnet-data

# Only if you deliberately want a clean first-run/register again:
# docker volume rm warpnet-claude-testnet-data
```

Remove the **container** freely; keep the **volume**. Re-running the `docker run` in §2 with
the same `-v warpnet-claude-testnet-data:/root/.warpdata` brings the account straight back —
no re-register, avatar intact. Wipe the volume only when you explicitly want a fresh
first-run.

Confirm `git status --porcelain` is clean — the probe package must never land in a commit.

## Cheat sheet for the most common backend debugging dance

1. **Tail the fat-node logs.** If a write fails on the server, it logs `middleware: handling of <path> ... failed: <reason>`. That line present = server-side bug (you're in the right skill; read the handler). Absent, with the client still showing broken data = client-side parse / contract bug (that's `warpnet-debug-frontend`).
2. **Run `go test ./test/ -count=1`.** Catches wire-contract drift without needing a device.
3. **Look at the actual JSON bytes.** Add a `log.Infof("DEBUG response: %s", string(b))` near the handler return; tail the logs. This dispels 90% of "the DTO must be wrong" guesses inside a minute — the handler's return statement is the ground truth of what's on the wire.
4. **Don't fix contention with retries.** Retry loops around a conflicting write, or "increase the timeout", hide rather than fix. Find the actual contention and fix the read-set.
5. **BadgerDB read-set discipline.** Never scan a prefix in the same RW txn that writes one key; split find-key (discardable txn) from the targeted write so the read-set and write-set are disjoint.

## When this skill doesn't apply

- **Client rendering / parsing / UI bugs** (blank rows that are a client-parse issue, avatars, jank, battery, stale `.aar`, dashboard "logged out" after a restart) → use `warpnet-debug-frontend`.
- **Adding a new route or feature** → use `warpnet-add-handler`.
