# fediverse-gateway

A thin ActivityPub gateway that makes **one** Warpnet user discoverable and
followable from Mastodon / the Fediverse.

It serves the minimum surface Mastodon's federation path exercises:

- `GET /.well-known/webfinger` — resolves `acct:USER@HOST` → actor URL
- `GET /users/{user}` — the `Person` actor document, with an RSA public key
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
  served from the live follow graph.
- **libp2p connector** (`nodeclient.go`) — a minimal client peer (same
  PSK/transport/security as a member node) that dials a Warpnet node and calls
  its routes. `nodeSource` reads the bridged user's profile via
  `PUBLIC_GET_USER`. `GATEWAY_PROBE=1` smoke-tests the connector against
  `GATEWAY_NODE_ADDR`.
- **Follower graph in Warpnet** — `Accept` records the remote actor through the
  existing `PUBLIC_POST_FOLLOW` route and fan-out reads `PUBLIC_GET_FOLLOWERS`
  (no new node routes); actor URLs travel as `ap:`-prefixed base64url follower
  ids and the inbox is resolved on demand. Enabled with `GATEWAY_NODE_ADDR`.
- **Outbound trigger** (`tweetpoller.go`) — polls `PUBLIC_GET_TWEETS` and
  federates the owner's new original posts via `publishNote` (stateless across
  restarts; history isn't replayed).
- **Phase 3 inbound** (`inbound.go`) — the inbox translates `Like` →
  `PUBLIC_POST_LIKE`, a reply `Create(Note)` → `PUBLIC_POST_REPLY`, `Announce` →
  `PUBLIC_POST_RETWEET`, and `Undo(Follow|Like)` → `PUBLIC_POST_UNFOLLOW`/
  `UNLIKE`, forwarding to the owner's node (remote actors as `ap:` ids;
  tweet/owner recovered from our URLs).
- **Media** (`mediaproxy.go`) — outbound `Create(Note)` carries image
  `attachment`s and `/media/{ref}` proxies the bytes from the node
  (`PUBLIC_GET_IMAGE`) so Mastodon can fetch them; the gateway stores nothing.

## Not yet wired

- Inbound `Delete` → delete (needs an AP-object-id → Warpnet-reply-id mapping;
  the Delete activity doesn't carry the reply's root, so it's out of scope for
  the stateless gateway for now).
- The HTTP signature code in `httpsig.go` is a minimal Cavage implementation;
  production should swap it for `superseriousbusiness/httpsig` behind the same
  `signRequest` / `verifyRequest`.
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

Alternatives: ngrok free static domain, or — for production — a cheap/free
domain on Cloudflare with a *named* Cloudflare Tunnel (avoids the shared-domain
blocklisting some instances apply to `*.ts.net` / `*.ngrok-free.app`).

## Phase 1 — run the gateway

Configuration is **environment-only** (importing the libp2p stack pulls in
config.init's pflag parsing, so the gateway must not define a second CLI flag
set — and every Warpnet node is env-configured too):

```sh
GATEWAY_HOST=my-host.tailXXXX.ts.net \
GATEWAY_USER=alice \
GATEWAY_DISPLAY_NAME="Alice on Warpnet" \
go run ./cmd/node/fediverse-gateway
# RSA key created at ./fediverse-gateway-key.pem on first run
```

Env vars: `GATEWAY_HOST`, `GATEWAY_ADDR` (default `127.0.0.1:8080`),
`GATEWAY_KEY`, `GATEWAY_USER`, `GATEWAY_DISPLAY_NAME`, `GATEWAY_SUMMARY`,
`GATEWAY_FOLLOWERS`. Set `GATEWAY_NODE_ADDR=/ip4/…/tcp/…/p2p/…` (+
`NODE_NETWORK`) to source the profile from a live Warpnet node instead of the
static stub.

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
