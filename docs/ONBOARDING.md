# Warpnet Contributor Onboarding

**A complete, end-to-end guide for new contributors** — the *why* behind every
piece, the *how it works*, the *how to run it*, and the *how to change it*,
across the whole Warpnet ecosystem.

This document is longer than a typical README on purpose. It is meant to take
you from "I just cloned the repo" to "I understand *why* the architecture looks
like this and can ship a feature" without reverse-engineering the codebase
first. Skim the table of contents and jump to what you need.

For the short version, see [`README.md`](../README.md) and
[`HOW-TO-HELP.md`](../HOW-TO-HELP.md). This guide goes deeper, and every section
tries to answer **"why is it built this way?"**, not just "what is it".

---

## Table of contents

1. [Philosophy: why Warpnet exists](#1-philosophy-why-warpnet-exists)
2. [The ecosystem at a glance](#2-the-ecosystem-at-a-glance)
3. [Node roles (and why each one exists)](#3-node-roles-and-why-each-one-exists)
4. [Repository layout](#4-repository-layout)
5. [Installing & using Warpnet](#5-installing--using-warpnet)
6. [Building from source](#6-building-from-source)
7. [Working on the desktop frontend (Vue + Wails)](#7-working-on-the-desktop-frontend-vue--wails)
8. [Working on the Android client (warpdroid + the AAR)](#8-working-on-the-android-client-warpdroid--the-aar)
9. [The business node: one UI, served over the wire](#9-the-business-node-one-ui-served-over-the-wire)
10. [Running locally](#10-running-locally)
11. [Configuration reference](#11-configuration-reference)
12. [How it works under the hood](#12-how-it-works-under-the-hood)
13. [Making changes: a worked example](#13-making-changes-a-worked-example)
14. [The sister repositories](#14-the-sister-repositories)
15. [Testing, versioning & git workflow](#15-testing-versioning--git-workflow)
16. [Troubleshooting & gotchas](#16-troubleshooting--gotchas)
17. [Community & getting help](#17-community--getting-help)

---

## 1. Philosophy: why Warpnet exists

Every "decentralized" social network still tends to keep a soft center:

- **Federated networks** (Mastodon) still put you on an *instance*. The admin
  holds your data, sets the rules, can defederate, and can switch the server
  off. You traded one landlord for thousands of smaller ones.
- **Relay-based networks** (Nostr) make the client thin and push storage onto
  *relays* — stateful servers that can rate-limit you, refuse your events, or
  delete your data.
- **"Protocol" networks** (atproto/Bluesky) decentralize in theory but
  concentrate around a few large providers in practice.

**Warpnet removes the server tier entirely.** Every user runs a node; the node
*is* their participation in the network. There is no instance to seize, no admin
to trust, and no relay that can quietly drop you.

| | **Warpnet** | Mastodon | Nostr | Bluesky / atproto |
|---|:---:|:---:|:---:|:---:|
| Architecture | True P2P (libp2p) | Federated instances | Client + relay | PDS + relays |
| Server required? | **None** | Yes (instance) | Yes (relays) | Yes (providers) |
| Where your data lives | **Your own node** | Admin's DB | Relays you post to | Your PDS provider |
| Transport security | **Noise protocol** | TLS to instance | TLS to relay | TLS to provider |
| Can an admin deplatform you? | **No admin exists** | Yes | Relay can drop you | Provider can drop you |
| Discovery | **DHT** | Instance + relays | Relay lists | Relays / indexers |
| Moderation | LLM moderator nodes | Per-instance | Per-client | Labelers |

**The core bet:** the only way to be genuinely censorship-resistant is to remove
the server, not to multiply it. So discovery happens over a Kademlia **DHT**,
transport is encrypted with the **Noise** protocol, content propagates **peer to
peer**, and your data lives in an embedded datastore on *your* machine. There is
nothing in the middle to capture, subpoena, or switch off.

### One binary *is* the whole product

Warpnet ships as a **single, self-contained executable — and it always will.**
Everything is baked into that one file: the node engine, the entire Vue UI
(embedded via `//go:embed frontend/dist`), the icon, the version, and even the
**full Go source tree** — embedded so the binary can hash and attest to its own
code (the "codebase hash" used during device pairing, §12). There is no separate
frontend to deploy, no runtime assets to fetch, no dependency to install
alongside it. You run one file and you *are* a full node with a UI.

**This single-binary form is the single most important way Warpnet is
distributed — by design, not by accident.** A lone executable needs no app
store, no installer, no website, and no server to pull anything from. It can be
checksummed, mirrored, copied to a USB stick, sideloaded, or handed
peer-to-peer from one person to the next. If a download page or an app store is
blocked, anyone who already has the file can simply give it to you — the same
resilience the network itself is built on. Packaging such as the Snap (§5) is
**only a convenience wrapper** around that one binary: delivery channels are
replaceable, but the single-file form is not.

A censorship-resistant *network* whose *client* could be choked off at an app
store or a download server would be resistant in name only. Keeping the whole
product in one portable, verifiable file closes that gap — which is exactly why
it is the canonical artifact every other distribution method just wraps.

**Why this matters for you as a contributor:** almost every design decision in
the codebase follows from "there is no server". No server means no central
database (so storage is local and per-node), no trusted endpoint (so every
message is signed and verified), no central moderator (so moderation is opt-in
and runs on dedicated LLM nodes), and no single UI host (so the same UI has to
reach a node three different ways — see §7–§9). When something looks more
complicated than it would be in a client/server app, this is usually why.

**Why AGPLv3?** A censorship-resistant network must stay free and open: anyone
who runs a modified version that others interact with has to share their
changes. Warpnet is AGPLv3, and it is not a startup — there is no monetization
and no owner. It is a protocol for people who want to own their tools.

---

## 2. The ecosystem at a glance

Warpnet is not one repository — it is a small constellation. The main node lives
in `warpnet`; the others are focused components that plug into or extend it.

| Repository | Language | What it is | Why it exists / relationship to the node |
|---|---|---|---|
| **`warp-net/warpnet`** | Go + Vue + Kotlin | The main project: node engine, desktop UI, Android client | This is the core. Everything else orbits it. |
| **`warp-net/moderation`** | Go + C/C++ (CGO) | LLM content-moderation engine (Llama Guard 3 via llama.cpp) | Moderation with no human admin: imported by the **moderator** node (`-tags=llama`). |
| **`warp-net/activity-pub-gw`** | Go | Stateless ActivityPub gateway to the Fediverse/Mastodon | Lets the closed P2P network be discovered/followed from Mastodon. Joins as a libp2p peer. |
| **`warp-net/libp2p-camouflage-transport`** | Go | libp2p transport that disguises traffic to defeat DPI censorship | "No server to block" only holds if the *traffic* can't be blocked either. A direct dependency of `warpnet`; wired into the Android client. |

The three sister repos are covered in detail in
[§14 The sister repositories](#14-the-sister-repositories).

### One protocol, three transports — the central idea

This is the most important architectural idea in the whole project, and it is
the thing most new contributors miss.

There is **one canonical handler per route** on the Go side. The desktop app,
the browser dashboard, and the Android app are all just **different ways of
getting a request to that same handler**. They share the exact same route
strings (`event/paths.go`) and the same `{path, body, …}` request envelope.

```
                         ┌───────────────────────────────────┐
                         │   the node (Go)                     │
                         │   core/handler/<feature>.go         │  ← ONE canonical handler per route
                         │   database/<feature>-repo.go        │
                         └───────────────▲─────────────────────┘
                                         │  same {path, body} envelope, same handlers
        ┌────────────────────────────────┼────────────────────────────────┐
        │                                 │                                 │
 in-process Wails call           AES-256-GCM WebSocket             libp2p stream over Noise
   (no network boundary)        (network boundary → encrypted)     (network boundary → signed)
        │                                 │                                 │
  Vue desktop app                  Vue browser dashboard              Android app (Tusky fork)
  on the MEMBER node               on the BUSINESS node               warpdroid/  (via gomobile AAR)
  (frontend/, §7)                  (§9)                               (§8)
```

Keep this diagram in mind: §7, §8, and §9 are really three answers to one
question — *"how does the UI talk to the node, and why that way?"*

---

## 3. Node roles (and why each one exists)

A Warpnet binary plays one of several roles. The roles exist because "everyone
runs a full node" is the ideal, but a real network also needs stable entry
points, optional moderation, and server-hosted presences. Each role is a
different build of the same codebase.

| Role | Entry point | What it does | CGO / build tag |
|---|---|---|---|
| **member** | `cmd/node/member/main.go` | The full "fat" node most people run. Holds local data, serves the desktop UI, pairs with mobile devices, participates in the P2P network. | `CGO_ENABLED=1`, `-tags webkit2_41` (Wails) |
| **relay** | `cmd/node/relay/main.go` | Stable, stateless entry points. Help new nodes find peers via the DHT and provide NAT-traversal relaying. | `CGO_ENABLED=0`, pure Go |
| **moderator** | `cmd/node/moderator/main.go` | Runs an on-device LLM (Llama Guard 3) to evaluate reported content. | `CGO_ENABLED=1`, `-tags=llama` |
| **business** | `cmd/node/business/main.go` | A headless node that serves the same UI to a **browser** over an encrypted WebSocket (for server/hosted deployments). | no special CGO/tags (plain `go build`) |
| **echo** | `cmd/node/member/echo-member.go` | A headless "bot" member node with an in-memory store, to populate a local network and for tests. | `CGO_ENABLED=0`, `-tags echo` |

> [!NOTE]
> **Why these CGO settings?** (These are taken straight from the `Dockerfile.*`
> for each role, so they match what ships.) The **member** node links GTK/WebKit
> for the Wails desktop UI → needs cgo. The **moderator** links llama.cpp →
> needs cgo. The **relay** and **echo** roles are pure Go on purpose, so they
> compile to tiny static binaries that run in `distroless` containers. The
> **business** node’s `Dockerfile.business` does **not** set `CGO_ENABLED` at
> all — it’s an ordinary `go build ./cmd/node/business` with no cgo libraries
> required.

> [!TIP]
> **Mental model:** *relays* are thin signposts, *members* are the network
> itself, *moderators* are optional opt-in referees, *business* is "Warpnet on a
> server, used from a browser", and *echo* is a test bot. Only **member** has a
> desktop GUI; only **member** and **business** serve the UI at all.

Note the older docs’ "bootstrap node": there is **no** `cmd/node/bootstrap`. The
**relay** role is what acts as a bootstrap/entry point now.

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
│   └── business/         #   headless node + browser dashboard (HTTP/WS)
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
├── frontend/             # Vue 3 UI (served by member via Wails, by business via WS)
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
**`event/paths.go`** (the route) → **`core/handler/like.go`** (the handler) →
**`database/like-repo.go`** (the storage). `like` is intentionally the cleanest
small reference, and `core/handler/like_test.go` is a complete worked test.

---

## 5. Installing & using Warpnet

Before contributing, *use* the thing. **Prefer a prebuilt binary** — you do not
need a Go toolchain to run Warpnet, and starting as a plain user is the fastest
way to understand what you're building.

### Install a prebuilt binary

**Linux (recommended):** install from the Snap Store — one command, no compiler:

```bash
sudo snap install warpnet
warpnet
```

[Snap Store →](https://snapcraft.io/warpnet)

> [!IMPORTANT]
> The Snap is just **one convenient channel**. The canonical artifact is the
> **single self-contained binary itself** (see
> [One binary *is* the whole product](#one-binary-is-the-whole-product)) — you
> can run it directly, verify its checksum, mirror it, sideload it, or share it
> peer-to-peer with no store and no server involved. This is Warpnet's primary,
> most resilient distribution method; every other channel just wraps that file.

### Platform support

| Platform | Status | How to run |
|---|---|---|
| **Linux** | ✅ Supported | Snap (prebuilt) or build from source. |
| **Windows** | ✅ Supported (developers) | Build from source (`wails build -platform windows -tags webkit2_41`, see `make build-windows`). |
| **macOS** | ❌ **Not supported** | The builds are **unsigned**, so macOS Gatekeeper blocks them. The code compiles and is tested on macOS in CI, but there is no distributable build — don’t expect to run it there. |

### First run & identity

On first launch you create a **local identity**: a keypair derived on your
device from your credentials. The private key never leaves your machine — there
is no account on any server, because there is no server. You are now a node.

### Networks

Warpnet runs separate, isolated networks. **The default network is `warpnet`
(mainnet).** Use the testnet to experiment safely:

```bash
warpnet --node.network testnet   # safe playground
warpnet --node.network mainnet   # the live network (alias of "warpnet")
```

Nodes only talk to other nodes on the **same network *and* a compatible
version** — the network membership key (PSK) is derived from both, on purpose
(see [§12](#12-how-it-works-under-the-hood)).

### What you can do

A Twitter-style social network: profiles, posts ("tweets"), replies, likes,
retweets, bookmarks, follows (with follow-request approval for private
accounts), direct chat, notifications, media/images, search, mutes/blocks,
content filters, and Twitter-archive import. Public content propagates
peer-to-peer; private data (timeline, chats, drafts) never leaves your node.

---

## 6. Building from source

This section covers the Go binaries. The two client UIs get their own deep
dives: [§7 frontend](#7-working-on-the-desktop-frontend-vue--wails) and
[§8 Android](#8-working-on-the-android-client-warpdroid--the-aar).

### Prerequisites

- [Go 1.26+](https://go.dev/doc/install) (the module targets `go 1.26.3`)
- [Wails v2.10.2](https://github.com/wailsapp/wails) — for the desktop client
- Linux GUI/build dependencies (only needed for the member/desktop node):

```bash
sudo apt update
sudo apt install -y pkg-config build-essential
sudo apt install -y \
  libgtk-3-dev libwebkit2gtk-4.1-dev libglib2.0-dev libcairo2-dev \
  libpango1.0-dev libgdk-pixbuf-2.0-dev libatk1.0-dev libsoup-3.0-dev
git submodule update --init --recursive
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

### Build & run the member node (desktop UI)

```bash
cd cmd/node/member
wails build -devtools -tags webkit2_41          # compile the binary
./build/bin/warpnet --node.network testnet      # run it on the testnet
```

`make run-main` does the equivalent. On **Windows**, use
`wails build -platform windows -tags webkit2_41` (or `make build-windows`).
**macOS is not a supported target** (unsigned — see §5).

### Build & run the other roles

```bash
# Relay (pure Go, no GUI). --node.print-psk prints the network key on startup.
go run cmd/node/relay/main.go --node.network testnet --node.print-psk

# Moderator (LLM). Needs a GGUF model present (see --node.moderator.modelpath)
# and a C/C++ toolchain. The CXXFLAGS quiet llama.cpp build warnings.
CGO_CXXFLAGS="-w -Wno-format -Wno-delete-incomplete" \
  go run -tags=llama cmd/node/moderator/main.go \
  --node.network testnet --node.port 4002 --node.seed moderatorlocalhost

# Business (headless node + browser dashboard). Password is REQUIRED — see §9.
go run ./cmd/node/business \
  --node.network testnet --node.server.port 4999 --node.server.password 'choose-a-secret'

# Echo (headless bot member, in-memory store — great for tests/local swarms)
go run -tags echo cmd/node/member/echo-member.go \
  --node.network testnet --node.seed echo --database.dir echo --node.port 4002
```

### Build tags & CGO, explained

| Tag | Used by | Why it exists |
|---|---|---|
| `webkit2_41` | member (desktop) | Selects the WebKit2GTK 4.1 bindings Wails uses to render the UI. |
| `llama` | moderator | Compiles in the `moderation` engine and its llama.cpp cgo bindings. |
| `echo` | echo bot | Swaps in the in-memory store and bot behavior. |
| `mobile` | Android AAR | Gates the gomobile bridge in `warpdroid/node/` (see §8). |

CGO is `1` for **member** (GTK/WebKit) and **moderator** (llama.cpp), `0` for
**relay** and **echo** (so they static-link into distroless containers), and
unset/default for **business** (no cgo needed). These come straight from the
`Dockerfile.*` files, so they match what ships.

---

## 7. Working on the desktop frontend (Vue + Wails)

**This is the section for frontenders.** It explains exactly how the UI reaches
the node, why it’s built this way, how to change it, and how to run a node with
your changes.

### What the frontend is

`frontend/` is a **Vue 3** single-page app (Vue 3.5, built with
`vue-cli-service`/Webpack, styled with Tailwind, tested with Vitest). It lives
**in-tree** and is the *same* app for both the desktop (member) and the browser
dashboard (business) — only the transport differs.

### How the UI talks to the node — the bridge

You almost never think about libp2p in the frontend. You call **one function**,
`Call({ path, body })`, and the node answers. Two files make that work:

1. **`frontend/src/service/service.js`** — the API surface. It exports the route
   constants (the **same strings as Go’s `event/paths.go`**) and thin helpers,
   e.g.:

   ```js
   export const PUBLIC_GET_TWEET = "/public/get/tweet/0.0.0";
   // ...a UI component calls:
   import { Call, PUBLIC_GET_TWEET } from "@/service/service";
   const resp = await Call({ path: PUBLIC_GET_TWEET, body: { id } });
   // resp.body is the handler's response payload
   ```

2. **`frontend/src/lib/transport.js`** — the bridge. It exposes `Call`,
   `IsFirstRun`, `IsDesktop`, etc., and **auto-detects which transport to use**:

   - **Desktop (member):** if `window.go.main.App.Call` exists (the Wails
     runtime injects it), it delegates straight to the **bound Go method** —
     an *in-process function call*, no network, no sockets. These bindings are
     auto-generated by Wails into `frontend/wailsjs/go/main/App.js` from the
     exported methods on the Go `App` struct (`Call`, `IsFirstRun`,
     `ConsumePendingDeepLink`). Regenerated whenever you run `wails build`/`wails dev`.
   - **Browser (business):** otherwise it opens a single **WebSocket** to `/ws`
     and rides every request on it, **AES-256-GCM encrypted** with
     `SHA-256(password)` (see §9).

   The request envelope is `{ path, body, message_id, node_id, timestamp }` and
   the response is `{ body, … }` — identical in both transports.

> [!NOTE]
> **Why a single `Call` with two backends?** Because the UI must run unchanged
> in two very different places: embedded in a desktop binary (where Go is a
> function call away) and in a remote browser (where the node is across a
> network and the link must be encrypted). Hiding that behind `transport.js`
> means components, views, and `service.js` never branch on environment — write
> the feature once, it works on both.

On the Go side, `cmd/node/member/app.go`’s `App.Call(...)` receives the envelope,
handles `login`/`logout` locally, and routes everything else through the node’s
in-process **`SelfStream`** to the same canonical handler a remote peer would
hit (see §12). So "calling the backend" from Vue and "a peer sending you a
request over libp2p" converge on the exact same handler code.

### How to change the frontend

- Edit components/views/router under `frontend/src/`.
- To call the backend, use (or add) a path constant in
  `frontend/src/service/service.js` and call `Call({ path, body })`. If the
  route is new, it must already exist in Go’s `event/paths.go` and have a
  handler (see §13).
- Keep the path strings **byte-identical** to `event/paths.go` — a typo here is
  a silent "route not found".

### How to run a node with your frontend changes

There are three workflows; pick by what you’re doing.

**A. Fast UI loop with live reload (recommended for iterating):**

```bash
cd cmd/node/member
wails dev -tags webkit2_41        # live-reloading window + devtools
```

`wails.json` sets `frontend:dev:serverUrl: "auto"`, so `wails dev` runs the Vue
dev server and reloads on save.

**B. Production-style desktop build (what users actually run):**

```bash
make frontend                                   # rebuild frontend/dist
cd cmd/node/member && wails build -devtools -tags webkit2_41
./build/bin/warpnet --node.network testnet
```

> [!IMPORTANT]
> **Why the extra `make frontend` step:** `wails.json` has
> **`skipfrontend: true`**, so `wails build` does **not** compile the Vue app
> for you. And the binary doesn’t use Wails’ own asset bundling — it embeds
> `frontend/dist` via `//go:embed frontend/dist` in `embedded.go`. So a
> production build only reflects UI changes after you rebuild the dist **and**
> rebuild the binary. Skip `make frontend` and you’ll run a stale UI.

> [!NOTE]
> **`frontend/dist` is committed to the repository — on purpose, against the
> usual convention.** Build output is normally git-ignored, but here the
> checked-in `dist/` is exactly what `//go:embed frontend/dist` bakes into the
> binary at `go build` time. Since `go build` never runs npm, committing `dist/`
> is what lets *anyone* — CI, `go install`, or a contributor who only touches Go
> — produce a working binary with a real UI **without a Node toolchain at all**.
> This is also what makes the single self-contained binary possible (see §1). The
> trade-off: when you change the UI you must rebuild **and commit**
> `frontend/dist` alongside your source, so the embedded copy stays in sync.

**C. In a plain browser via a business node (no GTK/Wails needed):**

```bash
go run ./cmd/node/business --node.network testnet \
  --node.server.port 4999 --node.server.password 'secret'
# then open http://localhost:4999 in your browser
```

This serves the same Vue app to your browser over the encrypted WebSocket. It’s
the easiest path if you can’t (or don’t want to) build the Wails/GTK desktop
stack. For pure visual/component work, `cd frontend && npm run serve` gives Vue
hot-reload (point it at a running business node for live data).

---

## 8. Working on the Android client (warpdroid + the AAR)

`warpdroid/` is a fork of [Tusky](https://github.com/tuskyapp/Tusky) (a Mastodon
client, Kotlin). Instead of talking HTTP to a Mastodon instance, it talks to a
Warpnet node over libp2p. To understand the Android client you have to
understand **why there’s an AAR**.

### Why an AAR exists (the whole point)

Warpnet’s protocol is non-trivial: a libp2p host, the Noise transport, the DHT,
Ed25519 signing, the DPI-camouflage transport, and the signed `event.Message`
envelope — all implemented in **Go**. Re-implementing all of that in Kotlin
would be a second, parallel protocol stack that would inevitably drift from the
Go one and break the network contract.

So instead, the Go networking code is compiled **into the Android app** as a
native library:

- `warpdroid/node/*.go` (guarded by `//go:build mobile`) is cross-compiled by
  **gomobile** into `warpnet.aar` — native `.so` libraries plus a generated
  JNI/Java binding — by `warpdroid/node/build-native.sh`:

  ```bash
  gomobile bind -tags mobile -androidapi 21 -target=android/arm64 \
    -ldflags="-checklinkname=0 -s -w" -trimpath -o warpnet.aar .
  # → moved into warpdroid/warpnet-transport/libs/warpnet.aar
  ```

The result: **the phone runs the *same* Go libp2p node as the desktop**, inside
the Android process. Kotlin doesn’t reimplement the protocol — it *calls* it.
That’s the concrete mechanism behind "one protocol, many clients": there is a
single source of truth for the wire protocol, and the mobile client reuses it.

### The three Android layers

```
warpdroid/
├── node/                 # Go (build tag `mobile`) → compiled to warpnet.aar via gomobile
├── warpnet-transport/    # Kotlin library that wraps the AAR (the bridge)
│   └── ...               #   WarpnetClient, ProtocolIds.kt, Moshi DTOs, EnvelopeSigner
└── app/                  # the Tusky-fork UI + WarpnetRepository (uses the transport)
```

The Go bridge (`warpdroid/node/mobile.go`) exposes deliberately simple,
**stringly-typed** functions, because gomobile can only bind a limited set of
types (so payloads are hex/JSON strings):

| Go function | Meaning |
|---|---|
| `Initialize(privKeyHex, network, pskHex, bootstrap)` | Spin up the phone’s libp2p node with the user’s key, the network PSK, and bootstrap peers. |
| `Connect(addrInfo)` | Connect to a peer (typically the paired fat/member node). |
| `Stream(protocolID, data)` | Open a libp2p stream on a route and return the response — **the mobile equivalent of the desktop’s `Call`/`SelfStream`**, using the same route strings. |
| `Sign(body)` | Ed25519-sign the body with the libp2p identity key, so the node’s auth middleware verifies it against the same peer ID it sees on the stream. |
| `PeerID()`, `IsConnected()`, `Connectedness()` | Status the Kotlin `ConnectionMonitor` polls to drive reconnect/UI. |
| `Pause()`/`Resume()` | Background/foreground transitions (mobile lifecycle). |
| `RefreshPeerAddrs(addrs)` | Update the fat node’s addresses after a pairing handshake. |
| `Disconnect()`, `Shutdown()` | Tear down. |

### Pairing a phone with a fat node — and why it works this way

In the glossary, a phone is a **thin client / alias / device**; the computer
node is the **fat node / backend**. Rather than make the phone a full,
always-online peer, Warpnet **pairs** it to a fat node and lets the phone act
*through* that node.

**Why pair at all?** A phone is the worst possible P2P peer: the OS kills
background apps, the radio sleeps, battery and storage are scarce, and its IP
changes constantly. It cannot reliably stay online to hold canonical data or
keep DHT/relay connections alive. The fat node runs on an always-on computer —
it holds the account’s data and keeps the connections up — and the phone becomes
a lightweight remote for it. The phone is an **alias/device** of the account
that lives on the fat node, not a second copy of it.

**The QR carries an `AuthNodeInfo`, never a private key.** To link the two
devices, the fat node renders its `AuthNodeInfo` as a **QR code**. That struct
(`domain/warpnet.go`) carries everything the phone needs to find, join, and be
authorized by the node:

| Field | Why it’s in the QR |
|---|---|
| `token` | A secret minted by the fat node — the **pairing authorization** (see below). |
| `psk` | The private-network key, so the phone can speak to the network at all. |
| `node_id` | The fat node’s **peer ID**, pinned during dialing to prevent impersonation. |
| `addresses` / `bootstrap_peers` | Where to reach the fat node and the network. |
| `network`, `user_id`, `role` | Which network/account the device is joining. |

Because `AuthNodeInfo` is bigger than a QR’s byte mode likes, the desktop
gzip-compresses it and Base45-encodes it (`frontend/src/lib/qr-payload.js`); the
phone reverses that with `QrPayloadCodec`/`QrCodeAnalyzer`. **Crucially, the fat
node’s private key never leaves the desktop** — the phone generates *its own*
libp2p identity.

**The handshake** (`PairingCoordinator` on the phone, `core/handler/pair.go` on
the node):

1. Phone scans the QR, decodes and validates the `AuthNodeInfo`.
2. Phone derives **its own** Ed25519 identity — deterministically from device
   info plus the fat node’s pinned peer ID, so the same phone always re-pairs to
   the same node with the same key (and pairing to a *different* node rotates the
   key).
3. Phone dials the node using addresses pinned with the advertised peer ID
   (`<addr>/p2p/<node_id>`). The **Noise handshake aborts if the remote’s key
   doesn’t match** the pinned ID — so nobody can impersonate the fat node.
4. Phone sends `PRIVATE_POST_PAIR` (`/private/post/admin/pair/0.0.0`) carrying
   the payload, which includes the **token**.
5. The node accepts **only if the token matches its own**, then records the
   phone’s peer ID as an authorized `Device` and replies with its **current
   public addresses**. The phone persists the pairing only on a
   `{"code":0,"message":"Accepted"}` reply.

**Why each piece is there:**

- **Own identity, not the master key** — a QR can be photographed, so the root
  private key must never be in it. The phone is a *separate, revocable* peer:
  lose the phone and the fat node simply forgets that `Device`; the account’s
  master key was never exposed.
- **Token = authorization** — possessing it proves the pairer physically saw
  *this* node’s screen. The node refuses any pair whose token doesn’t match, so a
  random peer can’t pair itself in.
- **PSK = network admission** — without the private-network key the phone can’t
  talk to the network at all; the QR hands it over out-of-band.
- **Peer-ID pinning = no MITM** — the dialed node must cryptographically prove it
  owns the advertised peer ID during the Noise handshake.
- **QR = a local, serverless, out-of-band channel** — pairing data crosses an
  air-gap (screen → camera) with no server and no typing, which fits Warpnet’s
  "no middle" stance; hence the gzip+Base45 packing to make it fit.

**Staying paired.** A home fat node’s public address drifts (dynamic IP, NAT), so
a background `PairRefreshWorker` periodically refreshes it: every successful pair
response returns the node’s current addresses, which the bridge merges into the
peerstore (`RefreshPeerAddrs`) so the phone can reconnect.

After pairing, the phone opens libp2p streams to the fat node using the shared
route strings, **signing each request with its own key** (`Sign`) so the node’s
auth middleware verifies it against the peer ID it sees on the stream. On hostile
mobile networks the camouflage transport (§14) disguises those connections as
HTTPS. As always, the Kotlin `ProtocolIds.kt` and Moshi DTOs must stay in
lockstep with Go’s `event/paths.go` and `domain/` — drift shows up as blank
fields (§16).

### How to build it

```bash
cd warpdroid && make build            # ./gradlew build  +  node/build-native.sh
# or rebuild just the native AAR after Go changes:
cd warpdroid/node && ./build-native.sh
```

When you change anything in `event/paths.go` or `domain/` that mobile uses,
rebuild the AAR **and** update the Kotlin mirrors (`ProtocolIds.kt`, DTOs).

---

## 9. The business node: one UI, served over the wire

The business node is the answer to: *"How do I run Warpnet on a server and use
it from a browser, when the desktop node bakes its UI into a GTK window?"* It’s
also the clearest example of *why* the frontend is decoupled from the backend.

### Member vs business: where the UI runs

- On the **member** node, the Vue app runs **inside** the process (Wails
  webview). The UI↔node link is an **in-process function call**
  (`window.go.main.App.Call`). There is no network on that boundary, so there is
  nothing to authenticate or encrypt there — it’s one program.
- On the **business** node there is **no desktop webview**. The node runs
  headless and instead **serves the same Vue app over HTTP** and bridges the UI
  to the node over a **WebSocket**. Now the UI↔node boundary *is* a network hop
  (possibly across the internet), so it **must** be authenticated and encrypted.

That is the entire reason the frontend is "split from the backend" in the
business node: the same UI, but reached over the wire instead of in-process.

### How it’s wired (`cmd/node/business/main.go`)

The business node starts a plain `net/http` server that:

- serves the embedded Vue app at `/` (`StaticHandler`),
- bridges the browser to the node at `/ws` (`BridgeHandler`),
- exposes `/healthz` and `/readyz`,

and prints `NODE IS LISTENING ON 'localhost:<port>'. PUT THIS ADDRESS INTO A
BROWSER`. The WebSocket frames are sealed with **AES-256-GCM** using
`key = SHA-256(password)` — the very same `--node.server.password` you launch it
with. The browser’s `transport.js` derives the identical key from the password
the user types, so:

- `is-first-run` and the `login` frame establish the channel (login precedes the
  key, so it’s the first encrypted frame);
- every frame after a successful login is encrypted;
- a successful login authenticates the socket.

That’s why `--node.server.password` is **required** (the node refuses to start
without it): it’s the pre-shared key for the UI↔node channel.

### Why you’d use it

- **Hosted / always-on presence:** run Warpnet on a VPS, use it from any
  browser — no desktop GTK stack required.
- **Frontend development without Wails:** the easiest way to hack on the Vue app
  on a machine where you can’t build the desktop client (see §7, workflow C).
- **Integration surface:** a headless node with a clean HTTP boundary is a
  natural place to attach bridges and dashboards.

---

## 10. Running locally

### A single node (isolated dev network)

Pick your own network name to stay completely isolated from testnet/mainnet:

```bash
./build/bin/warpnet --node.network myownnetwork                 # member
go run cmd/node/relay/main.go --node.network myownnetwork --node.print-psk
```

### Multiple nodes on one machine

Give each node a **unique port**, a **unique `--database.dir`**, and a stable
`--node.seed` (so its peer ID is deterministic across restarts):

```bash
make run-main      # member, port 4001, default storage dir
make run-second    # member, port 4002, seed "backendtest", database.dir "backend1"
make relay-main    # relay node (prints PSK)
make echo-main     # echo bot, port 4002, database.dir "echo"
```

A realistic local swarm is **one relay + one member + a few echo bots**: the
relay provides discovery, the echo bots give the member peers to interact with.

> [!WARNING]
> **Gotcha:** the desktop app holds a single-instance lock (`net.warpnet.app`)
> and BadgerDB takes an exclusive lock per data directory. If a second GUI
> window refuses to open, that’s why — populate the network with headless
> **echo**/**relay** nodes rather than multiple desktop instances.

### Data directory layout

Each node’s data is isolated by network and `--database.dir`:

```
~/.warpdata/                       # Linux/Android  (Windows: %LOCALAPPDATA%\warpdata)
├── testnet/
│   ├── storage/                   #   default member DB
│   ├── backend1/                  #   --database.dir backend1
│   └── echo/                      #   --database.dir echo
├── warpnet/                       # mainnet (alias: mainnet → warpnet)
└── myownnetwork/
```

Wipe testnet data with `make prune-testnet` (`rm -rf ~/.warpdata/testnet/*`).

### Docker

The **member node cannot run in Docker** — it needs the desktop GUI. The
headless roles do: `deploy/` has docker-compose stacks (relays + moderator +
business), and each role has its own Dockerfile (`Dockerfile.relay`,
`.moderator`, `.business`, `.echo`).

### Metrics (optional)

`docker-compose.metrics.yaml` brings up Prometheus + Grafana; nodes push to the
gateway in `--node.metrics.gateway`.

```bash
docker compose -f docker-compose.metrics.yaml up -d
make prometheus    # http://localhost:9090/targets
make grafana       # http://localhost:3000
```

---

## 11. Configuration reference

All configuration is resolved in [`config/config.go`](../config/config.go) with
[viper](https://github.com/spf13/viper) + [pflag](https://github.com/spf13/pflag).

**Precedence (highest wins):** an explicit **command-line flag** overrides an
**environment variable**, which overrides the built-in **default**. Env-var
names are the flag uppercased with dots → underscores (`node.port` → `NODE_PORT`).

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
| `--node.moderator.modelpath` | `NODE_MODERATOR_MODELPATH` | `…/Llama-Guard-3-1B.Q8_0.gguf` | Path to the GGUF model (moderator role). |
| `--node.server.port` | `NODE_SERVER_PORT` | `4999` | Dashboard HTTP/WS port (business role). |
| `--node.server.password` | `NODE_SERVER_PASSWORD` | *(empty)* | Pre-shared secret that encrypts dashboard WS traffic (business role — required). |
| `--logging.level` | `LOGGING_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `--logging.format` | `LOGGING_FORMAT` | `text` | `text` or `json`. |
| `--database.dir` | `DATABASE_DIR` | `storage` | Subdirectory under `~/.warpdata/<network>/`. |

Built-in bootstrap nodes: **mainnet** uses `207.154.221.44:4001/4002/4003` +
`130.94.88.38:4011`; **testnet** uses `207.154.221.44:4011/4022/4033`.

```bash
LOGGING_LEVEL=debug ./build/bin/warpnet --node.network testnet   # verbose logs
```

---

## 12. How it works under the hood

### Boot sequence (member node)

1. `cmd/node/member/main.go` starts Wails and the Vue UI, registers the
   `warpnet://` deep-link scheme, and binds the `App` struct so JS can call Go.
2. `App` opens BadgerDB, builds the auth/user repos, derives the network **PSK**
   from `(network, version)`, and waits for login.
3. Login (`PRIVATE_POST_LOGIN`) is handled **locally** in `app.go` — it derives
   the Ed25519 keypair and unlocks the DB. Logout tears the node down.
4. `NewMemberNode(...)` builds the libp2p host and wires every subsystem;
   `Start()` registers stream handlers and launches discovery/pubsub/mDNS.

### Core subsystems — and the reason each is there

| Package | Responsibility | Why it exists |
|---|---|---|
| `core/node` | libp2p host; registers handlers behind middleware; `SelfStream` + peer streaming. | The seam where "a local UI call" and "a remote peer request" converge on one handler. |
| `core/stream` | `WarpRoute` type; in-process loopback + peer stream pool. | Lets the same route run locally or across the network unchanged. |
| `core/handler` | One handler per route (50+). | All business logic; the unit a contributor adds. |
| `core/middleware` | Signature auth, logging, unwrap, idempotency. | No server = no trusted caller, so every request is verified and de-duplicated. |
| `core/discovery` | Tracks peers online/offline across DHT/mDNS/gossip/stream. | No directory server, so peers must be found and health-checked continuously. |
| `core/dht` / `core/mdns` | Kademlia routing & bootstrap; LAN discovery. | Decentralized "phone book". |
| `core/pubsub` | GossipSub topics (discovery, notifications). | Broadcast without a broker. |
| `core/relay` | CircuitRelay v2. | Most users are behind NAT; relays let them be reached without a server holding their data. |
| `core/crdt` | Conflict-free counters for stats (likes/retweets/replies). | Counts must converge across nodes with no central tally and survive crashes. |
| `core/warpnet` | Type aliases & protocol constants over libp2p. | One place for the libp2p surface. |

### The wire protocol

Every route is a string: `/<visibility>/<verb>/<entity>[/<sub>]/<version>`.
Examples from [`event/paths.go`](../event/paths.go):

```go
PRIVATE_POST_TWEET            = "/private/post/tweet/0.0.0"
PRIVATE_GET_TIMELINE          = "/private/get/timeline/0.0.0"
PUBLIC_GET_TWEET              = "/public/get/tweet/0.0.0"
PUBLIC_POST_LIKE              = "/public/post/like/0.0.0"
PUBLIC_POST_FOLLOW            = "/public/post/follow/0.0.0"
PUBLIC_POST_REPORT            = "/public/post/report/0.0.0"
```

- **`private/*`** acts on *your own* data and is only processed on the owner’s
  node (timeline, drafts, login, bookmarks…).
- **`public/*`** is the peer-to-peer surface: any node may send it, and the
  receiver verifies the signature before acting (like, follow, reply, fetch…).

Requests travel as an `event.Message` envelope:

```go
type Message struct {
    Body        json.RawMessage // inner event payload, signed byte-for-byte
    MessageId   domain.ID       // UUID — also used for idempotency
    NodeId      domain.ID       // sender peer ID
    Destination string          // the route string
    Timestamp   time.Time
    Version     string
    Signature   string          // base64( ed25519.Sign(privKey, Body) )
}
```

**Why signed envelopes and a public/private split?** With no trusted server, the
*message itself* must prove who sent it: the body is signed before transmission
and the auth middleware verifies it against the sender’s key/peer ID, so a peer
can’t forge actions on your behalf. The visibility prefix encodes intent —
private routes never leave your node; public routes are the only thing other
peers can ask of you.

### Stream routing: self vs peer (why both)

The same handler runs whether the request comes from your own UI or a remote
peer — only the transport differs:

- **`SelfStream`** — an **in-process loopback** stream. The desktop UI routes
  everything (except login/logout) through it, so your own node processes your
  own requests through the *same* handler + middleware stack a peer would hit.
  This is why there’s no separate "local API" to maintain.
- **Peer streaming** (`GenericStream` / the stream pool) — a real libp2p stream
  to another peer. Unreachable peers yield `warpnet.ErrNodeIsOffline`, and
  handlers degrade gracefully (store locally now, let propagation catch up).

### Handlers & middleware

Handlers are **factory functions** that take dependencies as **narrow
interfaces** and return a `warpnet.WarpHandlerFunc`:

```go
type WarpHandlerFunc func(buf []byte, s warpnet.WarpStream) (any, error)
```

They’re registered in the member node’s `setupHandlers` via
`SetStreamHandlers(...)`, each wrapped with the middleware chain: **auth**
(verify signature, enforce size limits), **logging**, **unwrap** (extract the
body), **idempotency** (cache by `MessageId` so a retried request isn’t executed
twice). The interface-injection style exists so handlers are unit-testable
without a live node (`like_test.go`).

### Database layer

Storage is **BadgerDB** (embedded LSM key-value store) behind a **repository
pattern** in `database/`, with hierarchical prefix keys (e.g.
`/LIKES/INCR/<tweetID>`) for cheap range scans/pagination. Counters that must
converge across nodes (likes, retweets, replies) are kept as a **CRDT
PN-counter** in `core/crdt` and gossiped — *because there is no central tally to
be authoritative*. `database/like-repo.go` is the canonical example.

### libp2p networking

- **Transport:** Noise (AEAD-encrypted, authenticated) over TCP, IPv4 + IPv6.
- **Private network (PSK):** every packet is gated by a key derived from
  `(network, version)`. Nodes on a different network *or* an incompatible
  version simply can’t speak — which also lets a version bump force an upgrade.
- **Discovery:** Kademlia DHT (with bootstrap/relay nodes) + mDNS on the LAN.
- **NAT traversal:** CircuitRelay v2 + hole punching.
- **Peerstore:** persisted in BadgerDB so addresses survive restarts.
- **Censorship resistance:** the
  [`libp2p-camouflage-transport`](#libp2p-camouflage-transport) dependency can
  disguise libp2p traffic as ordinary HTTPS; already used by the Android client.

### Identity & security

- **Keypair:** an Ed25519 identity derived deterministically from credentials;
  the private key never leaves the device.
- **Signing:** `security.Sign` / `VerifySignature` (Ed25519 + base64).
- **Codebase hash:** `embedded.go` embeds the entire Go source, and
  `security.GetCodebaseHashHex` hashes it. Used during device pairing so a thin
  client can verify *what code* it’s talking to — provenance without a CA.

### Moderation flow

Opt-in and decentralized — **no global consensus**. A node publishes a report
(`PUBLIC_POST_REPORT`); a **moderator** node runs Llama Guard 3 locally and
returns a verdict (`PUBLIC_POST_MODERATION_RESULT`); the member node applies it
locally. Each network can run its own moderators, or none.

---

## 13. Making changes: a worked example

Adding a feature touches several layers **in lockstep**. It looks like a lot the
first time and becomes mechanical after. The `like` feature is the reference —
read it before writing your own.

### Anatomy of the `like` feature

| Layer | File | What's there |
|---|---|---|
| Route | `event/paths.go` | `PUBLIC_POST_LIKE = "/public/post/like/0.0.0"` |
| DTOs | `event/event.go` | `LikeEvent{TweetId, OwnerId, UserId}`, `LikesCountResponse{Count}` |
| Handler | `core/handler/like.go` | `StreamLikeHandler(...) warpnet.WarpHandlerFunc` |
| Storage | `database/like-repo.go` | `LikeRepo.Like(tweetId, userId)` |
| Wiring | `cmd/node/member/node/member-node.go` | repo built + handler registered in `setupHandlers` |
| Test | `core/handler/like_test.go` | end-to-end handler test |

The handler shows the P2P propagation pattern: **store locally, then forward to
the author’s node**, treating an offline peer as a non-fatal outcome:

```go
num, err := repo.Like(ev.TweetId, ev.OwnerId)      // 1) store my like locally
// ...
likedUser, _ := userRepo.Get(ev.UserId)
if likedUser.NodeId != streamer.NodeInfo().ID.String() {
    _, err = streamer.GenericStream(likedUser.NodeId, event.PUBLIC_POST_LIKE, ev) // 2) forward
    if errors.Is(err, warpnet.ErrNodeIsOffline) {
        return event.LikesCountResponse{Count: num}, nil                          // 3) degrade
    }
}
```

### Checklist: add a new route

1. **Route** — add a constant to `event/paths.go`
   (`/<visibility>/<verb>/<entity>/<version>`).
2. **DTOs** — add request/response structs to `event/event.go`.
3. **Handler** — create `core/handler/<feature>.go`: define narrow interfaces
   for the data you need, write `StreamXHandler(...) warpnet.WarpHandlerFunc`.
4. **Storage** — add repo method(s) under `database/` (follow `like-repo.go`).
5. **Wire it up** — construct the repo and register the handler in
   `cmd/node/member/node/member-node.go` (`setupHandlers` → `SetStreamHandlers`).
6. **Test** — add `core/handler/<feature>_test.go` (copy `like_test.go`).
7. **Clients (if UI-facing):**
   - Desktop/browser: add the path constant + call in
     `frontend/src/service/service.js`, then wire the Vue component (§7).
   - Android: add it to `warpdroid` `ProtocolIds.kt`, add the matching Moshi
     DTO, call it from `WarpnetRepository`, and rebuild the AAR (§8).
8. **House rules** — AGPL header on every new `.go` file (copy from `like.go`,
   incl. the `// SPDX-License-Identifier: AGPL-3.0-or-later` line). Run
   `make tests`, `go vet ./...`, `golangci-lint run`.

> [!TIP]
> There’s a project skill, **`warpnet-add-handler`**, that encodes exactly this
> cross-layer flow if you’re working with the agent tooling.

### Keep the cross-language contract in lockstep

The biggest source of "works on desktop, blank on Android" bugs is **DTO/route
drift**. Go is the source of truth:

- `event/paths.go` ↔ `frontend/src/service/service.js` ↔ `warpdroid`
  `ProtocolIds.kt` — the **same** route strings.
- `domain/*` Go structs ↔ Moshi DTOs — the **same** JSON field names and
  nullability. A mismatch silently deserializes to blank fields.

There is no in-stream version negotiation: bump a route or DTO and every client
must move together. (The `warpnet-debug-stack` skill is built for diagnosing
these mismatches.)

---

## 14. The sister repositories

### `moderation`

**On-device LLM content moderation** — so the network can moderate without a
human admin. A small Go package that loads **Llama Guard 3 (1B, quantized
GGUF)** via a llama.cpp cgo binding and exposes a tiny interface:

```go
type Engine interface {
    Moderate(content string) (isSafe bool, reason string, err error)
    Close()
}
```

`NewLlamaEngine(modelPath, threads)` loads the model deterministically and
classifies text against the Llama Guard taxonomy (`S1…S14`, mapped to reasons in
`prompt.go`). All code is behind `//go:build llama`; `cgo_link.go` adds the
static-link path for the prebuilt archives in `native/`. The `warpnet`
**moderator** node imports it (build with `-tags=llama` and the
`CGO_CXXFLAGS` from §6, plus a GGUF model at `--node.moderator.modelpath`). No
network calls — moderation is fully self-hosted.

### `activity-pub-gw`

**A stateless ActivityPub gateway** bridging Warpnet to Mastodon/the Fediverse,
so Fediverse users can discover, follow, and interact with Warpnet users.

The hard rule (its `CLAUDE.md`): **the gateway is stateless.** The *only* things
on disk are (1) the RSA signing key (`GATEWAY_KEY`, for stable HTTP-signatures)
and (2) the Tailscale Funnel state (`GATEWAY_FUNNEL_DIR`, for a stable
hostname). Everything else — users, posts, followers, the federation set — is
derived at runtime from Warpnet (via the node’s public routes) or from the
Fediverse. *Why stateless?* A bridge that cached the social graph would become a
mini-instance — exactly the centralized "soft center" Warpnet rejects.

- `nodeclient.go` joins the network as a libp2p peer and queries public routes.
- `inbound.go` translates signature-verified ActivityPub activities
  (Follow/Like/Create/Undo) into Warpnet routes; `outbound.go` delivers Warpnet
  posts to Fediverse followers as signed `Create(Note)` activities.
- Deploy via **Tailscale Funnel** (`Dockerfile.gateway`, `TS_AUTHKEY` required);
  see `DEPLOY.md`. Bump the `gatewayVersion` const in `main.go` per commit.

### `libp2p-camouflage-transport`

**A DPI-evading libp2p transport.** "No server to block" only holds if the
*traffic* can’t be fingerprinted and blocked either. In censored networks,
deep-packet-inspection middleboxes fingerprint P2P traffic; this transport makes
it look like ordinary HTTPS:

- **TLS camouflage** — a real browser TLS fingerprint (uTLS:
  Chrome/Firefox/Safari/Edge/iOS/Android/Yandex), tunneling Noise + data inside TLS.
- **TCP fragmentation** — the ClientHello is split into tiny, randomly delayed
  segments so first-segment DPI can’t match a libp2p signature.
- **Active-probing defenses** — SNI/ALPN consistency + handshake timeouts.

Entry point: `NewCamouflageTransport(...)` returning a `transport.Transport`,
configurable via options (`WithBrowserFingerprint`, `WithSNI`,
`WithFragmentSize`, …). **It’s already a dependency of `warpnet`** (see
`go.mod`) and registered as a transport in the Android node
(`warpdroid/node/node.go`); it can be opted into elsewhere the same way.

---

## 15. Testing, versioning & git workflow

### Tests & linting

```bash
make tests          # CGO_ENABLED=0 go test -short -p 8 -v ./...
go vet ./...
golangci-lint run
```

### License headers

Every new `.go` file carries the AGPLv3 header — copy the block from
`core/handler/like.go` (the long notice **plus** the
`// SPDX-License-Identifier: AGPL-3.0-or-later` line).

### Versioning

The `version` file holds a semver patch **bumped on every commit**. On non-`main`
branches a git hook does it for you (and keeps `snap/snapcraft.yaml` in sync);
install once with `make setup-hooks` (`git config core.hooksPath .githooks`).
The pre-push hook tags `v<version>` and pushes to GitHub + the Codeberg mirror.

### Branches & PRs

1. Fork, then branch: `git checkout -b <IssueNumber>/<ShortFeatureName>`.
2. Make your change (AGPL header on new files; tests + vet + lint clean).
3. Commit in **imperative present tense** ("Add bookmark filter handler").
4. Open a PR that **names the route(s)/path(s) you touched** — reviewers map
   changes to layers via the route string.

Good first contributions: issues labeled
[`good first issue`](https://github.com/Warp-net/warpnet/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22),
frontend fixes, better error messages/docs, or running a public relay node.

---

## 16. Troubleshooting & gotchas

| Symptom | Likely cause / fix |
|---|---|
| Second desktop window won’t open | Single-instance lock (`net.warpnet.app`) + exclusive BadgerDB lock. Use headless **echo**/**relay** nodes for a local swarm. |
| Two nodes won’t see each other | Different `--node.network`, or **version mismatch** — the PSK is derived from `(network, version)`. Match both. |
| UI changes don’t show up (desktop) | `wails.json` has `skipfrontend: true` and the binary embeds `frontend/dist`. Run `make frontend` **then** rebuild the binary (or use `wails dev`). See §7. |
| Android shows blank rows/fields, desktop is fine | DTO/route **drift**: Kotlin Moshi DTO or `ProtocolIds.kt` out of sync with Go. Realign and rebuild the AAR. See §8/§13. |
| Business node won’t start | `--node.server.password` is **required** (it’s the WS channel key). See §9. |
| Member node won’t start in Docker | By design — it needs the Wails GUI. Run relay/moderator/business/echo in Docker. |
| Moderator fails to start | Missing GGUF model at `--node.moderator.modelpath`, or built without `-tags=llama` / cgo. |
| `wails build` fails on Linux | Missing GTK/WebKit dev libs — install the `apt` packages in §6 and use `-tags webkit2_41`. |
| Running on macOS | Not supported — builds are unsigned and Gatekeeper blocks them (§5). |
| Want a clean slate | `make prune-testnet` (or delete `~/.warpdata/<network>`). |
| Need to see what’s happening | `LOGGING_LEVEL=debug` (and `--logging.format json` for structured logs). |

---

## 17. Community & getting help

- **Telegram (dev chat):** https://t.me/warpnetdev — the fastest way to find a
  task that fits you.
- **Issues:** https://github.com/Warp-net/warpnet/issues
- **Codeberg mirror:** https://codeberg.org/Warpnet/warpnet
- **Snap Store:** https://snapcraft.io/warpnet

Warpnet is real, working, ambitious — and built and maintained by a single
person (one maintainer, no company, no team). If decentralized systems, libp2p,
or Go are your thing, there is a huge amount of room here to own a meaningful
piece, and your help has outsized impact. Every contribution — code, docs, bug reports, or just
kicking the tyres on testnet — genuinely helps.

**Built with Go, libp2p, Noise, and the conviction that a social network
shouldn’t have an owner.**
