---
name: warpnet-claude-node
description: Use this skill to stand up, log into, or tear down your own Warpnet node — the `Claude` account on the `NODE_SEED=claude` node, running from `Dockerfile.remote` on testnet. Triggers include "spin up a node", "start your own node", "log into the node", "I need a live node to reproduce this", "rebuild the container", "the node is gone / recreate it", "set up the Claude account", "the dashboard says logged out after a restart", "reset the node's data", or any task that begins with "log into the fat node first" and you don't have the user's desktop node in front of you. This skill owns the node fixture's lifecycle — image build, the dedicated Docker volume, account registration, the avatar, the `/ws` bridge, browser login, and teardown. Do NOT use this skill to diagnose a Go-side bug (use `warpnet-debug-backend`), a client parse/render bug (use `warpnet-debug-frontend`), to add a route (use `warpnet-add-handler`), or to verify a change on a throwaway local binary node (use `warpnet-testnet-verify`).
---

# Running your own Claude node (Docker, testnet)

Most debugging dances start with *"log into the fat node first."* When you don't have the
user's desktop node in front of you, stand up your own headless **remote node**
(`cmd/node/member/remote-member.go` — the same binary `warpnet-testnet-verify` uses) in a Docker
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

## 1. Build the image from the working tree

The build uses `Dockerfile.remote` (Go toolchain from `go.dev` over 443, vendored
modules, embeds `frontend/dist`). It builds `cmd/node/member/remote-member.go` with
`-tags remote` — without that tag you get the desktop member node instead. Run from
the repo root:

```bash
docker build -f Dockerfile.remote -t warpnet-remote:claude .
```

## 2. Run the container on testnet

`deploy/docker-compose-testnet.yml` is the reference for the env vars each node takes.
The `NODE_SEED` env fixes the node's deterministic libp2p ID (`config.go`:
`node.seed` ← `NODE_SEED`). The `/ws` channel needs no secret — the node's Noise NX
static key (`ws-noise.key`) lives in the data volume. Use `NODE_SEED=claude`, and mount the
node's **own** named volume at `/root/.warpdata` so the `Claude` account persists:

```bash
docker volume create warpnet-claude-testnet-data   # idempotent; the node's dedicated store

docker run -d --name warpnet-claude-testnet \
  -e NODE_NETWORK=testnet \
  -e NODE_SEED=claude \
  -e NODE_SERVER_PORT=4999 \
  -e NODE_PORT=4001 \
  -e LOGGING_LEVEL=info \
  -e LOGGING_FORMAT=json \
  -v warpnet-claude-testnet-data:/root/.warpdata \
  -p 4999:4999 \
  warpnet-remote:claude

# wait for the HTTP server to start serving the dashboard (see the caveat below)
until curl -sf -o /dev/null localhost:4999/; do sleep 1; done
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
      dockerfile: Dockerfile.remote
    network_mode: host
    restart: always
    environment:
      - NODE_PORT=4077
      - NODE_NETWORK=testnet
      - NODE_SEED=claude
      - NODE_SERVER_PORT=4999
      - LOGGING_LEVEL=info
      - LOGGING_FORMAT=json
    volumes:
      - warpnet-claude-testnet-data:/root/.warpdata   # dedicated, persists the Claude account

# and at the file's top level:
volumes:
  warpnet-claude-testnet-data:
```

**Liveness ≠ readiness.** The remote node registers only `/ws` and `/` — `/healthz` and
`/readyz` were dropped along with the business node, and the SPA fallback answers any
other path with `index.html` and a 200. So a 200 means the HTTP server is up, nothing
more: the libp2p node and its handlers don't exist until someone logs in over `/ws`. Any
routed call before login returns `{"code":500,"message":"not attached server node"}`.

## 3. Register the `Claude` account and set the avatar

The account identity is deterministic from `username + password + network`. Log in over
`/ws` to register it (first login on a fresh DB creates it):

- **username:** `Claude`
- **password:** `Claude1234$` (public; passes the policy — upper/lower/digit/special, 8–32 chars)

The avatar is a two-step flow that mirrors the Vue client
(`frontend/src/service/service.js`: `uploadImages` → `editMyProfile`):

1. `POST /private/post/image/0.0.0` with `{image1:"data:image/png;base64,<logo>", image2..4:""}`
   → returns `{key1,...}` (the handler re-encodes to JPEG and stores it; `core/handler/image.go`).
2. `POST /private/post/user/0.0.0` with `{username:"Claude", avatar_key:"<key1>"}` — sets
   `domain.User.AvatarKey` on the owner profile.

Use the repo's own mark, `cmd/node/member/icon.png`, as the logo (or any PNG/JPG).

The `/ws` bridge speaks Noise NX with **no plaintext fallback**. A Go probe secures the
connection with `security.NoiseHandshakeInitiator(read, write)` right after dialing
(read/write adapters over gorilla's `ReadMessage`/`WriteMessage`, binary frames), then
`session.Encrypt`/`session.Decrypt` each JSON frame. Drive it with a throwaway Go probe
using the vendored `gorilla/websocket` (delete it after — never commit it):

```bash
mkdir -p cmd/wsprobe && cat > cmd/wsprobe/main.go <<'EOF'
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Warp-net/warpnet/security"
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

## 4. Open the dashboard — a fresh tab after every restart

**ALWAYS open a fresh browser tab after every container rebuild / restart / recreate —
never reuse the same tab across a node restart.** The Vue frontend's transport is a
module-level singleton (`socket`, the Noise `session`, a `pending` map of per-request promises +
timers) with auto-reconnect (`frontend/src/lib/transport.js`). Restart the node under a
long-lived tab a few times and that singleton wedges: a half-dead WebSocket plus pending
promises that never resolve. The tell is nasty and easy to misdiagnose — `is-first-run`
still works, but **login hangs before it ever transmits
the frame** (hook `WebSocket.prototype.send` and you'll see *zero* frames), with **no
console error and no `authenticating user` line in the node logs**. Do **not** conclude
"the browser login / `/ws` channel is broken" — it isn't: a Go probe using
`security.NoiseHandshakeInitiator` authenticates instantly, proving the node is healthy. The fix is simply a new tab / fresh
browser context, which resets the singleton. Reopen the tab whenever the node behaves as
"logged out" or calls return empty/`Anonymous` after a restart.

## 5. Teardown

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

## Once the node is up

This skill ends where the debugging begins. With a live node in front of you:

- **Go-side symptoms** (Transaction Conflict, connection flapping, a handler emitting the
  wrong payload, the wire contract) → `warpnet-debug-backend`.
- **Driving the UI against this node** (screenshot verification, notification triage,
  interacting as a real user, the base UI test plan) and client parse/render bugs →
  `warpnet-debug-frontend`.
- **Confirming a new/changed route is live**, or a multi-node swarm on a throwaway local
  binary node rather than this persistent fixture → `warpnet-testnet-verify`.
