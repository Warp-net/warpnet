# Deploying the fediverse-gateway

A step-by-step runbook for putting one Warpnet user on the Fediverse, fronted by
**Tailscale Funnel** (public HTTPS with no domain and no certificate management).

Deployment is three parts: **A)** one-time Tailscale setup in the browser,
**B)** gathering the Warpnet values, **C)** building and running the container —
then verifying.

## Prerequisites

- **Docker** on the host that will run the gateway.
- A free **Tailscale** account (created in step A1).
- Outbound internet — the gateway joins Warpnet through the network's public
  bootstrap nodes on its own; you don't run or point at a specific node.

The gateway keeps only keys/identity on disk; profiles and the follower graph
live in Warpnet. It is agnostic to node, user, and network.

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

## B. Warpnet settings (all optional)

The gateway joins Warpnet by itself and serves **any** user, so none of these
are required:

| Variable            | When to set it                                               | Default     |
| ------------------- | ----------------------------------------------------------- | ----------- |
| `NODE_NETWORK`      | only for a non-default network                              | `warpnet`   |
| `GATEWAY_NODE_ADDR` | to add an explicit entry peer instead of the bootstrap nodes | (bootstrap) |

> If you do set `GATEWAY_NODE_ADDR`, note that inside a container `127.0.0.1` is
> the container, not the host — use the node's **LAN IP** or `--network host`.

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
  -e TS_AUTHKEY=tskey-auth-xxxxxxxx \      # from A4
  warpnet-gateway
```

The gateway joins Warpnet through the network's bootstrap nodes on its own — no
`GATEWAY_NODE_ADDR` or `NODE_NETWORK` needed (add them only per the table in B).

- With no `GATEWAY_HOST` (the default) the gateway self-hosts via Funnel — you do
  **not** publish any ports; inbound traffic arrives through Tailscale, so the
  container needs only outbound internet.
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
gateway: joined Warpnet; any user is resolvable via the network
gateway: serving public https://warpnet-gw.tailXXXX.ts.net via Tailscale Funnel
```

Any Warpnet user is reachable at `@<warpnet-user-id>@warpnet-gw.tailXXXX.ts.net`.

**D2. (One-time) Disable node key expiry.** At
<https://login.tailscale.com/admin/machines>, on the `warpnet-gw` row open the
`⋯` menu → **Disable key expiry** (otherwise the node asks to re-authenticate
after ~180 days).

**D3. From any Mastodon account**, search `@<warpnet-user-id>@warpnet-gw.tailXXXX.ts.net`:
the profile should resolve, and **Follow** should flip to "Following" (not stay
pending). The gateway logs `inbox: Follow from …` and `accept: Follow accepted …`.

---

## Troubleshooting

| Symptom                                          | Cause → fix                                                                 |
| ------------------------------------------------ | --------------------------------------------------------------------------- |
| Funnel-access error on startup                   | A2 (HTTPS) or A3 (funnel in ACL) not done                                   |
| a login URL is printed and startup hangs         | `TS_AUTHKEY` empty/expired → regenerate (A4)                                |
| `serving the static profile only` in logs        | couldn't reach Warpnet bootstrap → check outbound internet, or set `GATEWAY_NODE_ADDR` (LAN IP / `--network host`) |
| handle doesn't resolve from Mastodon             | Funnel isn't public (check the D1 Funnel line) or you searched the wrong host |
| `signature verification failed` on inbound       | host clock drift → sync NTP                                                 |

---

## If the network is unreachable

If the gateway can't reach any Warpnet bootstrap peer, it logs `serving the
static profile only` and serves an empty source — no users resolve until it can
join. Check the container's outbound internet (or set `GATEWAY_NODE_ADDR`).
Normally it joins automatically and serves any user.

## Without Docker

The same binary runs directly with the same environment variables —
see the "Phase 1 — run the gateway" section of [README.md](README.md).
