# WarpNet Contributor Onboarding

> A complete, end-to-end guide for new contributors: the *why*, the *how it
> works*, the *how to run it*, and the *how to change it* — across the whole
> WarpNet ecosystem.

This document is longer than a typical README on purpose. It is meant to take
you from "I just cloned the repo" to "I understand the architecture and can ship
a feature" without having to reverse-engineer the codebase first. Skim the table
of contents and jump to what you need.

For the short version, see [`README.md`](../README.md) and
[`HOW-TO-HELP.md`](../HOW-TO-HELP.md). This guide goes deeper.

---

## Table of contents

1. [Philosophy: why WarpNet exists](#1-philosophy-why-warpnet-exists)
2. [The ecosystem at a glance](#2-the-ecosystem-at-a-glance)
3. [Node roles](#3-node-roles)
4. [Repository layout](#4-repository-layout)
5. [Using WarpNet (as a user)](#5-using-warpnet-as-a-user)
6. [Building from source](#6-building-from-source)
7. [Running locally](#7-running-locally)
8. [Configuration reference](#8-configuration-reference)
9. [How it works under the hood](#9-how-it-works-under-the-hood)
10. [Making changes: a worked example](#10-making-changes-a-worked-example)
11. [The sister repositories](#11-the-sister-repositories)
12. [Testing, versioning & git workflow](#12-testing-versioning--git-workflow)
13. [Troubleshooting & gotchas](#13-troubleshooting--gotchas)
14. [Community & getting help](#14-community--getting-help)

---

## 1. Philosophy: why WarpNet exists

Every "decentralized" social network still tends to keep a soft center:

- **Federated networks** (Mastodon) still put you on an *instance*. The admin
  holds your data, sets the rules, can defederate, and can switch the server
  off. You trade one landlord for thousands of smaller ones.
- **Relay-based networks** (Nostr) make the client thin and push storage onto
  *relays* — stateful servers that can rate-limit you, refuse your events, or
  delete your data.
- **"Protocol" networks** (atproto/Bluesky) decentralize in theory but
  concentrate around a few large providers in practice.

**WarpNet removes the server tier entirely.** Every user runs a node; the node
*is* their participation in the network. There is no instance to seize, no admin
to trust, and no relay that can quietly drop you.

| | **WarpNet** | Mastodon | Nostr | Bluesky / atproto |
|---|:---:|:---:|:---:|:---:|
| Architecture | True P2P (libp2p) | Federated instances | Client + relay | PDS + relays |
| Server required? | **None** | Yes (instance) | Yes (relays) | Yes (providers) |
| Where your data lives | **Your own node** | Admin's DB | Relays you post to | Your PDS provider |
| Transport security | **Noise protocol** | TLS to instance | TLS to relay | TLS to provider |
| Can an admin deplatform you? | **No admin exists** | Yes | Relay can drop you | Provider can drop you |
| Discovery | **DHT** | Instance + relays | Relay lists | Relays / indexers |
| Moderation | LLM moderator nodes | Per-instance | Per-client | Labelers |

**The core bet:** the only way to be genuinely censorship-resistant is to remove
the server, not to multiply it. Discovery happens over a Kademlia **DHT**,
transport is encrypted with the **Noise** protocol, content propagates **peer to
peer**, and your data lives in an embedded datastore on *your* machine. There is
nothing in the middle to capture, subpoena, or switch off.

**Why AGPLv3?** A censorship-resistant network must stay free and open: anyone
who runs a modified version that others interact with has to share their
changes. WarpNet is AGPLv3, and it is not a startup — there is no monetization
and no owner. It is a protocol for people who want to own their tools.

---

## 2. The ecosystem at a glance

WarpNet is not one repository — it is a small constellation. The main node lives
in `warpnet`; the others are focused components that plug into or extend it.

| Repository | Language | What it is | Relationship to the node |
|---|---|---|---|
| **`warp-net/warpnet`** | Go + Vue + Kotlin | The main project: node engine, desktop UI, Android client | This is the core. Everything else orbits it. |
| **`warp-net/moderation`** | Go + C/C++ (CGO) | LLM content-moderation engine (Llama Guard 3 via llama.cpp) | Imported by the **moderator** node (`-tags=llama`). |
| **`warp-net/activity-pub-gw`** | Go | Stateless ActivityPub gateway to the Fediverse/Mastodon | Joins the network as a libp2p peer; bridges to Mastodon. |
| **`warp-net/libp2p-camouflage-transport`** | Go | libp2p transport that disguises traffic to defeat DPI censorship | A direct dependency of `warpnet`; wired into the Android client. |
| **`filinvadim/blurry`** | Go | Standalone libp2p + CRDT distributed key-value store | Sibling/research project; not a runtime dependency of the node. |

The four sister repos are covered in detail in
[§11 The sister repositories](#11-the-sister-repositories).

### One protocol, two clients

This is the most important architectural idea in the whole project:

```
                    ┌─────────────────────────────┐
                    │   desktop "fat" member node  │
                    │   cmd/node/member/...        │
                    │   core/handler/<feature>.go  │  ← ONE canonical handler per route
                    │   database/<feature>-repo.go │
                    └──────────────▲───────────────┘
                                   │
                       ┌───────────┴────────────┐
                       │                         │
            in-process Wails Call      libp2p stream over Noise
                       │                         │
              Vue desktop app           Android client (Tusky fork)
              frontend/                 warpdroid/
```

Both the desktop UI and the Android app talk to a **member node**, and both hit
the *same* canonical Go handler for a given route. They differ only in *how* the
request reaches the node:

- **Desktop:** the Vue app runs inside the node binary (Wails). It calls the Go
  `App.Call(...)` method directly, in-process.
- **Android:** the app opens a **libp2p stream** to the node over Noise, using
  the same route string.

The shared contract is a set of route strings in
[`event/paths.go`](../event/paths.go). Change a route, and every client that
uses it has to move in lockstep.

---

## 3. Node roles

A WarpNet binary can play one of several roles. The three first-class roles
described in the README are **relay**, **member**, and **moderator**; there are
two auxiliary roles (**business** and **echo**) used for bridging and testing.

| Role | Entry point | What it does | CGO / build tag |
|---|---|---|---|
| **member** | `cmd/node/member/main.go` | The full "fat" node most people run. Holds local data, serves the desktop UI, pairs with mobile devices, participates in the P2P network. | CGO on, `-tags webkit2_41` (Wails) |
| **relay** | `cmd/node/relay/main.go` | Stable, stateless entry points. Help new nodes find peers via the DHT and provide NAT-traversal relaying. (This is what older docs called the "bootstrap node" — there is no separate `cmd/node/bootstrap`.) | CGO off, pure Go |
| **moderator** | `cmd/node/moderator/main.go` | Runs an on-device LLM (Llama Guard 3) to evaluate reported content. Decentralized: each network can run its own. | CGO on, `-tags=llama` |
| **business** | `cmd/node/business/main.go` | A headless node exposing an authenticated HTTP/WebSocket dashboard (for external integrations and bridges). | CGO on |
| **echo** | `cmd/node/member/echo-member.go` | A headless "bot" member node with an in-memory store. Used to populate a local network and for testing. | CGO off, `-tags echo` |

> **Mental model:** *relays* are thin signposts, *members* are the network
> itself, *moderators* are optional opt-in referees, and *business/echo* are
> support roles. Only the **member** node has a GUI.

---

## 4. Repository layout

A map of the main `warpnet` repo. Start here and you won't get lost.

```
warpnet/
├── cmd/node/             # entry points, one dir per role
│   ├── member/           #   the fat node + desktop app (main.go, app.go, echo-member.go)
│   │   ├── auth/         #   login / identity / keypair derivation
│   │   ├── deeplink/     #   warpnet:// URL scheme handling (per-OS)
│   │   └── node/         #   member-node.go — wires the whole node together
│   ├── relay/            #   relay node
│   ├── moderator/        #   LLM moderator node
│   └── business/         #   dashboard / bridge node
├── core/                 # the heart — networking, routing, handlers
│   ├── node/             #   WarpNode: libp2p host, stream dispatch, middleware glue
│   ├── stream/           #   WarpRoute type, peer streaming, in-process loopback
│   ├── handler/          #   ONE file per route (start with like.go — cleanest example)
│   ├── middleware/       #   auth (signature verify), logging, unwrap, idempotency
│   ├── discovery/        #   multi-source peer discovery (DHT, mDNS, gossip, stream)
│   ├── dht/ mdns/        #   Kademlia DHT and LAN multicast discovery
│   ├── pubsub/           #   GossipSub topics (discovery, notifications)
│   ├── relay/            #   CircuitRelay v2 (NAT traversal)
│   ├── crdt/             #   conflict-free counters for stats (likes/retweets/…)
│   └── warpnet/          #   libp2p type aliases & protocol constants
├── database/             # repository layer over the embedded store
│   └── local-store/      #   BadgerDB wrapper + prefixed-key builder
├── domain/               # shared domain types (User, Tweet, Notification, IDs)
├── event/                # the wire protocol: paths.go (routes) + event.go (DTOs)
├── security/             # keys, signing, Noise/PSK, codebase-hash integrity
├── config/               # node configuration (flags / env / defaults)
├── frontend/             # Vue 3 desktop UI (talks to the node via Wails)
├── warpdroid/            # Android client (Tusky fork) + gomobile AAR bridge
├── warpfone/             # iOS client (placeholder — not implemented yet)
├── metrics/              # Prometheus push-metrics client
├── deploy/               # docker-compose stacks + deploy scripts
├── snap/                 # Snap packaging (snapcraft.yaml)
├── embedded.go           # go:embed of frontend/dist + the Go source (for hashing)
├── version               # patch-bumped on every commit
└── Makefile              # handy dev commands
```

If you want to trace a feature end to end, read it in this order:

1. **`event/paths.go`** — the route string.
2. **`core/handler/like.go`** — the handler.
3. **`database/like-repo.go`** — the storage.

`like` is intentionally the cleanest small reference in the codebase, and
`core/handler/like_test.go` is a complete worked test.

---

## 5. Using WarpNet (as a user)

Before contributing, it helps to *use* the thing.

### Install (no compiler needed)

```bash
sudo snap install warpnet
warpnet
```

[Snap Store →](https://snapcraft.io/warpnet)

### First run & identity

On first launch you create a **local identity**: a keypair derived on your
device from your credentials. The private key never leaves your machine. You are
now a node — there is no account on any server to register.

### Networks

WarpNet runs separate, isolated networks. **The default network is `warpnet`
(mainnet).** Use the testnet to experiment safely:

```bash
warpnet --node.network testnet   # safe playground
warpnet --node.network mainnet   # the live network (alias of "warpnet")
```

Nodes only talk to other nodes on the **same network *and* compatible version** —
the network membership key (PSK) is derived from both (see
[§9](#9-how-it-works-under-the-hood)).

### What you can do

The product is a Twitter-style social network: profiles, posts ("tweets"),
replies, likes, retweets, bookmarks, follows (including follow
requests/approval for private accounts), direct chat/messages, notifications,
media/images, search, mutes/blocks, content filters, and importing a Twitter
archive. Public content propagates peer-to-peer; private data (your timeline,
chats, drafts) stays on your node.

---

## 6. Building from source

### Prerequisites

- [Go 1.26+](https://go.dev/doc/install) (the module targets `go 1.26.3`)
- [Wails v2.10.2](https://github.com/wailsapp/wails) — for the desktop client
- Linux GUI/build dependencies (for the member node):

```bash
sudo apt update
sudo apt install -y pkg-config build-essential
sudo apt install -y \
  libgtk-3-dev \
  libwebkit2gtk-4.1-dev \
  libglib2.0-dev \
  libcairo2-dev \
  libpango1.0-dev \
  libgdk-pixbuf-2.0-dev \
  libatk1.0-dev \
  libsoup-3.0-dev
git submodule update --init --recursive
```

Install Wails:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

### Build & run the member node (desktop UI)

```bash
cd cmd/node/member
wails build -devtools -tags webkit2_41          # compile the binary
./build/bin/warpnet --node.network testnet      # run it on the testnet
```

The Makefile shortcut `make run-main` does the equivalent (with
`-m -nosyncgomod` for a vendored, reproducible build).

### Build & run the other roles

```bash
# Relay (pure Go, no GUI). --node.print-psk prints the network key on startup.
go run cmd/node/relay/main.go --node.network testnet --node.print-psk

# Moderator (LLM). Needs the GGUF model present (see --node.moderator.modelpath)
# and a C/C++ toolchain. The CXXFLAGS quiet llama.cpp build warnings.
CGO_CXXFLAGS="-w -Wno-format -Wno-delete-incomplete" \
  go run -tags=llama cmd/node/moderator/main.go \
  --node.network testnet --node.port 4002 --node.seed moderatorlocalhost

# Business (headless dashboard node)
go run cmd/node/business/main.go \
  --node.network testnet --node.server.port 4999 --node.server.password 'choose-a-secret'

# Echo (headless bot member, in-memory store — great for testing)
go run -tags echo cmd/node/member/echo-member.go \
  --node.network testnet --node.seed echo --database.dir echo --node.port 4002
```

### Build tags & CGO, explained

| Tag | Used by | Why |
|---|---|---|
| `webkit2_41` | member (desktop) | Selects the WebKit2GTK 4.1 bindings Wails uses to render the UI. |
| `llama` | moderator | Compiles in the `moderation` engine and its llama.cpp CGO bindings. |
| `echo` | echo bot | Swaps in the in-memory store and bot behavior. |

The **member**, **moderator**, and **business** roles need `CGO_ENABLED=1`
(Wails/GTK, llama.cpp, and TLS respectively). The **relay** and **echo** roles
build as pure Go (`CGO_ENABLED=0`) and ship in tiny distroless containers.

### The frontend (Vue)

The desktop UI is a **Vue 3** SPA (Vue 3.5, built with `vue-cli-service`/Webpack,
styled with Tailwind, tested with Vitest). It lives **in-tree** at `frontend/`
and is compiled to `frontend/dist`, which is baked into the Go binary via
`//go:embed frontend/dist` in [`embedded.go`](../embedded.go).

> Older docs mention pulling a separate `warpnet-frontend` Go module and running
> `go mod vendor`. That is **obsolete** — the frontend is now in-tree and
> embedded directly.

To change the UI:

```bash
make frontend           # = cd frontend && make rebuild (clean npm install + build)
# then rebuild the member binary so the new dist gets re-embedded:
cd cmd/node/member && wails build -devtools -tags webkit2_41
```

### The Android client (warpdroid)

`warpdroid/` is a fork of [Tusky](https://github.com/tuskyapp/Tusky) (Kotlin).
Instead of talking HTTP to a Mastodon instance, it talks to a local member node
over libp2p. The Go networking code is compiled into an **Android AAR** with
gomobile and consumed by the Kotlin app:

```bash
make aar                # builds the Go AAR, then runs go mod tidy && go mod vendor
```

The Kotlin side mirrors the wire contract: `ProtocolIds.kt` mirrors
`event/paths.go`, and the DTOs (Moshi `@JsonClass`) mirror the Go `domain`
models. These must stay in lockstep — see [§10](#10-making-changes-a-worked-example).

---

## 7. Running locally

### A single node (isolated dev network)

Pick your own network name to stay completely isolated from testnet/mainnet:

```bash
# member node
./build/bin/warpnet --node.network myownnetwork
# relay node
go run cmd/node/relay/main.go --node.network myownnetwork --node.print-psk
```

### Multiple nodes on one machine

Give each node a **unique port**, a **unique `--database.dir`**, and a stable
`--node.seed` (so its peer ID is deterministic across restarts). The Makefile
ships ready-made targets:

```bash
make run-main      # member, port 4001, default storage dir
make run-second    # member, port 4002, seed "backendtest", database.dir "backend1"
make relay-main    # relay node (prints PSK)
make echo-main     # echo bot, port 4002, database.dir "echo"
```

A realistic local swarm is **one relay + one member + a few echo bots**. The
relay provides discovery; the echo bots give the member peers to interact with.

> **Gotcha:** the desktop app holds a single-instance lock (`net.warpnet.app`)
> and BadgerDB takes an exclusive lock per data directory. If a second GUI
> window refuses to open, that's why — populate the network with **echo** and
> **relay** nodes (headless) rather than multiple desktop instances.

### Data directory layout

Each node's data is isolated by network and `--database.dir`:

```
~/.warpdata/                       # Linux/macOS/Android  (Windows: %LOCALAPPDATA%\warpdata)
├── testnet/
│   ├── storage/                   #   default member DB
│   ├── backend1/                  #   --database.dir backend1
│   └── echo/                      #   --database.dir echo
├── warpnet/                       # mainnet (alias: mainnet → warpnet)
│   └── storage/
└── myownnetwork/
```

Wipe testnet data with `make prune-testnet` (`rm -rf ~/.warpdata/testnet/*`).

### Docker

The **member node cannot run in Docker** — it needs the desktop GUI (Wails).
The headless roles do run in containers. The `deploy/` directory has
docker-compose stacks (relays + moderator + business for testnet/mainnet), and
each role has its own Dockerfile:

```
Dockerfile.relay   Dockerfile.moderator   Dockerfile.business   Dockerfile.echo
```

### Metrics (optional)

`docker-compose.metrics.yaml` brings up Prometheus + Grafana. Nodes push metrics
to the gateway in `--node.metrics.gateway`.

```bash
docker compose -f docker-compose.metrics.yaml up -d
make prometheus    # opens http://localhost:9090/targets
make grafana       # opens http://localhost:3000
```

---

## 8. Configuration reference

All configuration is resolved in [`config/config.go`](../config/config.go) using
[viper](https://github.com/spf13/viper) + [pflag](https://github.com/spf13/pflag).

**Precedence (highest wins):** an explicit **command-line flag** overrides an
**environment variable**, which overrides the built-in **default**. Env-var
names are the flag names uppercased with dots replaced by underscores
(`node.port` → `NODE_PORT`).

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--node.network` | `NODE_NETWORK` | `warpnet` | Network name. `testnet` for the playground; `mainnet` is normalized to `warpnet`. |
| `--node.port` | `NODE_PORT` | `4001` | libp2p TCP listen port (IPv4 + IPv6). |
| `--node.seed` | `NODE_SEED` | *(derived)* | Seed for deterministic peer-ID generation. If empty, derived from `"seed"+network+dbDir+host+port`. |
| `--node.host.v4` | `NODE_HOST_V4` | `0.0.0.0` | IPv4 bind address. |
| `--node.host.v6` | `NODE_HOST_V6` | `::` | IPv6 bind address. |
| `--node.bootstrap` | `NODE_BOOTSTRAP` | *(empty)* | Extra bootstrap multiaddrs, comma-separated. The built-in nodes for the chosen network are appended automatically. |
| `--node.print-psk` | `NODE_PRINT_PSK` | `false` | Print the network PSK on startup (handy for relays/moderators). |
| `--node.metrics.gateway` | `NODE_METRICS_GATEWAY` | `130.94.88.38:4091` | Prometheus push-gateway address. |
| `--node.moderator.modelpath` | `NODE_MODERATOR_MODELPATH` | `/root/.warpdata/Llama-Guard-3-1B.Q8_0.gguf` | Path to the GGUF model (moderator role). |
| `--node.server.port` | `NODE_SERVER_PORT` | `4999` | Dashboard HTTP/WS port (business role). |
| `--node.server.password` | `NODE_SERVER_PASSWORD` | *(empty)* | Pre-shared secret that encrypts dashboard WS traffic (business role). |
| `--logging.level` | `LOGGING_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `--logging.format` | `LOGGING_FORMAT` | `text` | `text` or `json`. |
| `--database.dir` | `DATABASE_DIR` | `storage` | Subdirectory name under `~/.warpdata/<network>/`. |

> The legacy guide referenced flags like `--node.host`, `--server.host`,
> `--server.port`, and `--node.metrics.server`. Those names are **out of date** —
> use the table above (note `--node.host.v4`/`.v6` and `--node.server.*`).

Built-in bootstrap nodes (from `config.go`):

- **mainnet (`warpnet`)**: three nodes on `207.154.221.44` (`:4001/:4002/:4003`)
  plus an RU node on `130.94.88.38:4011`.
- **testnet**: three nodes on `207.154.221.44` (`:4011/:4022/:4033`).

Enable debug logging via env var:

```bash
LOGGING_LEVEL=debug ./build/bin/warpnet --node.network testnet
```

---

## 9. How it works under the hood

### Boot sequence (member node)

1. `cmd/node/member/main.go` starts Wails and the Vue UI; it registers the
   `warpnet://` deep-link scheme and binds the `App` struct so JS can call Go.
2. On startup `App` opens the BadgerDB store, builds the auth/user repos, derives
   the network **PSK** from `(network, version)`, and waits for the user to log
   in.
3. Login (`PRIVATE_POST_LOGIN`) is handled **locally** in `app.go` — it derives
   the ed25519 keypair and unlocks the local database. Logout
   (`PRIVATE_POST_LOGOUT`) tears the node down.
4. Once authenticated, `NewMemberNode(...)` (in `cmd/node/member/node/`) builds
   the libp2p host and wires every subsystem; `Start()` registers stream
   handlers and launches discovery/pubsub/mDNS.

### Core subsystems

| Package | Responsibility |
|---|---|
| `core/node` | Wraps the libp2p host; registers stream handlers behind middleware; provides `SelfStream` (loopback) and peer streaming. |
| `core/stream` | The `WarpRoute` type and routing; in-process loopback streams + the peer stream pool. |
| `core/handler` | One handler per route (50+). Business logic lives here. |
| `core/middleware` | Cross-cutting concerns: signature auth, logging, body unwrap, idempotency. |
| `core/discovery` | Tracks peers as online/offline across DHT, mDNS, gossip, and live streams. |
| `core/dht` / `core/mdns` | Kademlia DHT routing & bootstrap; LAN discovery via multicast DNS. |
| `core/pubsub` | GossipSub topics (peer discovery, notifications). |
| `core/relay` | CircuitRelay v2 for NAT/firewall traversal. |
| `core/crdt` | Crash-safe conflict-free counters for stats (likes/retweets/replies), gossiped between nodes. |
| `core/warpnet` | Type aliases and protocol constants over libp2p. |

### The wire protocol

Every route is a string of the form:

```
/<visibility>/<verb>/<entity>[/<sub>]/<version>
```

Examples from [`event/paths.go`](../event/paths.go):

```go
PRIVATE_POST_TWEET            = "/private/post/tweet/0.0.0"
PRIVATE_GET_TIMELINE          = "/private/get/timeline/0.0.0"
PUBLIC_GET_TWEET              = "/public/get/tweet/0.0.0"
PUBLIC_POST_LIKE              = "/public/post/like/0.0.0"
PUBLIC_POST_FOLLOW            = "/public/post/follow/0.0.0"
PUBLIC_POST_REPORT            = "/public/post/report/0.0.0"
PUBLIC_POST_MODERATION_RESULT = "/public/post/moderate/result/0.0.0"
```

- **`private/*`** routes act on *your own* data and are only processed on the
  owner's node (timeline, drafts, login, bookmarks, blocks…).
- **`public/*`** routes are the peer-to-peer surface: any node may send them, and
  the receiving node verifies the signature before acting (like, follow, reply,
  fetch a tweet…).

Requests travel as an `event.Message` envelope:

```go
type Message struct {
    Body        json.RawMessage // the inner event payload, signed byte-for-byte
    MessageId   domain.ID       // UUID — also used for idempotency
    NodeId      domain.ID       // sender peer ID
    Destination string          // the route string
    Timestamp   time.Time
    Version     string
    Signature   string          // base64( ed25519.Sign(privKey, Body) )
}
```

The desktop bridge (`AppMessage` in `app.go`) uses the JSON field `path` for the
destination; internally it becomes `Message.Destination`. The body is signed
*before* transmission and verified by the auth middleware against the sender's
key, so a peer can't forge actions on your behalf.

### Stream routing: self vs peer

The same handler runs whether a request comes from your own UI or from a remote
peer — only the transport differs:

- **`SelfStream`** — an **in-process loopback** stream. The desktop UI's
  `App.Call` routes everything except login/logout through `SelfStream`, so the
  local node processes its own requests with the same handler/middleware stack a
  remote peer would hit.
- **Peer streaming** (`GenericStream` / the stream pool) — opens a real libp2p
  stream to another node's peer ID. If the peer is unreachable you get
  `warpnet.ErrNodeIsOffline`, and handlers degrade gracefully (store locally,
  let propagation catch up later).

`WarpRoute` exposes helpers like `ProtocolID()`, `IsPrivate()`, and `IsGet()`
used during dispatch.

### Handlers & middleware

Handlers are **factory functions** that take their dependencies as **narrow
interfaces** and return a `warpnet.WarpHandlerFunc`:

```go
type WarpHandlerFunc func(buf []byte, s warpnet.WarpStream) (any, error)
```

Inside, a handler unmarshals the event, validates it, mutates local state via a
repo, optionally forwards to another node, and returns a value that gets
marshaled back as the response. Handlers are registered in the member node's
`setupHandlers` via `SetStreamHandlers(...)`, where each is wrapped with the
middleware chain:

- **auth** — verifies the ed25519 signature and enforces body-size limits
  (a few MB normally; larger for Twitter-archive imports);
- **logging** — request/response logging;
- **unwrap** — extracts the inner body from the stream wrapper;
- **idempotency** — caches responses by `MessageId` so a retried request isn't
  executed twice.

### Database layer

Storage is **BadgerDB** (an embedded LSM key-value store) behind a **repository
pattern** in `database/`. Keys are built with a hierarchical prefix builder
(e.g. `/LIKES/INCR/<tweetID>`), which makes range scans and cursor pagination
cheap. Counters that must converge across nodes (likes, retweets, reply counts)
are kept as a **CRDT PN-counter** in `core/crdt` and gossiped, so they survive
crashes and merge without coordination. `database/like-repo.go` is the canonical
example.

### libp2p networking

- **Transport:** Noise (AEAD-encrypted, authenticated) over TCP, IPv4 + IPv6.
- **Private network (PSK):** every packet is gated by a pre-shared key derived
  from `(network, version)` in `security/`. Nodes on a different network *or* an
  incompatible version simply cannot speak to each other — this is also how a
  version bump can force an upgrade.
- **Discovery:** Kademlia DHT (with the configured bootstrap/relay nodes) plus
  mDNS on the LAN; online/offline state is tracked by `core/discovery`.
- **Pubsub:** GossipSub topics for discovery and notifications.
- **NAT traversal:** CircuitRelay v2 via relay nodes, hole punching where
  possible.
- **Peerstore:** persisted in BadgerDB so peer addresses survive restarts.
- **Censorship resistance (optional):** the
  [`libp2p-camouflage-transport`](#libp2p-camouflage-transport) dependency can
  disguise libp2p traffic as ordinary HTTPS; it is already wired into the Android
  client.

### Identity & security

- **Keypair:** an ed25519 identity derived deterministically from the user's
  credentials (`security/`); the private key never leaves the device.
- **Signing:** `security.Sign` / `VerifySignature` wrap `ed25519` with base64.
- **Codebase hash:** `embedded.go` embeds the entire Go source, and
  `security.GetCodebaseHashHex` hashes it. This provenance hash is used when a
  mobile device pairs with a node, so a thin client can verify what it's talking
  to.

### Moderation flow

Moderation is opt-in and decentralized — there is **no global consensus**. A node
publishes a report (`PUBLIC_POST_REPORT`); a **moderator** node runs Llama Guard
3 locally (the [`moderation`](#moderation) engine) and returns a verdict
(`PUBLIC_POST_MODERATION_RESULT`); the member node then applies it locally. Each
network can run its own moderators, or none.

---

## 10. Making changes: a worked example

Adding a feature usually means touching several layers **in lockstep**. It looks
like a lot the first time and becomes mechanical after that. The `like` feature
is the reference implementation — read it before writing your own.

### Anatomy of the `like` feature

| Layer | File | What's there |
|---|---|---|
| Route | `event/paths.go` | `PUBLIC_POST_LIKE = "/public/post/like/0.0.0"` |
| DTOs | `event/event.go` | `LikeEvent{TweetId, OwnerId, UserId}`, `LikesCountResponse{Count}` |
| Handler | `core/handler/like.go` | `StreamLikeHandler(...) warpnet.WarpHandlerFunc` |
| Storage | `database/like-repo.go` | `LikeRepo.Like(tweetId, userId)` |
| Wiring | `cmd/node/member/node/member-node.go` | repo constructed + handler registered in `setupHandlers` |
| Test | `core/handler/like_test.go` | end-to-end handler test |

The handler is a clean illustration of the P2P propagation pattern:

```go
func StreamLikeHandler(
    repo LikesStorer,            // local like storage
    userRepo LikedUserFetcher,   // user lookups
    notifyRepo ModerationNotifier,
    streamer LikeStreamer,       // for forwarding to other nodes
) warpnet.WarpHandlerFunc {
    return func(buf []byte, s warpnet.WarpStream) (any, error) {
        var ev event.LikeEvent
        if err := json.Unmarshal(buf, &ev); err != nil { return nil, err }
        // ...validate ev.OwnerId / ev.UserId / ev.TweetId...

        num, err := repo.Like(ev.TweetId, ev.OwnerId)  // 1) store my like locally
        if err != nil { return nil, err }

        // 2) if the liked tweet belongs to a *different* node, forward the like
        likedUser, _ := userRepo.Get(ev.UserId)
        if likedUser.NodeId != streamer.NodeInfo().ID.String() {
            _, err = streamer.GenericStream(likedUser.NodeId, event.PUBLIC_POST_LIKE, ev)
            if errors.Is(err, warpnet.ErrNodeIsOffline) {
                return event.LikesCountResponse{Count: num}, nil // graceful degrade
            }
        }
        // 3) the author's node, on receiving the same route, stores it and
        //    adds a "X liked your tweet" notification
        return event.LikesCountResponse{Count: num}, nil
    }
}
```

Note the idioms worth copying: **narrow dependency interfaces** (not concrete
types), **store-locally-then-propagate**, and **treating an offline peer as a
non-fatal outcome**.

### Checklist: add a new route

1. **Route** — add a constant to `event/paths.go` following the
   `/<visibility>/<verb>/<entity>/<version>` convention.
2. **DTOs** — add request/response structs to `event/event.go`.
3. **Handler** — create `core/handler/<feature>.go`: define narrow interfaces for
   the data you need, write `StreamXHandler(...) warpnet.WarpHandlerFunc`.
4. **Storage** — add the method(s) to a repo under `database/` (new repo? follow
   `like-repo.go`).
5. **Wire it up** — construct the repo and register the handler in
   `cmd/node/member/node/member-node.go` (`setupHandlers` → `SetStreamHandlers`).
6. **Test** — add `core/handler/<feature>_test.go` (copy `like_test.go`).
7. **Clients** (only if the route is UI-facing):
   - Desktop: add the path constant + call in `frontend/src/service/service.js`,
     then wire the Vue component.
   - Android: add it to `warpdroid` `ProtocolIds.kt`, add the matching Moshi DTO,
     and call it from `WarpnetRepository`.
8. **House rules** — AGPL header on every new `.go` file (copy from `like.go`,
   including the `// SPDX-License-Identifier: AGPL-3.0-or-later` line). Run
   `make tests`, `go vet ./...`, and `golangci-lint run`.

> There is also a project skill, **`warpnet-add-handler`**, that encodes exactly
> this cross-layer flow if you're working with the agent tooling.

### Changing the desktop UI only

Edit Vue under `frontend/src/`, then `make frontend` and rebuild the member
binary so the new `dist` is re-embedded (see [§6](#6-building-from-source)).

### Cross-language contract: keep it in lockstep

The single biggest source of "it works on desktop but is blank on Android" bugs
is **DTO/route drift**. The Go side is the source of truth:

- `event/paths.go` ↔ `frontend/src/service/service.js` ↔ `warpdroid`
  `ProtocolIds.kt` — the **same** route strings.
- `domain/*` Go structs ↔ Moshi DTOs in `warpdroid` — the **same** JSON field
  names and nullability. A mismatch silently deserializes to blank fields.

There is no in-stream version negotiation: when you bump a route or a DTO, every
client must move together. (The `warpnet-debug-stack` skill is built for
diagnosing these cross-layer mismatches.)

---

## 11. The sister repositories

### `moderation`

**On-device LLM content moderation.** A small Go package that loads **Llama Guard
3 (1B, quantized GGUF)** through a llama.cpp CGO binding and exposes a tiny
interface:

```go
type Engine interface {
    Moderate(content string) (isSafe bool, reason string, err error)
    Close()
}
```

`NewLlamaEngine(modelPath, threads)` loads the model deterministically
(temperature 0) and classifies text against the Llama Guard safety taxonomy
(`S1…S14`, mapped to human-readable reasons in `prompt.go`). All code is behind
`//go:build llama`; `cgo_link.go` adds the static-link path for the prebuilt
archives under `native/`.

**Integration:** the `warpnet` **moderator** node imports this package and calls
`NewLlamaEngine` on startup. Build with `-tags=llama` and the
`CGO_CXXFLAGS="-w -Wno-format -Wno-delete-incomplete"` shown earlier; provide a
GGUF model at `--node.moderator.modelpath`. No network calls, no external API —
moderation is fully self-hosted.

### `activity-pub-gw`

**A stateless ActivityPub gateway** that bridges WarpNet to Mastodon and the
wider Fediverse, so Fediverse users can discover, follow, and interact with
WarpNet users.

The hard rule (see its `CLAUDE.md`): **the gateway is stateless.** The *only*
things allowed on disk are (1) the RSA signing key (`GATEWAY_KEY`) for stable
HTTP-signatures and (2) the Tailscale Funnel node state (`GATEWAY_FUNNEL_DIR`)
for a stable hostname. Everything else — users, posts, followers, the federation
set — is derived at runtime from WarpNet (via the node's public routes) or from
the Fediverse (via ActivityPub).

- **How it talks to WarpNet:** `nodeclient.go` joins the network as an ordinary
  libp2p peer (DHT + bootstrap relays) and queries public routes to resolve
  users, tweets, media, and the follower graph.
- **Inbound** (`inbound.go`): HTTP-signature-verified ActivityPub activities
  (Follow/Like/Create/Undo) are translated into WarpNet routes.
- **Outbound** (`outbound.go`): when a WarpNet user gains a Fediverse follower,
  the gateway polls for new posts and delivers signed `Create(Note)` activities.
- **Deploy:** typically via **Tailscale Funnel** (auto-provisioned `*.ts.net`
  HTTPS) using `Dockerfile.gateway`; `TS_AUTHKEY` is required. See `DEPLOY.md`.
  Bump the `gatewayVersion` constant in `main.go` on every commit.

### `libp2p-camouflage-transport`

**A DPI-evading libp2p transport.** In censored networks, deep-packet-inspection
middleboxes fingerprint and block P2P traffic. This transport makes libp2p
traffic look like ordinary HTTPS:

- **TLS camouflage** — the client uses a real browser TLS fingerprint (uTLS:
  Chrome/Firefox/Safari/Edge/iOS/Android/Yandex) and tunnels the Noise handshake
  and data inside TLS.
- **TCP fragmentation** — the initial ClientHello is split into tiny segments
  with random delays, so stateful DPI that only inspects the first segment can't
  match a libp2p signature.
- **Active-probing defenses** — SNI/ALPN consistency checks and handshake
  timeouts.

Public entry point: `NewCamouflageTransport(...)` returning a
`transport.Transport`, configurable via options (`WithBrowserFingerprint`,
`WithSNI`, `WithFragmentSize`, `WithMaxDelay`, …). **It is already a dependency
of `warpnet`** (see `go.mod`) and registered as a transport in the Android
client (`warpdroid/node/node.go`); it can be opted into elsewhere the same way.

### `blurry`

**A standalone, embeddable libp2p + CRDT key-value store.** It replicates a
key-value space conflict-free across peers using Merkle/delta-CRDTs over
GossipSub, with a Kademlia DHT for discovery and BadgerDB for local storage. It
exposes a thin REST API (`GET/PUT/DELETE /v1/kv/:key`, batch writes) and a CLI
(`cmd/blurry`).

```bash
cd blurry
go build -mod=vendor -o blurry ./cmd/blurry
./blurry --data_dir ./data --listen_port 4001 --http_port 8001
```

Relationship to WarpNet: a **sibling/research project**, not a runtime
dependency of the node. It's a clean place to study libp2p + CRDT replication in
isolation, and a candidate building block for future shared-state features.

---

## 12. Testing, versioning & git workflow

### Tests & linting

```bash
make tests          # CGO_ENABLED=0 go test -short -p 8 -v ./...
go vet ./...
golangci-lint run
```

Keep `go vet` and the linter clean before opening a PR.

### License headers

Every new `.go` file must carry the AGPLv3 header. The simplest path is to copy
the block from `core/handler/like.go` (the long AGPL notice **plus** the
`// SPDX-License-Identifier: AGPL-3.0-or-later` line).

### Versioning

The `version` file holds a semver patch that is **bumped on every commit**. On
non-`main` branches a git hook does this for you (and keeps
`snap/snapcraft.yaml` in sync). Install the hooks once:

```bash
make setup-hooks    # git config core.hooksPath .githooks
```

The pre-push hook tags `v<version>` and pushes to both GitHub and the Codeberg
mirror. Don't hand-edit the version when the hook is active — let it do its job.

### Branches & PRs

1. Fork the repo.
2. Branch: `git checkout -b <IssueNumber>/<ShortFeatureName>`.
3. Make your change (AGPL header on new files; tests + vet + lint clean).
4. Commit in **imperative present tense** ("Add bookmark filter handler").
5. Open a PR that **names the route(s)/path(s) you touched** — reviewers map
   changes to layers via the route string.

Good first contributions: browse issues labeled
[`good first issue`](https://github.com/Warp-net/warpnet/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22),
fix the frontend, improve error messages/docs, or run a public relay node.

---

## 13. Troubleshooting & gotchas

| Symptom | Likely cause / fix |
|---|---|
| Second desktop window won't open | Single-instance lock (`net.warpnet.app`) + exclusive BadgerDB lock. Use **echo**/**relay** nodes to populate a local network, not multiple GUIs. |
| Two nodes won't see each other | Different `--node.network`, or **version mismatch** — the PSK is derived from `(network, version)`. Match both. |
| Android shows blank rows/fields, desktop is fine | DTO/route **drift**: Kotlin Moshi DTO or `ProtocolIds.kt` is out of sync with the Go `domain`/`event/paths.go`. Realign field names & route strings. |
| Member node won't start in Docker | By design — it needs the Wails GUI. Run **relay/moderator/business/echo** in Docker instead. |
| Moderator fails to start | Missing GGUF model at `--node.moderator.modelpath`, or built without `-tags=llama` / CGO. |
| `wails build` fails on Linux | Missing GTK/WebKit dev libs — install the `apt` packages in [§6](#6-building-from-source) and use `-tags webkit2_41`. |
| Frontend changes don't show up | You must rebuild the frontend (`make frontend`) **and** rebuild the Go binary so `frontend/dist` is re-embedded. |
| Want a clean slate | `make prune-testnet` (or delete `~/.warpdata/<network>`). |
| Need to see what's happening | `LOGGING_LEVEL=debug` (and `--logging.format json` for structured logs). |

---

## 14. Community & getting help

- **Telegram (dev chat):** https://t.me/warpnetdev — the fastest way to find a
  task that fits you.
- **Issues:** https://github.com/Warp-net/warpnet/issues
- **Codeberg mirror:** https://codeberg.org/Warpnet/warpnet
- **Snap Store:** https://snapcraft.io/warpnet

WarpNet is real, working, ambitious, and carried by a very small team. If
decentralized systems, libp2p, or Go are your thing, there is a lot of room here
to own a meaningful piece. Every contribution — code, docs, bug reports, or just
kicking the tyres on testnet — genuinely helps.

**Built with Go, libp2p, Noise, and the conviction that a social network
shouldn't have an owner.**
