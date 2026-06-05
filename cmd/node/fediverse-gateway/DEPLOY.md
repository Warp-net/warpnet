# Deploying the fediverse-gateway

A step-by-step runbook for putting one Warpnet user on the Fediverse, fronted by
**Tailscale Funnel** (public HTTPS with no domain and no certificate management).

Deployment is three parts: **A)** one-time Tailscale setup in the browser,
**B)** gathering the Warpnet values, **C)** building and running the container —
then verifying.

## Prerequisites

- A running **Warpnet node** (the one whose user you want to bridge).
- **Docker** on the host that will run the gateway.
- A free **Tailscale** account (created in step A1).

The gateway keeps only keys/identity on disk; the profile and follower graph live
in Warpnet. It needs outbound internet (for Tailscale) and network reach to the
Warpnet node.

---

## A. Tailscale setup (one-time, in the browser)

Funnel is gated by your tailnet, so a few one-time toggles are required. Nothing
is installed on the host — the gateway binary *is* the Tailscale node (embedded
`tsnet`).

**A1. Account.** Sign in at <https://login.tailscale.com>. This creates your
tailnet with a domain like `tailXXXXX.ts.net`.

**A2. Enable HTTPS certificates.** At <https://login.tailscale.com/admin/dns>
turn on **MagicDNS**, then **Enable HTTPS**. (Tailscale issues and renews the
cert for your `*.ts.net` name automatically; this toggle just permits it.)

**A3. Allow Funnel.** At <https://login.tailscale.com/admin/acls> paste the
policy (the default allow-all plus a `nodeAttrs` grant for Funnel):

```jsonc
{
	"acls": [
		{ "action": "accept", "src": ["*"], "dst": ["*:*"] },
	],
	"nodeAttrs": [
		{ "target": ["autogroup:member"], "attr": ["funnel"] },
	],
}
```

**A4. Auth key.** At <https://login.tailscale.com/admin/settings/keys> click
**Generate auth key**: turn **Reusable** on, leave **Ephemeral** off, save, and
copy the `tskey-auth-...` value — it becomes `TS_AUTHKEY`.

---

## B. Warpnet values to gather

| Variable            | What it is                                                    | Where to find it                          |
| ------------------- | ------------------------------------------------------------- | ----------------------------------------- |
| `GATEWAY_USER`      | the owner's user id on the node; becomes the `@USER@host` handle | your profile / owner id on the node     |
| `GATEWAY_NODE_ADDR` | the node multiaddr `/ip4/<ip>/tcp/<port>/p2p/<peerid>`        | the running node's logs / info            |
| `NODE_NETWORK`      | the Warpnet network name (usually `warpnet`)                  | the node config                           |

> **Important — the IP in `GATEWAY_NODE_ADDR`:** inside a container `127.0.0.1`
> is the container itself, not the host. If the node runs on the same machine,
> use the host's **LAN IP** in the multiaddr, or run the container with
> `--network host`.

---

## C. Build and run (Docker)

**C1. Build the image** (run from the repository root — the build context is the
module root):

```sh
docker build -f Dockerfile.gateway -t warpnet-gateway .
```

**C2. Run it:**

```sh
docker run -d --name warpnet-gw -v warpnet-gw-data:/data \
  -e GATEWAY_FUNNEL=1 \
  -e TS_AUTHKEY=tskey-auth-xxxxxxxx \                                 # from A4
  -e GATEWAY_USER=alice \                                            # from B
  -e GATEWAY_DISPLAY_NAME="Alice on Warpnet" \
  -e GATEWAY_NODE_ADDR=/ip4/192.168.1.50/tcp/4001/p2p/12D3Koo... \   # from B (LAN IP!)
  -e NODE_NETWORK=warpnet \
  warpnet-gateway
```

- With `GATEWAY_FUNNEL=1` you do **not** publish any ports — inbound traffic
  arrives through Tailscale. The container needs only outbound internet plus
  network reach to the node.
- The `/data` volume holds the RSA key, the follower fallback, and the Tailscale
  node identity, so the `*.ts.net` hostname stays stable across restarts.

---

## D. Verify

**D1. Logs:**

```sh
docker logs -f warpnet-gw
```

Look for:

```
gateway: tailscale funnel node up as warpnet-gw.tailXXXX.ts.net
gateway: bridged actor is @alice@warpnet-gw.tailXXXX.ts.net -> https://.../users/alice
gateway: serving public https://warpnet-gw.tailXXXX.ts.net via Tailscale Funnel
```

Your public handle is `@alice@warpnet-gw.tailXXXX.ts.net`.

**D2. (One-time) Disable node key expiry.** At
<https://login.tailscale.com/admin/machines>, on the `warpnet-gw` row open the
`⋯` menu → **Disable key expiry** (otherwise the node asks to re-authenticate
after ~180 days).

**D3. From any Mastodon account**, search `@alice@warpnet-gw.tailXXXX.ts.net`:
the profile should resolve, and **Follow** should flip to "Following" (not stay
pending). The gateway logs `inbox: Follow from …` and `accept: Follow accepted …`.

---

## Troubleshooting

| Symptom                                          | Cause → fix                                                                 |
| ------------------------------------------------ | --------------------------------------------------------------------------- |
| Funnel-access error on startup                   | A2 (HTTPS) or A3 (funnel in ACL) not done                                   |
| a login URL is printed and startup hangs         | `TS_AUTHKEY` empty/expired → regenerate (A4)                                |
| `connect Warpnet node` / timeouts to the node    | wrong `GATEWAY_NODE_ADDR` (`127.0.0.1` ≠ host inside a container) → LAN IP or `--network host` |
| handle doesn't resolve from Mastodon             | Funnel isn't public (check the D1 Funnel line) or you searched the wrong host |
| `signature verification failed` on inbound       | host clock drift → sync NTP                                                 |

---

## Running without a node (quick test)

If `GATEWAY_NODE_ADDR` is omitted, the container still starts in a static-stub
mode: the profile comes from `GATEWAY_DISPLAY_NAME`/`GATEWAY_SUMMARY` and the
follower list is a local file. Good for smoke-testing discovery + Follow, but
posts and inbound interactions are not bridged.

## Without Docker

The same binary runs directly with the same environment variables —
see the "Phase 1 — run the gateway" section of [README.md](README.md).
