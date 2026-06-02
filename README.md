<div align="center">

# Warpnet

### A social network with no servers, no instances, and no company in the middle.

Warpnet is a fully decentralized, peer-to-peer social network built in Go. Every user runs a node. The node *is* the network. There is no central server to seize, no instance admin to trust, and no relay that can quietly drop you.

[![Go Version](https://img.shields.io/badge/Go-1.26+-brightgreen)](https://golang.org/dl/)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://github.com/Warp-net/warpnet/blob/main/LICENSE.md)
[![Build](https://github.com/Warp-net/warpnet/actions/workflows/build.yaml/badge.svg)](https://github.com/Warp-net/warpnet/actions/workflows/build.yaml)
[![codecov](https://codecov.io/github/Warp-net/warpnet/graph/badge.svg)](https://codecov.io/github/Warp-net/warpnet)
[![Snap](https://snapcraft.io/warpnet/badge.svg)](https://snapcraft.io/warpnet)
[![Telegram](https://img.shields.io/badge/chat-telegram-blue.svg)](https://t.me/warpnetdev)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#-contributing)

[**Install**](#-quick-start-users) · [**Build from source**](#-build-from-source-developers) · [**How it works**](#-how-it-works) · [**Contributing**](#-contributing) · [**Roadmap**](#-roadmap)

<a href="https://github.com/Warp-net/.github/raw/main/docs/warpscreen.jpg">
  <img src="https://github.com/Warp-net/.github/raw/main/docs/warpscreen.jpg" alt="Warpnet screenshot" width="720">
</a>

</div>

---

## Table of Contents

- [Why Warpnet exists](#-why-warpnet-exists)
- [What makes it different](#-what-makes-it-different)
- [Features](#-features)
- [Quick start (users)](#-quick-start-users)
- [Build from source (developers)](#-build-from-source-developers)
- [How it works](#-how-it-works)
- [Project layout](#-project-layout)
- [Contributing](#-contributing)
- [Roadmap](#-roadmap)
- [FAQ](#-faq)
- [Community](#-community)
- [License](#-license)

---

## Why Warpnet exists

Every "decentralized" social network you've heard of still has a soft center.

- **Federated networks** (like Mastodon) still put you on an *instance*. The instance admin holds your data, sets the rules, can defederate from others, and can simply shut the server down. You traded one landlord for thousands of smaller ones.
- **Relay server based networks** (like Nostr) make the client thin and push storage onto *relays* — servers that can rate-limit you, refuse your events, or disappear.
- **"Protocol" networks** (like the atproto ecosystem) decentralize in theory but concentrate around a handful of large providers in practice.

Warpnet takes the harder road on purpose: **there is no server tier at all.** Each participant runs a node that stores its own data locally and talks directly to other nodes. Discovery happens over a DHT, transport is encrypted end-to-end with the Noise protocol, and content spreads by propagating between peers. There is nothing in the middle to capture, subpoena, or switch off.

That's a real engineering challenge — and that's exactly why it's interesting to build.

## What makes it different

A fair comparison, because the alternatives are good projects making different tradeoffs:

| | **Warpnet** | Mastodon | Nostr | Bluesky / atproto |
|---|:---:|:---:|:---:|:---:|
| Architecture | True P2P (libp2p) | Federated instances | Client + relay server | PDS + relays |
| Server required? | **None** | Yes (instance) | Yes (relays) | Yes (providers) |
| Where your data lives | **Your own node** | Instance admin's DB | Relay servers you post to | Your PDS provider |
| Transport security | **Noise protocol** | TLS to instance | TLS to relay | TLS to provider |
| Can an admin deplatform you? | **No admin exists** | Yes (instance admin) | Relay server can drop you | Provider can drop you |
| Discovery | **DHT** | Instance + relay routers | Relay server lists | Relays / indexers |
| Moderation | LLM moderator nodes | Per-instance | Per-client | Labelers |

Warpnet's bet: the only way to be genuinely censorship-resistant is to remove the server entirely, not to multiply it.

## Features

- **Serverless by design** — no instances, no relay servers (just stateless relay routers), no central database. Peers connect directly over [libp2p](https://libp2p.io/).
- **Encrypted everywhere** — inter-node communication runs over the **Noise protocol**.
- **Local-first storage** — your posts, follows, and timeline live in an embedded datastore on *your* machine.
- **Censorship-resistant** — public content propagates peer-to-peer through the DHT; there's no single point that can be blocked.
- **Two first-class clients, one protocol** — a desktop app (Wails + Vue) and an Android app (a [Tusky](https://github.com/tuskyapp/Tusky) fork) both speak the same node protocol.
- **Opt-in decentralized moderation** — dedicated moderator nodes use LLM, so moderation happens without any human intervention.
- **Cross-platform** — install via Snap, or build for your platform from source.
- **Fully open source** — AGPLv3, forever.

## Quick start (users)

The fastest way to join the network. No compiler, no Go toolchain — one command:

```bash
sudo snap install warpnet
warpnet
```

[![Get it from the Snap Store](https://snapcraft.io/static/images/badges/en/snap-store-black.svg)](https://snapcraft.io/warpnet)

On first launch you'll create a local identity (a keypair that stays on your device) and connect to bootstrap nodes for peer discovery. That's it — you're a node.

### Networks

Warpnet runs separate networks. Use the testnet to experiment without affecting real data:

```bash
warpnet --node.network testnet   # safe playground
warpnet --node.network mainnet   # the live network
```

## 🛠 Build from source (developers)

### Prerequisites

- [Go 1.26+](https://go.dev/doc/install)
- [Wails v2.10.2](https://github.com/wailsapp/wails) (for the desktop client)

### Run a member node with the desktop UI

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
  cd cmd/node/member # pick the member node dir
  wails build -devtools -tags webkit2_41 # compile a binary
  ./build/bin/warpnet --node.network testnet # run binary on a testnet
```


> **Heads up:** the `version` file and `snap/snapcraft.yaml` are bumped automatically by the pre-commit hook on non-`main` branches. Run `make setup-hooks` once and let it do its job — don't edit the version by hand.

## How it works

Warpnet has three node roles:

| Role | What it does |
|---|---|
| **relay** | Stable entry points that help new nodes find peers via the DHT. Stateless. Thin. |
| **member** | The full "fat" node most people run — holds local data, serves the UI, and participates in the network. |
| **moderator** | LLM moderation node. |

### One protocol, two clients

Both the desktop UI and the Android app talk to a **member node**, and both hit the *same* canonical handler on the Go side. They only differ in how the request reaches the node:

```
                    ┌─────────────────────────────┐
                    │   desktop "fat" member node │
                    │   cmd/node/member/...       │
                    │   core/handler/<feature>.go │  ← ONE canonical handler per route
                    │   database/<feature>-repo.go│
                    └──────────────▲──────────────┘
                                   │
                       ┌───────────┴────────────┐
                       │                         │
            in-process Wails Call      libp2p stream over Noise
                       │                         │
              Vue desktop app           Android client (Tusky fork)
              frontend/                 warpdroid/
```

## Project layout

A map for new contributors — start here and you won't get lost:

```
warpnet/
├── cmd/node/         # entry points: bootstrap, member, moderator nodes
├── core/             # the heart — handlers, streaming, node wiring
│   └── handler/      # one file per route (start with like.go — cleanest example)
├── database/         # repository layer over the embedded local datastore
├── domain/           # core domain types (IDs, models)
├── event/            # the wire protocol: paths.go (routes) + event.go (DTOs)
├── security/         # keys, signing, Noise/PSK plumbing
├── config/           # node configuration
├── frontend/         # Vue desktop UI (talks to the node via Wails)
├── metrics/          # observability
├── deploy/           # deployment manifests
└── snap/             # Snap packaging
```

If you want to trace a feature end to end, read **`event/paths.go`** (the route), **`core/handler/like.go`** (the handler), and **`database/like-repo.go`** (storage) in that order. `like` is intentionally the cleanest small reference in the codebase.

## Contributing

**This is the part that matters most, and the part where help is most wanted.** Warpnet is real, working, and ambitious — and right now it's carried by a very small team. If decentralized systems, libp2p, or Go are your thing, there's a lot of room here to own a meaningful piece.

### Good places to start

- 🏷️ Browse [issues labeled **`good first issue`**](https://github.com/Warp-net/warpnet/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) — small, self-contained tasks with clear boundaries.
- 🐛 [Report a bug](https://github.com/Warp-net/warpnet/issues/new?labels=bug&template=bug-report---.md) or [request a feature](https://github.com/Warp-net/warpnet/issues/new?labels=enhancement&template=feature-request---.md).
- 📖 Read **[HOW-TO-HELP.md](https://github.com/Warp-net/warpnet/blob/main/HOW-TO-HELP.md)** for the longer guide.
- 💬 Say hi in the [Telegram dev chat](https://t.me/warpnetdev) — the quickest way to find a task that fits you.

### A note on the architecture

Adding a feature usually means touching several layers in lockstep: a path in `event/paths.go`, a DTO in `event/event.go`, a handler in `core/handler/`, a repo method in `database/`, registration in `member-node.go`, plus tests. If the route is meant for the desktop or Android UI, the client side mirrors the same path string. It sounds like a lot, but it's mechanical once you've done it once — and `core/handler/like.go` + `like_test.go` are a complete worked example you can copy.

### Workflow

1. Fork the repo.
2. Create a feature branch: `git checkout -b IssueNumber/AmazingFeature`
3. Make your changes (AGPL header on every new `.go` file — copy it from `like.go`).
4. Run `make tests` and make sure `go vet ./...` / `golangci-lint run` are clean.
5. Commit in imperative present tense and open a PR that names the path(s) you touched.

Every contribution — code, docs, bug reports, or just kicking the tyres on testnet — is genuinely appreciated. And yes: a ⭐ helps more people find the project.

## FAQ

**Is there a server I need to run or pay for?**
No. You run a node; the node is your participation in the network. Relay nodes only help with peer discovery.

**Where is my data stored?**
Locally, on your own device, in an embedded datastore. Public posts propagate to peers; private data never leaves your node.

**How is this different from Mastodon?**
Mastodon is *federated* — you still live on an instance run by an admin. Warpnet has no instances and no admins.

**How is this different from Nostr?**
Nostr stores your events on stateful relay servers. Warpnet has no relay tier — nodes are full peers connected over libp2p.

**Why AGPLv3?**
Because a censorship-resistant network should stay free and open: anyone running a modified version that others interact with must share their changes.

**Is it production-ready?**
It's actively developed and pre-1.0. Use testnet to explore, expect rough edges, and please report what you find.

## Community

- **Telegram:** [warpnetdev](https://t.me/warpnetdev)
- **Codeberg mirror:** [codeberg.org/Warpnet/warpnet](https://codeberg.org/Warpnet/warpnet)
- **Snap Store:** [snapcraft.io/warpnet](https://snapcraft.io/warpnet)

**Maintainer:** Vadim Filin — [github.com.mecdy@passmail.net](mailto:github.com.mecdy@passmail.net)

<a href="https://github.com/Warp-net/warpnet/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=Warp-net/warpnet" alt="Contributors" />
</a>

## License

Warpnet is free software, licensed under the **GNU Affero General Public License v3.0 or later**. See [LICENSE.md](https://github.com/Warp-net/warpnet/blob/main/LICENSE.md) for the full text.

---

<div align="center">

**Built with Go, libp2p, Noise, and the conviction that a social network shouldn't have an owner.**

If you believe that too — [grab a good first issue](https://github.com/Warp-net/warpnet/issues) and join in.

</div>
