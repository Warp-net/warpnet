# Network connections and transport security

What a Warpnet node and the Android app connect to, what is wrapped around
every connection, and who can see what. Written for users, packagers and
reviewers, with pointers to the code behind each statement.

---

## Node roles

| Role | Runs on | On the network |
|---|---|---|
| **member** | your computer or server | your node: stores your data, serves the desktop UI and paired phones |
| **relay** | a host with a public IP | bootstrap entry point to the DHT and libp2p circuit relay v2 for nodes behind NAT; stores nothing on disk |
| **moderator** | a server | judges reports, see [MODERATION.md](MODERATION.md) |

The default bootstrap list (`config/config.go`) is three relay nodes run by the
Warpnet project on one host, `207.154.221.44` (TCP 4001–4003 on mainnet,
4011/4022/4033 on testnet). Start a node with
`--node.bootstrap <multiaddr>,<multiaddr>` (or `NODE_BOOTSTRAP`) to replace the
list entirely; the relay role is the same AGPL code (`cmd/node/relay`), so
anyone can run their own.

## What is wrapped around every connection

All roles and the Android app use one transport,
[libp2p-camouflage-transport](https://github.com/Warp-net/libp2p-camouflage-transport),
and the same stack on top of it:

```
application streams     Warpnet protocols (/private/..., /public/...)
yamux                   stream multiplexing
Noise XX                encryption and peer authentication   <- the security layer
pnet (PSK, XSalsa20)    network/version separator, NOT a secret
TLS camouflage (uTLS)   looks like Chrome -> www.googleapis.com, NOT verified
TCP                     TLS greeting split across two segments
```

### TLS camouflage

Its only job is to get through networks that block unknown protocols with
deep packet inspection (DPI). It is not a security layer.

- The dialing side sends a TLS ClientHello with a Chrome fingerprint (uTLS
  `HelloChrome_Auto`), SNI `www.googleapis.com` and ALPN `h2`/`http/1.1`.
  The write that carries the SNI is split in the middle of the name, so the
  name never sits whole in one TCP segment (`dpi-spoof.go`).
- The listening side (member, relay, moderator) answers with Go's
  `crypto/tls` and a certificate chain it generates at start-up: a leaf for
  `www.googleapis.com` signed by a throwaway CA named like a Cloudflare
  intermediate. Nobody trusts that CA; it only gives the handshake a
  realistic shape. Under TLS 1.3 the certificate is encrypted anyway.
- The dialing side does not verify the certificate (`InsecureSkipVerify`).
  The peer is authenticated by Noise inside the tunnel instead.
- Nothing goes to Google or Cloudflare. The TCP connection goes to the IP
  address of the peer. A traffic monitor such as PCAPdroid therefore shows
  `www.googleapis.com` against the address of your node or of a relay.

### The pre-shared key is public

`libp2p.PrivateNetwork` needs a 32-byte key. Warpnet derives it as
`SHA-256(network name + major version)`, for example `SHA-256("warpnet0")`
(`security/psk.go`). Anyone can compute it; the pairing QR code carries it
only for convenience, and `--node.print-psk` prints it.

Its purpose is separation, not secrecy: a node from another network or from an
incompatible major version fails on the first bytes instead of talking a
protocol it does not understand. Do not count it towards confidentiality.

### Noise

Noise (`Noise_XX_25519_ChaChaPoly_SHA256`, go-libp2p) encrypts and
authenticates every connection with the peers' Ed25519 identity keys. The
dialer knows the peer ID it expects — the Android app takes it from the QR
code — and the handshake fails if the other side cannot prove that identity.
That is what protects content, including on relayed connections. On top of it
the node accepts a request only if it is fresh and signed with the key behind
the connection's peer ID, and serves private routes only to devices registered
at pairing (`core/middleware/auth.go`).

## What a member node does

- Listens on TCP 4001, IPv4 and IPv6.
- Asks your router for a port mapping over UPnP/NAT-PMP (`libp2p.NATPortMap`).
- Learns whether it is reachable from the outside with AutoNAT v2, and answers
  AutoNAT checks for other nodes.
- If it is not reachable, reserves a slot on the bootstrap relays (circuit
  relay v2, `autorelay` with the bootstrap list as static relays) and
  advertises `/p2p-circuit` addresses through them.
- Upgrades relayed connections to other nodes to direct ones with hole
  punching (DCUtR) where the NATs allow it.
- Offers a relay itself once it is publicly reachable (limits in
  `core/relay/relay.go`: 1 hour and 1 GiB per circuit).
- Joins the Kademlia DHT under `/warpnet` (or `/testnet`), announces itself
  over mDNS on the local network, and exchanges public content over gossip.

## What the Android app does

The app runs a dial-only libp2p host (`warpdroid/node/node.go`): no listening
sockets, the same camouflage transport, pnet, Noise and yamux, a circuit relay
client and a DHT client.

1. **Start.** It connects to every bootstrap address your node handed it at
   pairing and refreshes its DHT routing table. That routing-table refresh
   sends DHT queries to the bootstrap nodes and possibly to other publicly
   reachable Warpnet nodes. This happens whether or not your node is on the
   local network.
2. **Reaching your node.** It dials all addresses your node reported:
   private (local network) ones first, public ones 250 ms later, relay ones
   last (libp2p's default ranking holds them back another 500 ms when a
   public address exists). The first connection that completes is used and
   the remaining dials are cancelled.
3. **Relayed session.** When only the relay address answers, the session runs
   phone ↔ relay ↔ node: a stream inside the phone's camouflaged connection to
   the relay, with its own Noise handshake end to end between the phone and
   the node.
   The app does not hole-punch: your node does try DCUtR on the inbound
   relayed connection, but the app has no listen addresses to dial and no
   DCUtR handler, so the node logs `holepunch: protocol error`. A relayed
   session stays relayed until the next reconnect, and the relay limits above
   apply.

The app makes no other network connections: no Google services, no analytics,
no update checks.

## Who sees what

| Observer | Sees | Does not see |
|---|---|---|
| Your ISP, Wi-Fi or mobile network | IP addresses and ports, timing and volume, a TLS ClientHello for `www.googleapis.com` | peer IDs, protocols, content |
| Bootstrap/relay node (DHT traffic) | your IP address, peer ID and public key, the `warpdroid` user agent, DHT queries | your posts, messages or anything from your node |
| Relay node (relayed session) | both IP addresses and peer IDs, timing and volume | content — it forwards Noise ciphertext |
| Your own node | everything the app does | — |

## Keeping the project's hosts out of the path

Run your own relay node (`cmd/node/relay`) on a host with a public IP and start
your member node with `--node.bootstrap` pointing at it. The member node uses
that list for discovery and relaying, and hands the same list to the phone at
pairing, so neither of them contacts `207.154.221.44`. If your relay itself
joins the public network, DHT queries can still reach other public Warpnet
nodes.
