# fediverse-gateway (Phase 1 skeleton)

A thin, stateless ActivityPub gateway that makes **one** Warpnet user
discoverable and followable from Mastodon / the Fediverse.

It serves the minimum surface Mastodon's federation path exercises:

- `GET /.well-known/webfinger` — resolves `acct:USER@HOST` → actor URL
- `GET /users/{user}` — the `Person` actor document, with an RSA public key
- `POST /users/{user}/inbox` and `POST /inbox` — verifies the HTTP signature
  and answers inbound `Follow` with a signed `Accept`
- `GET /users/{user}/{outbox,followers,following}` — empty collections
- `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.0`

The gateway stores **no user content**. The only durable secret is the RSA
signing key on disk (Mastodon verifies HTTP signatures against RSA, while
Warpnet node identities are Ed25519).

## Implemented so far

- **Phase 1** — discovery + follow: WebFinger, RSA-keyed actor document, inbox
  with HTTP-signature verification, `Follow` → signed `Accept`.
- **Phase 2 (outbound)** — `Accept` now persists the remote follower
  (`followers.go`, JSON store via `-followers`); the `followers` collection
  reflects it; `publishNote` builds a `Create(Note)` from a Warpnet tweet and
  fans it out (signed) to every follower's inbox.

## Not yet wired

- The **libp2p connector** to a live Warpnet node — reading the real
  user/profile and tweets and triggering the fan-out on new tweets. Until then
  `source.go` returns a single operator-configured user (`-user/-display-name/
  -summary`) and `publishNote` is exercised only by tests / a future trigger.
- Inbound interaction translation (Create/Like/Announce/Undo/Delete → Warpnet)
  — Phase 3.
- The HTTP signature code in `httpsig.go` is a minimal, self-contained Cavage
  implementation. Production should swap it for `superseriousbusiness/httpsig`
  (the library GoToSocial uses) behind the same `signRequest` / `verifyRequest`.

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

```sh
go run ./cmd/node/fediverse-gateway \
  -host my-host.tailXXXX.ts.net \
  -addr 127.0.0.1:8080 \
  -user alice \
  -display-name "Alice on Warpnet" \
  -summary "Bridged from Warpnet"
# the RSA key is created at ./fediverse-gateway-key.pem on first run
```

Flags can also be set via env: `GATEWAY_HOST`, `GATEWAY_ADDR`, `GATEWAY_KEY`,
`GATEWAY_USER`, `GATEWAY_DISPLAY_NAME`, `GATEWAY_SUMMARY`.

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
