# fediverse-gateway

A thin ActivityPub gateway that makes Warpnet users discoverable and followable
from Mastodon / the Fediverse. It is agnostic to node, user, and network: it
joins Warpnet through the network's bootstrap nodes and resolves any requested
user via the public routes.

It serves the minimum surface Mastodon's federation path exercises:

- `GET /.well-known/webfinger` — resolves `acct:USER@HOST` → actor URL
- `GET /users/{user}` — the `Person` actor document, with an RSA public key
- `GET /users/{user}/statuses/{id}` — resolves a Note id back to the Warpnet
  tweet, so peers can dereference, reply to, and boost our posts
- `POST /users/{user}/inbox` and `POST /inbox` — verifies the HTTP signature
  and answers inbound `Follow` with a signed `Accept`
- `GET /users/{user}/{outbox,followers,following}` — empty collections
- `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.0`

The gateway keeps **no Warpnet content** and, when connected to a node, **no
follower state**: on disk it holds only the RSA signing key (Mastodon verifies
HTTP signatures against RSA, while Warpnet identities are Ed25519). The AP
follow graph lives in Warpnet, reusing the existing follow routes (a local JSON
store is used only as a dev fallback when no node is configured).

## Implemented so far

- **Phase 1** — discovery + follow: WebFinger, RSA-keyed actor document, inbox
  with HTTP-signature verification, `Follow` → signed `Accept`.
- **Phase 2 (outbound)** — `publishNote` builds a `Create(Note)` from a Warpnet
  tweet and fans it out (signed) to followers; the `followers` collection is
  served from the live follow graph, and each Note is dereferenceable at
  `/statuses/{id}` (`serveStatus`, via `PUBLIC_GET_TWEET`).
- **libp2p connector** (`nodeclient.go`) — a minimal client peer (same
  PSK/transport/security as a member node) that joins Warpnet through the
  network's bootstrap nodes (no specific node required) and routes requests to
  any entry peer. `nodeSource` resolves *any* requested user's profile via
  `PUBLIC_GET_USER`. `GATEWAY_PROBE=1` smoke-tests the connector.
- **Follower graph in Warpnet** — `Accept` records the remote actor through the
  existing `PUBLIC_POST_FOLLOW` route and fan-out reads `PUBLIC_GET_FOLLOWERS`
  (no new node routes); actor URLs travel as `ap:`-prefixed base64url follower
  ids and the inbox is resolved on demand.
- **Outbound federation** (`outbound.go`, `tweetpoller.go`) — driven by the
  follower graph: when a Warpnet user gains a Fediverse follower (an accepted
  inbound `Follow`), `outboundFederation` starts a poller (`PUBLIC_GET_TWEETS`)
  that federates that user's new original posts via `publishNote`. Never pinned
  to a configured user; history isn't replayed across restarts.
- **Phase 3 inbound** (`inbound.go`) — the inbox translates `Like` →
  `PUBLIC_POST_LIKE`, a reply `Create(Note)` → `PUBLIC_POST_REPLY`, `Announce` →
  `PUBLIC_POST_RETWEET`, and `Undo(Follow|Like|Announce)` →
  `PUBLIC_POST_UNFOLLOW`/`UNLIKE`/`UNRETWEET`, forwarding to the owner's node
  (remote actors as `ap:` ids, shown as `user@host`; tweet/owner recovered from
  our URLs).
- **Media** (`mediaproxy.go`) — outbound `Create(Note)` carries image
  `attachment`s and `/media/{ref}` proxies the bytes from the node
  (`PUBLIC_GET_IMAGE`) so Mastodon can fetch them; the gateway stores nothing.
- **HTTP Signatures** (`httpsig.go`) — signing and verification delegate to
  `superseriousbusiness/httpsig` (the library GoToSocial uses for Mastodon
  interop); the gateway keeps only the policy the library leaves to the caller:
  the minimum signed header set, Date freshness, and digest↔body binding.
- **Outbound follows** (`outbound.go`) — for each federated user, `followPoller`
  polls `PUBLIC_GET_FOLLOWINGS`; when that user follows a Fediverse actor (an
  `ap:`-encoded id) it delivers a signed `Follow` to the actor's inbox, and
  `Undo(Follow)` on unfollow (the first poll seeds a baseline).
- **SSRF-hardened fetches** (`server.go`) — the outbound client re-validates
  every redirect hop and the resolved dial IP (rejecting loopback/private/
  link-local), so attacker-supplied actor/key URLs can't reach internal services.

## Not yet wired

- `Delete` (actual deletion). Inbound `Delete` is acknowledged and logged as a
  stub (`handleDelete`), but the gateway does not perform the deletion: it's the
  owner-only `PRIVATE_DELETE_TWEET` route and the gateway has only `PUBLIC_*`
  access. Deletion is done manually through direct node access. (A public,
  bridged-author-authorized delete route on the node would let the gateway do
  it; the id mapping is solvable statelessly via ids derived from the AP object
  id.)
- Outbound owner interactions beyond Follow (owner liking/boosting/replying to
  Fediverse posts): needs a reverse Warpnet-id → AP-id mapping that the ingest
  hashing doesn't provide — same family as Delete.
- Live end-to-end validation against a node + Mastodon (needs network egress).

## Phase 0 — public HTTPS endpoint without a domain or certificates

Federation is domain-based and HTTPS-only; Mastodon rejects plain HTTP and
self-signed certs. You don't need to buy a domain or manage certs — run a
tunnel that terminates TLS and gives you a **stable** public hostname. A
rotating hostname makes you a "new account" on every restart and breaks
existing followers, so avoid throwaway/quick tunnels for anything but a
one-shot test.

### Tailscale Funnel (recommended: free, stable `*.ts.net`, auto Let's Encrypt)

```sh
# one-time: install Tailscale and sign in
tailscale up

# expose the gateway's local port to the public internet over HTTPS
# (Funnel allows ports 443, 8443, 10000; it forwards to your local 8080)
tailscale funnel --bg 8080
tailscale funnel status      # prints e.g. https://my-host.tailXXXX.ts.net
```

Your public `HOST` is the printed `my-host.tailXXXX.ts.net` (no scheme).
Your federated handle becomes `@USER@my-host.tailXXXX.ts.net`.

#### Embedded funnel (no separate `tailscaled` / CLI)

Set `GATEWAY_FUNNEL=1` and the gateway brings up Funnel itself (embedded
`tsnet`): it joins the tailnet as its own node, derives `GATEWAY_HOST` from the
node name, and serves public HTTPS on `:443` with an auto-provisioned cert — no
`tailscale` CLI or system daemon needed.

```sh
TS_AUTHKEY=tskey-auth-...  \  # tagged, reusable key (else a login URL is logged on first run)
GATEWAY_FUNNEL=1 GATEWAY_USER=alice \
  go run ./cmd/node/fediverse-gateway
```

- The tailnet must have HTTPS certs enabled and grant this node the `funnel`
  node-attribute (ACL `nodeAttrs`), or startup fails with a Funnel-access error.
- `GATEWAY_FUNNEL_HOSTNAME` (default `warpnet-gw`) sets the node name; the
  persisted `GATEWAY_FUNNEL_DIR` (default `fediverse-gateway-tsnet`) keeps that
  name — and your federated handle — stable across restarts.
- `GATEWAY_HOST` / `GATEWAY_ADDR` are ignored in this mode.

Alternatives: ngrok free static domain, or — for production — a cheap/free
domain on Cloudflare with a *named* Cloudflare Tunnel (avoids the shared-domain
blocklisting some instances apply to `*.ts.net` / `*.ngrok-free.app`).

## Phase 1 — run the gateway

Configuration is **environment-only** (importing the libp2p stack pulls in
config.init's pflag parsing, so the gateway must not define a second CLI flag
set — and every Warpnet node is env-configured too):

```sh
GATEWAY_HOST=my-host.tailXXXX.ts.net \
go run ./cmd/node/fediverse-gateway
# RSA key created at ./fediverse-gateway-key.pem on first run
```

Env vars: `GATEWAY_HOST`, `GATEWAY_ADDR` (default `127.0.0.1:8080`),
`GATEWAY_KEY`, `GATEWAY_FOLLOWERS`. The gateway joins Warpnet through the
network's bootstrap nodes automatically (`NODE_NETWORK`, default `warpnet`);
`GATEWAY_NODE_ADDR=/ip4/…/tcp/…/p2p/…` adds an explicit entry peer but isn't
required. There is **no per-user config**: discovery/interactions work for any
user, and a user's posts start federating outbound once they gain a Fediverse
follower. Set `GATEWAY_FUNNEL=1` (+ optional `TS_AUTHKEY`,
`GATEWAY_FUNNEL_HOSTNAME`, `GATEWAY_FUNNEL_DIR`) to self-host the public HTTPS
endpoint via embedded Tailscale Funnel (see Phase 0).

### Smoke-test the connector

```sh
GATEWAY_PROBE=1 GATEWAY_NODE_ADDR=/ip4/…/tcp/…/p2p/… GATEWAY_USER=<id> NODE_NETWORK=<net> \
  go run ./cmd/node/fediverse-gateway
# dials GATEWAY_NODE_ADDR and reads the GATEWAY_USER profile via PUBLIC_GET_USER
```

(Requires outbound network access to that node.)

## Milestone check

From any Mastodon account, search `@USER@HOST` (e.g. `@alice@my-host.tailXXXX.ts.net`):

1. The profile resolves (WebFinger + actor document worked).
2. Click **Follow** — it should flip to "Following", not stay pending
   (the gateway received the `Follow`, verified its signature, and delivered a
   signed `Accept`).

Watch the gateway logs for `inbox: Follow from …` and
`accept: Follow accepted …`.

### Gotchas

- The gateway must see the public `Host` header (most tunnels preserve it). If
  signature verification fails on inbound, check that `Host` reaching the
  gateway equals `HOST`.
- Keep the clock synced (NTP); HTTP signatures are time-sensitive.
- Some instances run authorized-fetch ("secure mode"); the gateway already
  signs outbound GETs to handle this.

## Deploy (Docker)

A full step-by-step runbook (Tailscale setup, values to gather, verification,
troubleshooting) is in [DEPLOY.md](DEPLOY.md). In short:

`Dockerfile.gateway` (repo root) builds a static binary from the vendored tree
(no submodule/cgo needed) and runs it. State persists under `/data`, so mount a
volume to keep the RSA key, followers, and — crucially — the Tailscale node
identity (a stable `*.ts.net` hostname).

```sh
# from the repo root (build context is the module root)
docker build -f Dockerfile.gateway -t warpnet-gateway .

docker run -d --name warpnet-gw -v warpnet-gw-data:/data \
  -e GATEWAY_FUNNEL=1 \
  -e TS_AUTHKEY=tskey-auth-... \
  -e GATEWAY_USER=alice \
  -e GATEWAY_DISPLAY_NAME="Alice on Warpnet" \
  -e GATEWAY_NODE_ADDR=/ip4/…/tcp/…/p2p/… \
  -e NODE_NETWORK=warpnet \
  warpnet-gateway
docker logs -f warpnet-gw   # prints the @user@host handle once Funnel is up
```

With `GATEWAY_FUNNEL=1` the container needs only outbound internet (no published
ports) — Funnel ingress reaches it through Tailscale. To instead front it with
an external tunnel, drop `GATEWAY_FUNNEL`/`TS_AUTHKEY`, add `-e GATEWAY_HOST=…`
and `-p 8080:8080` (default `GATEWAY_ADDR` is `:8080` inside the container —
set `GATEWAY_ADDR=0.0.0.0:8080`).
