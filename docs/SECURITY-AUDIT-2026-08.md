# Warpnet — Security Assessment Report

| | |
|---|---|
| **Target** | Warpnet — decentralized P2P social network (Go / libp2p, Vue+Wails desktop, Android) |
| **Repository** | `github.com/Warp-net/warpnet` |
| **Revision audited** | `27ead3ce` (branch `develop`) |
| **Assessment date** | 2026-08-14 |
| **Assessment type** | White-box source code review + automated static analysis + dependency analysis |
| **Classification** | Internal — contains unremediated vulnerability details |

---

## 1. Executive Summary

Warpnet was assessed in a white-box review covering the Go node (networking, cryptography, storage, request handlers), the Wails/Vue desktop client, the Android client, and the build and deployment supply chain.

The codebase shows genuine security engineering effort in many places. All peer links use Noise encryption with no plaintext fallback. Message envelopes are ed25519-signed using canonical length-prefixed signing bytes that resist field-aliasing. The local database is encrypted at rest. The idempotency cache is peer-scoped, bounded, and returns defensive copies. Relay resources are capped against amplification. The media-metadata scheme uses Argon2id correctly with key zeroization. On the client side there is no `v-html` anywhere, no WebView in the Android app, and Android pairing secrets live in Keystore-backed encrypted storage. There is no `InsecureSkipVerify` in first-party code, no committed private key material, and CI already runs `go mod verify` and `govulncheck`.

Against that, the assessment identified **four Critical and eleven High severity issues**. They are not independent defects; they are repeated instances of one architectural gap:

> **Warpnet consistently authenticates *who is speaking* but almost never authorizes *what they are allowed to say*.**

The signature layer is well built and correctly proves that a message was signed by the connecting peer's own key. What is missing everywhere is the next step — checking whether that peer may invoke this route, author this content, or issue this verdict. Because the network's only admission gate (the libp2p PSK) is computed from a hardcoded constant in open-source code, "any peer" means "any host on the Internet."

The four Critical findings:

1. **Every `/private/*` route is reachable by any peer, with no owner check.** An attacker reads the victim's direct messages and notifications and overwrites their profile and settings. A `WarpRoute.IsPrivate()` helper exists in the codebase but is referenced *only by tests* — the authorization gate was designed and never wired up.
2. **Following someone hands anyone impersonating them a remote write primitive.** Gossip payloads on a followed user's topic are re-signed *with the victim's own private key* and executed as a local self-stream on an attacker-chosen route, bypassing the replay gate. One publish compromises a user's entire follower graph.
3. **Content authorship is taken from the request body, unbound to the signing peer.** An attacker sets `UserId` to the victim's ID and the node creates the tweet *inside the victim's account*, then broadcasts it to all the victim's followers as genuinely authored.
4. **The remote dashboard's password gate fails open.** The AES codec falls back to accepting plaintext when decryption fails, the listener binds all interfaces while printing "localhost", and every request is signed with the owner's key — unauthenticated account takeover on hosted nodes.

Two further observations worth the reader's attention:

**A cryptographic inversion.** Argon2id with a 64 MiB memory cost protects a *throwaway, deliberately-brute-forceable* media password, while the *permanent, unrevocable account identity* gets one unsalted round of SHA-256 (WRP-07). The strong primitive is already in-tree and applied to the wrong asset. The same inversion appears on Android, where the device identity key is derived purely from public `android.os.Build` fields (WRP-12).

**An integrity mechanism that cannot work.** The codebase-integrity challenge (`core/challenge/`, `GetCodebaseHashHex`) is unwired dead code, and by design could not defend against a tampered node even if wired: a node self-computes and self-reports its own hash, so a modified node simply reports the clean value (WRP-39). It should not be relied on for any trust decision.

We recommend treating WRP-01 through WRP-04 as blocking for any deployment carrying real user data.

### Findings by severity

| Severity | Count | IDs |
|---|---|---|
| **Critical** | 4 | WRP-01 … WRP-04 |
| **High** | 11 | WRP-05 … WRP-15 |
| **Medium** | 13 | WRP-16 … WRP-28 |
| **Low** | 10 | WRP-29 … WRP-38 |
| **Informational** | 6 | WRP-39 … WRP-44 |

---

## 2. Scope and Methodology

### 2.1 In scope

| Component | Paths |
|---|---|
| Node core | `core/node`, `core/stream`, `core/pubsub`, `core/dht`, `core/relay`, `core/discovery`, `core/mdns`, `core/crdt` |
| Auth & middleware | `core/middleware`, `cmd/node/member/auth` |
| Request handlers | `core/handler/*` (57 files) |
| Cryptography | `security/*` |
| Storage | `database/`, `database/local-store`, `database/datastore` |
| Moderation / consensus | `core/handler/moderation.go`, `cmd/node/moderator/*` |
| Clients | `frontend/src` (Vue/Wails), `warpdroid/` (Android) |
| Supply chain | `.github/workflows`, `Dockerfile.*`, `deploy/`, `docker-compose*`, `snap/`, `go.mod` |
| Update channel | `core/selfupdate` |

### 2.2 Out of scope

Vendored third-party code under `vendor/` was reviewed only for version and known-vulnerability status, not line by line. Generated artifacts (`frontend/dist`, `frontend/wailsjs`) were excluded. No live network testing, fuzzing campaign, or physical-device assessment was performed.

### 2.3 Methodology

The review followed a threat-model-first approach with the primary attacker defined as **an unprivileged peer able to join the Warpnet overlay** — a capability established as trivial early in the assessment (WRP-05).

1. **Reconnaissance** — entry-point mapping from `cmd/node/member/main.go`, protocol route enumeration from `event/paths.go`, trust-boundary identification.
2. **Automated analysis** — `gosec` (128 files, 29,951 LOC), `govulncheck`, `golangci-lint`, plus secret scanning across the working tree and the full git history.
3. **Manual review by domain** — six parallel specialist reviews covering network/transport, input validation and media, secrets and key management, client-side (desktop and Android), supply chain and CI/CD, and the identity/consensus trust model.
4. **Verification** — every Critical and High finding was independently re-verified against source by the lead auditor. Where practical, claims were confirmed by execution rather than inspection (see WRP-04).
5. **Triage** — automated findings were manually reviewed to eliminate false positives. Of the six `gosec` HIGH-confidence findings, five were determined to be false positives; all are documented in Appendix A.

Severity uses CVSS 3.1 qualitative bands adjusted for exploitation context. Because the overlay is joinable by any Internet host, network-reachable issues are scored `AV:N/PR:N`.

---

## 3. Critical Findings

### WRP-01 — Critical — All private routes are reachable by any network peer without authorization

| | |
|---|---|
| **CWE** | CWE-862: Missing Authorization |
| **CVSS 3.1** | 9.1 (`AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N`) |
| **Location** | `core/node/node.go:206-220`, `core/middleware/auth.go:98-104`, `core/stream/routes.go:46` |
| **Status** | Verified by lead auditor; independently reported by three specialist reviews |

**Description**

`SetStreamHandlers` registers *every* handler — public and private alike — on the network-facing libp2p host behind one identical middleware chain:

```go
// core/node/node.go:212
streamHandler := logMw(authMw(unwrapMw(h.Handler)))
n.node.SetStreamHandler(h.Path, streamHandler)
```

`AuthMiddleware` derives the verifying key from the connection's remote peer and checks the signature:

```go
// core/middleware/auth.go:98-99
pubKey := warpnet.FromIDToPubKey(remotePeer)
if err := security.VerifySignature(pubKey, msg.SigningBytes(), msg.Signature); err != nil {
```

This proves only *"you signed with your own key"* — authenticity, not authorization. No check exists that the caller is the node owner or a paired device. Private handlers then act on the local owner unconditionally:

```go
// core/handler/settings.go:60,78 — any peer rewrites the owner's settings
owner := authRepo.GetOwner()
repo.SetNotificationSettings(owner.UserId, ev)

// core/handler/user.go:313-315 — any peer overwrites the owner's profile
userRepo.Update(owner.UserId, ev)
```

The intended control appears to have been designed and then never connected. `WarpRoute.IsPrivate()` is defined at `core/stream/routes.go:46` and, as verified by exhaustive search, is referenced **only from `routes_test.go`**. The `PRIVATE_` prefix is therefore a naming convention describing who *normally* calls a route, not an enforced boundary.

The exposed surface includes `PRIVATE_GET_MESSAGES`, `PRIVATE_GET_CHATS`, `PRIVATE_GET_NOTIFICATIONS`, `PRIVATE_GET_BLOCKS`, `PRIVATE_GET_STATS`, `PRIVATE_POST_USER`, `PRIVATE_POST_TWEET`, `PRIVATE_DELETE_TWEET`, `PRIVATE_POST_GATEWAY_SETTINGS`, and `PRIVATE_POST_PAIR` (registered across `cmd/node/member/node/member-node.go:412-847`).

**Attack scenario**

A node's peer ID and multiaddresses propagate normally through discovery gossip and the DHT. An attacker derives the public PSK (WRP-05), joins the overlay, dials the victim's node, and issues requests signed with their *own* freshly generated key. The middleware accepts every one. The attacker reads the victim's entire private message history and notifications, then overwrites the victim's profile and settings — including, via `PRIVATE_POST_GATEWAY_SETTINGS`, repointing the owner's ActivityPub gateway to an attacker-controlled node, which redirects federated traffic.

**Recommendation**

Enforce authorization centrally in the middleware rather than per handler. Reject any request on a route where `stream.WarpRoute(s.Protocol()).IsPrivate()` is true unless `s.Conn().RemotePeer() == s.Conn().LocalPeer()` (loopback self-stream) or the remote peer is present in the paired-device store. Stop deriving the acting identity from `authRepo.GetOwner()` on network-reachable routes. Add a regression test asserting that a foreign peer is rejected on a representative private route.

---

### WRP-02 — Critical — Gossip payloads are re-signed with the owner's key and executed as privileged self-streams

| | |
|---|---|
| **CWE** | CWE-269: Improper Privilege Management; CWE-345: Insufficient Verification of Data Authenticity |
| **CVSS 3.1** | 9.3 (`AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:N`) |
| **Location** | `core/pubsub/gossip.go:457-486`, `cmd/node/member/pubsub/member-pubsub.go:120-124`, `core/stream/loopback-stream.go:57-63` |
| **Status** | Verified by lead auditor |

**Description**

Subscribing to a user — that is, following them — wires that user's gossip topic directly to `SelfPublish`:

```go
// cmd/node/member/pubsub/member-pubsub.go:120-124
handler := pubsub.TopicHandler{
    TopicName: fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId),
    Handler:   g.pubsub.SelfPublish,
}
```

`SelfPublish` takes the received bytes, reads an **attacker-controlled destination route** from them, re-signs the payload **with the local node's own private key**, and injects it into the local handler chain:

```go
// core/pubsub/gossip.go:457-486
route := stream.WarpRoute(simulatedStreamMessage.Destination)   // attacker-chosen
if route.IsGet() { return nil }                                  // writes pass through
simulatedStreamMessage.Signature = base64.StdEncoding.EncodeToString(
    ed25519.Sign(g.privKey, simulatedStreamMessage.SigningBytes()),   // re-signed as OWNER
)
_, err = g.node.SelfStream(route, data)
```

The self-stream's loopback connection reports the local peer at *both* ends:

```go
// core/stream/loopback-stream.go:57-63
func (c *LoopbackConn) LocalPeer() peer.ID  { return c.stream.localPeerID }
func (c *LoopbackConn) RemotePeer() peer.ID { return c.stream.localPeerID }
```

`AuthMiddleware` therefore verifies the freshly-forged signature against the owner's own key — it passes — **and skips the replay/freshness gate entirely**, because that gate is explicitly exempted for loopback (`core/middleware/auth.go:107`). The payload executes with full owner privilege.

A second equivalent path exists for any subscribed topic whose handler is `nil`: `runListener` falls through to the same `SelfPublish` (`core/pubsub/gossip.go:190-197`), and `PrefollowHandlers` registers exactly such nil handlers (`member-pubsub.go:98-107`).

GossipSub topics are open to any publisher and user IDs are public. No `RegisterTopicValidator` exists anywhere in the tree. libp2p's `StrictSign` authenticates only *some* publishing peer's node key, never the followed user's identity, and the application never compares `msg.ReceivedFrom` against the topic owner.

**Attack scenario**

The attacker joins the overlay, subscribes to `user-update-<aliceId>` for any popular user Alice, and publishes one crafted message whose `Destination` names a privileged write route. **Every node that follows Alice** re-signs that payload with its own owner key and executes it. Reachable targets include `PRIVATE_POST_TWEET` (inject content into every follower's timeline), `PRIVATE_DELETE_TWEET` (mass deletion), `PRIVATE_POST_BLOCK` (force every follower to permanently blocklist an arbitrary node, causing mass network partition), `PRIVATE_POST_GATEWAY_SETTINGS` (mass configuration tampering), and `PUBLIC_POST_MODERATION_RESULT` (mass shadow-ban, see WRP-06).

A single publish compromises Alice's entire follower graph simultaneously. Because the effect scales with follower count and requires no credentials, this is wormable and constitutes a network-wide integrity failure.

**Recommendation**

Never re-sign foreign data with the local key — this is the core defect. Verify the *original* `event.Message.Signature` against the claimed author's public key before acting, and propagate that original signature rather than substituting a local one. Restrict `SelfPublish` to an explicit allowlist of content routes, never privileged `/private/*` writes. Register libp2p pubsub topic validators and reject `user-update-<X>` payloads whose author is not `X`. Remove the nil-handler fallthrough in `runListener` so unhandled topics fail closed.

---

### WRP-03 — Critical — Content authorship is taken from the request body, unbound to the authenticated peer

| | |
|---|---|
| **CWE** | CWE-345: Insufficient Verification of Data Authenticity; CWE-290: Authentication Bypass by Spoofing |
| **CVSS 3.1** | 8.8 (`AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N`) |
| **Location** | `core/handler/tweet.go:156-190`, `core/handler/following.go:124-168`, `core/handler/block.go:73-95`, `core/handler/reaction.go:86-131` |
| **Status** | Verified by lead auditor |

**Description**

Handlers derive the *acting identity* from fields in the attacker-supplied payload rather than from the authenticated peer. In the tweet handler the author is `ev.UserId`, straight from the request body:

```go
// core/handler/tweet.go:156-190
isMyOwnTweet := owner.UserId == ev.UserId
if !isMyOwnTweet && (followRepo == nil || !followRepo.IsFollowing(owner.UserId, ev.UserId)) {
    return event.Accepted, nil
}
tweet, err := tweetRepo.Create(ev.UserId, ev)
...
if isMyOwnTweet { // publish to friends timelines
    broadcaster.PublishUpdateToFollowers(owner.UserId, event.PRIVATE_POST_TWEET, bt)
}
```

The victim's `UserId` is public. An attacker who sets `ev.UserId` to it makes `isMyOwnTweet` true, so the node creates the tweet **inside the victim's own account** and then actively **broadcasts it to all of the victim's followers** as genuinely authored content.

The same body-trusts-actor pattern recurs across the social graph. `following.go:124-168` uses `ev.FollowerId` from the body — `s.Conn().RemotePeer()` is consulted only to *fetch* the follower's profile (line 92), never to authorize — permitting forged follow edges and forged "X started following you" notifications. `block.go:73-95` trusts `ev.BlockerId` and then calls `escalateToPeerBlocklist`, which permanently blocklists the target's node. `reaction.go:86-131` trusts both `ev.OwnerId` (actor) and `ev.UserId` (recipient), emitting notifications with attacker-chosen `ActorId` and `RecepientId`.

**Attack scenario**

A single direct stream to the victim's node at `/private/post/tweet/0.0.0` carrying `UserId = <victim's userId>` publishes attacker-chosen text into the victim's account and pushes it to every follower as authentic. This is complete authorship impersonation against the product's core guarantee, and it does not require the gossip path of WRP-02 — a direct dial suffices.

**Recommendation**

Derive the acting identity from the authenticated peer, never from the payload. Bind `ev.UserId`, `OwnerId`, `BlockerId`, and `FollowerId` to the peer ID that signed the stream — or to the owner when the stream is loopback — and reject any mismatch. For gossip-delivered events, bind the actor to the topic owner. This check belongs in one shared helper invoked by every mutating handler, so that new handlers inherit it by default rather than having to remember it.

---

### WRP-04 — Critical — Remote dashboard `/ws` executes any route as the owner, with a bypassable password gate on `0.0.0.0`

| | |
|---|---|
| **CWE** | CWE-287: Improper Authentication; CWE-306: Missing Authentication for Critical Function |
| **CVSS 3.1** | 9.0 (`AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H`) |
| **Location** | `security/aes.go:160-168`, `cmd/node/member/remote/bridge.go:58-68,236-253`, `cmd/node/member/remote-member.go:123,131` |
| **Affects** | `remote` build tag only (Docker/server deployments). The Wails desktop application is **not** affected. |
| **Status** | **Verified empirically** with an executable proof-of-concept |

**Description**

Three defects compose into unauthenticated owner impersonation.

**(a) The password gate fails open.** The AES codec returns the raw frame as plaintext whenever decryption fails:

```go
// security/aes.go:160-168
func (c AESCodec) Decode(frame []byte) (plain []byte, encrypted bool) {
    if len(c.Key) == 0 { return frame, false }
    if p, err := aesGCMDecrypt(c.Key, frame); err == nil { return p, true }
    return frame, false      // <-- attacker plaintext accepted
}
```

The bridge consumes that result and dispatches regardless of the `encrypted` flag (`bridge.go:155-175`). A client that simply never encrypts is fully accepted, so `NODE_SERVER_PASSWORD` provides no access control at all.

**(b) Every dispatched request is signed with the owner's private key**, with no per-connection session check:

```go
// cmd/node/member/remote/bridge.go:247-248
req.Signature = security.Sign(b.auth.PrivateKey(), req.SigningBytes())
respData, err := n.SelfStream(stream.WarpRoute(req.Destination), req)
```

Auth state is process-global, so once the legitimate operator has logged in — the steady state for a long-running server — every connection inherits that authority.

**(c) The listener binds all interfaces while claiming otherwise:**

```go
// cmd/node/member/remote-member.go:123,131
srv := &http.Server{Addr: ":" + port, ...}   // 0.0.0.0
fmt.Printf("\033[1mNODE IS LISTENING ON 'localhost%s'. ...", srv.Addr)
```

The misleading banner actively encourages operators to assume loopback binding. `Dockerfile.remote:27` exposes port 4999 and `docker-compose.yaml:7` uses `network_mode: host`. The `sameOrigin` CSWSH guard returns `true` when the `Origin` header is absent (`bridge.go:58-62`), so any non-browser client passes it unconditionally.

**Proof of concept**

The plaintext bypass was confirmed by execution, not inspection:

```go
codec := AESCodec{Key: AESKeyFromPassword("correct-horse-battery-staple")}
attacker := []byte(`{"message_id":"1","path":"/private/get/user","body":{}}`)
plain, encrypted := codec.Decode(attacker)
// Result: encrypted=false, plain == attacker (passed through unmodified)
```

**Attack scenario**

Any host that can reach port 4999 on a hosted Warpnet node opens a WebSocket without an `Origin` header, sends plaintext JSON naming any route, and the node executes it signed as the owner — reading direct messages, posting as the owner, or permanently pairing the attacker's own device via `PRIVATE_POST_PAIR`. No password and no credentials are required.

**Recommendation**

Fail closed: when a key is configured, reject any frame that does not decrypt rather than falling back to plaintext. Bind to `127.0.0.1` by default and require explicit opt-in for other interfaces. Correct the startup banner to print the actual bind address. Introduce a per-connection bearer token established at login and require it before `call()` will dispatch, so authority is scoped per connection rather than per process.

---

## 4. High Severity Findings

### WRP-05 — High — The "private network" PSK is derived entirely from public constants

**CWE-798** · `security/psk.go:109-145`, `cmd/node/member/node/member-node.go:147`

The libp2p private-network key is a hash of the network name, the major version, and a constant compiled into every binary:

```go
const spbFounding = -((int64(133129) << 16) + 51200)   // security/psk.go:109
entropy := generateAnchoredEntropy()                    // sha256 applied 10x to that constant
seed := append([]byte(network), []byte(majorStr)...)
seed = append(seed, entropy...)
return ConvertToSHA256(seed), nil
```

Every input is public, so any reader of this AGPL repository computes the identical PSK for any network and version.

**Impact.** `libp2p.PrivateNetwork(psk)` provides **no admission control whatsoever** — it partitions networks and versions, nothing more. No connection gater or peer allowlist exists anywhere in `core/` or `cmd/` (verified by exhaustive search); the only allow/deny logic is an application blocklist consulted *after* the connection is established (`core/discovery/discovery.go:254`). This finding is what elevates WRP-01, WRP-02, WRP-03, and WRP-06 from "any authorized member" to "any host on the Internet," and is why those are scored `AV:N/PR:N`.

**Recommendation.** Document the PSK explicitly as a network-partitioning value and never treat it as a security boundary — the immediate risk is that product documentation and UX imply a privacy guarantee that does not exist. Make all authorization decisions independent of PSK possession. If genuine admission control is wanted, derive the PSK from an operator secret distributed out of band and add a `ConnectionGater` enforcing an allowlist and per-IP caps at `InterceptSecured`.

---

### WRP-06 — High — Moderation verdicts are forgeable by any peer; no trusted-moderator root exists

**CWE-862, CWE-347** · `core/handler/moderation.go:103-116`, `event/event.go:686-695`, `cmd/node/moderator/round/round.go:55`

The verdict handler recovers the verifying key *from the self-declared moderator ID in the attacker's own payload*, then checks the signature against that key:

```go
moderatorPeer := warpnet.FromStringToPeerID(ev.ModeratorID)
pubKey := warpnet.FromIDToPubKey(moderatorPeer)
if err := ev.Verify(pubKey); err != nil { ... }
```

This verifies only *internal self-consistency* — "whoever claims to be the moderator signed this with their own key" — which any attacker with a fresh keypair satisfies trivially. Exhaustive search confirms **no allowlist, trust root, or pinned set of moderator public keys exists anywhere in the codebase**. The `Voters` quorum is documented as informational; receivers verify only the chair's signature.

The moderator fleet's own consensus is separately weak: the quorum target is three distinct node IDs (`round/round.go:55`) where node identity is a free ed25519 keypair with no stake or proof-of-work cost. That machinery is moot for consumers in any case, since members apply verdicts arriving directly on the followers topic.

The in-code comment at `moderation.go:99-102` shows the authors reasoned carefully about *authenticity* here, correctly rejecting the connection-peer approach because verdicts arrive via pubsub. The gap is that self-consistent authenticity was mistaken for authorization.

**Impact.** An attacker signs a `FAIL` verdict against any user or tweet and publishes it. Every recipient shadow-bans the target — hiding their bio, display name, and content, and dropping their posts from timelines. This is an unauthenticated, zero-cost, network-wide censorship and takedown primitive usable against any user.

**Recommendation.** Ship a version-pinned allowlist of trusted moderator public keys compiled into the binary — this is the missing trust root — and reject verdicts whose `ModeratorID` is absent from it. For stronger guarantees require an m-of-n threshold of allowlisted signatures over the `Voters` set. This fix is only meaningful alongside WRP-08, since predictable moderator seeds would let an attacker reconstruct an allowlisted key.

---

### WRP-07 — High — Identity private key and database encryption key derive from one unsalted SHA-256

**CWE-916, CWE-759** · `database/auth-repo.go:106-126`, `security/pk.go:46-60`, `database/local-store/db.go:254-255`

Both of the account's root secrets derive from the password with a single fast hash and **no salt**. The ed25519 identity key:

```go
// database/auth-repo.go:115-120
pkSeed := base64.StdEncoding.EncodeToString(
    security.ConvertToSHA256(
        []byte(username + "@" + password + "@" + repo.network + strings.Repeat("@", len(password))),
    ),
)
privateKey, err := security.GenerateKeyFromSeed([]byte(pkSeed))
```

`GenerateKeyFromSeed` (`security/pk.go:50-59`) applies one further SHA-256 and derives the key deterministically. The apparent "salt" — `strings.Repeat("@", len(password))` — is fully derivable and contributes nothing. The BadgerDB at-rest encryption key is weaker still:

```go
// database/local-store/db.go:254
hashSum := security.ConvertToSHA256([]byte(username + "@" + password))
execOpts := db.badgerOpts.WithEncryptionKey(hashSum)
```

This is a brain wallet. The corresponding public key **is the libp2p peer ID**, broadcast throughout the network by design, and the username is a public profile field (`domain/warpnet.go:199,233,366`). The attacker therefore holds both the verification oracle and the entire "salt." Each guess costs roughly two SHA-256 operations plus an ed25519 keygen — hundreds of millions per second on commodity GPUs — against a policy floor of 8 characters (`cmd/node/member/auth/auth.go:246-275`). The same weakness exposes any stolen database directory (laptop theft, backup exfiltration, a hosted node's Docker volume) to offline cracking.

**Impact.** Recovering the password yields the victim's permanent network identity: signing posts and direct messages as them, impersonating their node, and decrypting their database. Because the identity *is* the key, **compromise is unrevocable** — there is no rotation path short of abandoning the account.

**Note on inconsistency.** `security/aes.go:70-77` already implements Argon2id at 64 MiB with correct key zeroization, but applies it to a deliberately weak single-use media password. The strong KDF exists in-tree and is applied to the low-value secret while the permanent identity gets bare SHA-256.

**Recommendation.** Interpose a memory-hard KDF with a random, locally-persisted salt between the password and both derived keys, reusing the existing `deriveKey` helper. The preferred design generates a random ed25519 identity at enrollment and wraps it under an Argon2id-derived key-encryption key rather than deriving the identity from the password at all — this also restores a rotation path. Migration requires a re-enrollment or re-wrap flow for existing accounts.

---

### WRP-08 — High — Predictable and committed node seeds allow infrastructure impersonation

**CWE-798, CWE-330** · `config/config.go:114-117`, `cmd/node/relay/main.go:80-85`, `cmd/node/moderator/main.go:76-77`, `deploy/docker-compose-testnet.yml`

Relay, moderator, and bootstrap identities derive from a `NODE_SEED` string via `GenerateKeyFromSeed`. The default seed is built entirely from public values:

```go
// config/config.go:114-117
seed = "seed" + network + dbDir + host + port
```

Concrete seed values are committed to the repository (`NODE_SEED=echo-testnet` active in `deploy/docker-compose-testnet.yml`, with `warpnet1/2/3` and `warpnet-moderator-*` in adjacent comments). Bootstrap peers are pinned at fixed addresses in `config/config.go:49-59`.

**Impact.** Anyone reading the public repository, or guessing the trivially-structured default, recomputes the private key of a relay, bootstrap, or moderator node. That permits impersonation of pinned infrastructure — enabling eclipse or man-in-the-middle positioning against the bootstrap set — and, combined with WRP-06, signing moderation verdicts as a node other participants recognize.

**Recommendation.** Generate infrastructure identities randomly once, store the private key as a deployment secret, and inject it from a secret store. Never derive an infrastructure identity from a string that is committed, defaulted, or built from public values. **Rotate all currently-deployed relay, moderator, and bootstrap identities**, since their seeds are already public.

---

### WRP-09 — High — Self-update verifies a checksum from the same trust domain as the artifact

**CWE-494** · `core/selfupdate/selfupdate.go:273-311`, `core/selfupdate/github.go:143-169`, `cmd/node/relay/main.go:114-118`, `config/config.go:72`

The updater fetches an archive and a SHA-256 listing from a GitHub release and compares them. Both come from the *same* release assets, so an actor able to replace the binary can replace the checksum identically. There is no code signature over either. Self-update defaults to enabled and relays check hourly, installing and restarting automatically.

**Impact.** Compromise of any single release-publishing credential — a maintainer token, an account, or the release workflow's `contents: write` token — yields silent remote code execution on every auto-updating relay within an hour. Relays are the network's bootstrap tier, making this a network-wide integrity risk. Downgrade attacks *are* correctly prevented (`selfupdate.go:226`), so the gap is tampering, not rollback.

**Recommendation.** Sign release artifacts with an offline project key and verify that signature in `stage()` before installation, embedding the public key in the binary. The project already depends on ed25519 and could reuse it to sign the release manifest. Treat TLS to GitHub as defense in depth, not as the integrity control.

---

### WRP-10 — High — Unclamped `limit` on public list routes causes remote memory exhaustion

**CWE-770, CWE-789** · `database/local-store/db.go:719,747`, `database/user-repo.go:485,557-558`

An attacker-controlled `limit` is used directly as a slice capacity with no ceiling:

```go
// database/local-store/db.go:719 and :747
items := make([]ListItem, 0, *limit)
items := make([]string, 0, *limit)
```

Reachable from public routes that forward `ev.Limit` unclamped: `PUBLIC_GET_USERS` (`user.go:218`), `PUBLIC_GET_USERS_SEARCH` (`user.go:194`), `PUBLIC_GET_FOLLOWERS` and `PUBLIC_GET_FOLLOWINGS` (`following.go:357,371`), and `PUBLIC_GET_WHOTOFOLLOW` (`who-to-follow.go:42`).

**Impact.** A single small, validly-signed message — `{"user_id":"<victim>","limit":5000000000}` — triggers an ~80 GB allocation and the node is killed by the OS. Handler panics are recovered per stream (`logging.go:41`), but a SIGKILL from the OOM killer is not recoverable. Choosing a value in the 10^8–10^10 band produces a real allocation rather than the recoverable "cap out of range" panic.

**Recommendation.** Clamp `limit` to a hard maximum at the database boundary in `list`/`ListKeys` before the `make`, independent of any handler-level validation. The underlying iterator already stops at real data, so the pre-allocated capacity provides no benefit and can simply be a small constant.

---

### WRP-11 — High — Unbounded poll `optionsNum` causes remote memory exhaustion

**CWE-770** · `database/poll-repo.go:180`, `core/handler/poll.go:90,123,150`

```go
// database/poll-repo.go:180 — only guard is optionsNum <= 0
votes = make([]uint64, optionsNum)
for i := range votes { votes[i], err = repo.optionVotes(tweetId, i) ... }
```

`optionsNum` arrives as `ev.OptionsNum` from `PUBLIC_GET_POLL` and `PUBLIC_POST_POLL_VOTE`. It is overridden only when the node already holds the poll definition locally; otherwise the attacker's value is used directly.

**Impact.** `{"user_id":"<this node's owner>","tweet_id":"x","options_num":2000000000}` allocates roughly 16 GB and drives a two-billion-iteration loop of database reads. Unauthenticated remote denial of service.

**Recommendation.** Bound `optionsNum` to the poll option ceiling already enforced elsewhere in the domain layer before calling `Results`, and reject requests exceeding it.

---

### WRP-12 — High — Android device identity key is derived only from public, non-secret inputs

**CWE-330, CWE-340** · `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/Ed25519IdentityStore.kt:31-67`, consumed at `warpdroid/app/.../pairing/PairingCoordinator.kt:59`

The 64-byte ed25519 libp2p key that authenticates the mobile device to its paired node is `SHA256(deviceMaterial | memberPeerId)`, where `deviceMaterial` consists solely of public `android.os.Build` fields (`BRAND`, `MANUFACTURER`, `MODEL`, `DEVICE`, `BOARD`, `HARDWARE`, `PRODUCT`, `FINGERPRINT`, `ID`) and `memberPeerId` is the fat node's **public** peer ID. There is no random seed, no Keystore key, no `ANDROID_ID`, and no per-install salt.

None of the `Build` fields are device-unique — they identify a firmware build and are identical across every unit of a given model and build, and any installed app can read them without a permission.

**Impact.** An attacker who knows the victim's phone model and build (feasible for a targeted victim, or brute-forceable across common `Build.FINGERPRINT` values) and the node's public peer ID reconstructs the exact private key and the identical peer ID, then signs envelopes indistinguishable from the victim's device. Combined with WRP-01 (no per-peer authorization) this yields device impersonation against the paired node.

**Recommendation.** Generate a 32-byte seed from `SecureRandom` once, persist it in the Keystore-backed `EncryptedSharedPreferences` the app already uses for the pairing QR, and derive the identity from that secret seed — optionally still mixed with `memberPeerId` for per-node rotation. Never derive a private key solely from `Build` fields.

---

### WRP-13 — High — Pre-authentication 50 MiB buffering with no read deadline

**CWE-770, CWE-400** · `core/middleware/middleware.go:70`, `core/middleware/auth.go:62-64`, `core/stream/stream.go:251-258`

Every inbound stream — regardless of route, and **before** the signature is verified at line 99 — buffers up to 50 MiB into the heap:

```go
reader := io.LimitReader(s, limit+1)     // MaxLimit = units.MiB * 50
data, err := io.ReadAll(reader)
```

No read deadline is ever set on inbound remote streams, so a peer that trickles a few bytes and stalls holds the reading goroutine indefinitely. This heap growth is not tracked by libp2p's resource-manager memory scopes. Separately, **outbound** response reads are entirely unbounded:

```go
// core/stream/stream.go:251-258
buf := bytes.NewBuffer(nil)
_, err = buf.ReadFrom(rw)      // no limit, no deadline
```

That path is reached automatically during discovery, so a malicious peer this node dials can return a multi-gigabyte or slow-drip response with no user interaction.

**Recommendation.** Set a short read deadline on inbound streams before reading. Size limits per route — JSON control messages need kilobytes, not 50 MiB — reserving a large ceiling only for media. Wrap outbound response reads in `io.LimitReader` with a per-route cap and a deadline.

---

### WRP-14 — High — Snap release workflow grants `write-all` while running an unpinned third-party action

**CWE-1395, CWE-732** · `.github/workflows/snap.yml:13,21,30`

The workflow sets `permissions: write-all`, holds `SNAPCRAFT_STORE_CREDENTIALS`, and runs `canonical/setup-lxd@main` — a third-party action pinned to a **mutable branch**.

**Impact.** A compromise of that action's `main` branch executes attacker code in a job holding both a write-everything repository token and Snap Store publishing credentials, enabling malicious commits, poisoned releases, and a backdoored Snap package published to users. The maintainer-only trigger bounds who starts a run but does nothing to bound the third-party code.

**Recommendation.** Reduce to `contents: read`, pin `canonical/setup-lxd` to a full commit SHA, and scope store credentials to the publishing step alone.

---

### WRP-15 — High — Unauthenticated peer-address injection forces outbound dials to attacker-chosen hosts

**CWE-918** · `core/pubsub/gossip.go:618-641`, `core/discovery/discovery.go:209-246,260-262,283-303`

The discovery topic handler trusts advertised `AddrInfo` structures from unauthenticated gossip and passes each into the connect pipeline, which dials them. Addresses are not filtered against private, loopback, or link-local ranges beforehand, even though a suitable helper (`IsPublicMultiAddress`) already exists at `core/warpnet/warpnet.go:432-457`. Relatedly, discovery caches aliases from a peer's *self-reported* `NodeInfo` and later grants `SetMaxNodePriority` to anything matching that cache, letting a peer self-claim priority.

**Impact.** An attacker causes arbitrary nodes to initiate connections to chosen IP:port pairs, including RFC1918 and loopback addresses, turning the network into a distributed internal port scanner whose results are observable via timing, logs, and metrics. The rate limiter (capacity 32, 2 per 10s) bounds frequency but not target selection.

**Recommendation.** Filter every address learned from untrusted gossip through `IsPublicMultiAddress` before dialing, and authenticate alias claims against the paired-device store before granting priority.

---

## 5. Medium Severity Findings

**WRP-16 — No application-layer rate limiting on content creation.** `core/handler/*`, `core/middleware/`. Handlers validate shape only (280-rune tweets, poll bounds); there is no per-peer or per-user throttle anywhere. The only limits are the 50 MiB payload cap and an idempotency cache keyed by message ID, neither of which throttles distinct requests. Combined with WRP-01 and WRP-03 this permits unbounded forged tweets, reactions, follows, and notifications. **Fix:** token-bucket limits per authenticated peer and per acting user, applied in middleware before dispatch.

**WRP-17 — Report channel enables targeted takedowns and moderator resource exhaustion.** `core/handler/report.go:56-118`, `cmd/node/moderator/moderator.go:204-233`. Any user may report any target; validation covers only reason length and type. Each report opens a vote round involving a fetch plus LLM inference. Flooding `ReportsTopic` forces unbounded inference rounds across the moderator fleet. Reporter identity *is* correctly stamped by the publisher rather than taken from the body. **Fix:** rate-limit reports per reporter identity, deduplicate and threshold before opening rounds, and bound concurrent rounds.

**WRP-18 — No rate limiting or lockout on login.** `cmd/node/member/auth/auth.go:103-120`, `database/local-store/db.go:245-260`. No attempt counting, backoff, or lockout; `ErrWrongPassword` returns as fast as Badger can attempt a key. Combined with WRP-04's `0.0.0.0` bind and plaintext-accepting codec, this permits unlimited automated guessing against a network-reachable node. **Fix:** per-connection and per-IP failed-attempt backoff; keep the endpoint loopback-only.

**WRP-19 — Social blocks are not enforced at the connection layer.** `database/node-repo.go:769-807`, `core/discovery/discovery.go:254`. `BlocklistPermanent` is written to the node repository, but with no `ConnectionGater` installed, `IsBlocklisted` is consulted only during discovery and for tweet-level content. A blocked peer can still dial the node and open streams directly, including the WRP-01 and WRP-03 surfaces — so "blocking" an abuser does not actually stop them. **Fix:** install a `ConnectionGater` consulting `IsBlocklisted` in `InterceptSecured`/`InterceptAddrDial`, and drop existing connections on block.

**WRP-20 — Image decompression bomb: no dimension bound before decode.** `core/handler/image.go:261-265`, reached from `core/handler/import.go:101`. Only the *compressed* input is bounded (50 MiB); the decoded pixel buffer is not, so a ~1 KB PNG declaring huge dimensions allocates `width*height*4` bytes. Reachable via the paired device and Twitter-archive import rather than by arbitrary peers, which caps severity. Foreign images fetched over `PUBLIC_GET_IMAGE` are stored as opaque base64 and never decoded — correctly avoiding a remote decode path. **Fix:** call `image.DecodeConfig` first and reject images above a pixel ceiling before `image.Decode`.

**WRP-21 — Hardcoded dashboard password committed.** `docker-compose.yaml:14` contains `NODE_SERVER_PASSWORD=MySecretPassword9000$`. The `deploy/*.yml` files correctly use `${NODE_SERVER_PASSWORD}` interpolation, making this root file — the one most likely to be copied as a starting point — the outlier. **Fix:** use interpolation, rotate the value, add secret scanning to CI.

**WRP-22 — Dashboard channel key is an unsalted SHA-256 of the password.** `security/aes.go:153-156`, used at `cmd/node/member/remote-member.go:113`. No salt or stretching, so a weak password is trivially recovered from captured ciphertext. **Fix:** Argon2id with a salt, or a negotiated random session key.

**WRP-23 — Every member node runs an open relay.** `core/node/options.go:62` enables `EnableRelayService` unconditionally. Any peer can reserve and relay traffic through any publicly reachable member node, consuming bandwidth and fronting traffic with the operator's IP — an abuse-attribution risk. Per-circuit caps at `core/relay/relay.go:67-86` do correctly prevent high-ratio amplification. **Fix:** enable the relay service only on designated relay nodes.

**WRP-24 — Containers run as root with host networking.** No `USER` directive in `Dockerfile.remote` or `Dockerfile.moderator`; `Dockerfile.relay`/`Dockerfile.echo` use `distroless/static-debian12` rather than the `:nonroot` variant. All compose files use `network_mode: host`. **Fix:** non-root `USER`, `:nonroot` images, explicit published ports, `cap_drop: [ALL]`, `no-new-privileges`.

**WRP-25 — Unauthenticated monitoring services on the host network.** `docker-compose.metrics.yaml:12-20` runs Grafana host-networked with no `GF_SECURITY_ADMIN_PASSWORD` (default `admin/admin`); `deploy/docker-compose-testnet.yml:141-150` exposes a Prometheus pushgateway on `:4091` with no authentication, permitting anonymous metric read and injection. **Fix:** set a strong Grafana password, bind monitoring to localhost or a private network, restrict the pushgateway.

**WRP-26 — Third-party GitHub Actions pinned to mutable tags.** `softprops/action-gh-release@v2`, `codecov/codecov-action@v5`, `docker/*@v3/@v6`, `gradle/actions/setup-gradle@v4`, and notably `golangci/golangci-lint-action@v8` **with `version: latest`** (`tests-static-check.yaml:30-33`), in jobs carrying `contents: write` and `packages: write`. **Fix:** pin to full commit SHAs; pin the linter version.

**WRP-27 — Session token entropy derives from the password rather than a CSPRNG.** `database/auth-repo.go:106-113`. The seed is `username@password@network@randChar@time.Now().String()`, where `randChar` contributes only ~7 bits. Token secrecy rests on the password and a timestamp rather than on random bytes. Not independently exploitable, but the near-zero random contribution is misleading. **Fix:** generate from 32 bytes of `crypto/rand`.

**WRP-28 — Predictable RNG for adversarial moderator audit sampling.** `cmd/node/moderator/moderator/moderator.go:162` seeds sampling with `rand.New(rand.NewSource(time.Now().UnixNano()))`. The `//nolint:gosec` annotation reasons that sampling is "not crypto" — but sampling here *is* adversarial: a node that predicts when it will be challenged can behave selectively while misbehaving otherwise. **Fix:** use `crypto/rand` for challenge selection.

---

## 6. Low Severity Findings

**WRP-29 — Message signature does not bind the destination route.** `event/event.go:303-309`. `SigningBytes()` covers only `Body` and the Unix-nanosecond timestamp; `Destination`, `MessageId`, `NodeId`, and `Version` are unsigned, so the signature is not domain-separated per route. Impact is low today because the libp2p protocol ID enforces the route and the signature is bound to the connection's peer, but it would become exploitable if a message were ever accepted off-connection. **Fix:** include route and message ID in the signing bytes.

**WRP-30 — Non-constant-time session token comparison.** `core/handler/pair.go:61` uses `!=`. The token is high-entropy and sits behind libp2p encryption and network jitter, so practical exploitation is unlikely. **Fix:** `crypto/subtle.ConstantTimeCompare`.

**WRP-31 — Pairing token has no expiry, single-use property, or revocation.** `core/handler/pair.go:57-64`, `database/auth-repo.go:98-100`. The token is set once and lives for the whole process lifetime, and `domain/warpnet.go:43-58` renders it together with the PSK into the pairing QR. A token that leaks once — a screenshot, a shoulder-surfed QR, a log line — grants permanent device pairing. **Fix:** short-lived single-use tokens with a revocation path; treat the QR as a secret in the UX.

**WRP-32 — Frontend persists the channel AES key in `localStorage`.** `frontend/src/lib/transport.js:112`. Any XSS in the dashboard exfiltrates the channel key permanently. **Fix:** hold in memory or `sessionStorage`.

**WRP-33 — No Content-Security-Policy on the Wails renderer.** `frontend/public/index.html`, `cmd/node/member/main.go:63-65`. No live XSS sink was found — there is no `v-html`, mustache escaping is used throughout, and `v-linkify` runs on already-escaped content with `ignoreTags:["script","style"]` — but the renderer displays bridged Mastodon content and loads remote media, and `app.go:280` `Call()` signs every message with the owner's key. Any future XSS would therefore act with full owner authority. **Fix:** add a strict CSP (`default-src 'self'`, constrained `img-src`/`frame-src`, no `unsafe-inline`).

**WRP-34 — Exported `MainActivity` honours a `REDIRECT_URL` extra.** `warpdroid/.../MainActivity.kt:547-552`, `AndroidManifest.xml:41-44`. A malicious local app can launch the activity and force navigation to an arbitrary URL. No privilege gain; phishing assistance only. Pre-existing Tusky pattern. **Fix:** honour the extra only for internally-originated intents, or validate the host.

**WRP-35 — Database key injection via `/` in untrusted IDs.** `database/local-store/prefix-tree.go:112-143` concatenates untrusted IDs into keys with a `/` delimiter and never rejects an ID containing `/`. The store is a flat KV with server-controlled parent segments, so practical impact is limited, but attacker-controlled trailing IDs can synthesize extra key segments. **Fix:** reject `/` in IDs, or hash them before key construction.

**WRP-36 — Connection manager grace period defeats flood shedding.** `core/warpnet/warpnet.go:293-300`: `NewConnManager(20, 50, WithGracePeriod(time.Hour))`. New connections are never trimmed for an hour. The resource manager is left at `DefaultLimits.AutoScale()` with the configurable path still a TODO. **Fix:** shorten the grace period, lower the high-water mark, set explicit limits.

**WRP-37 — Non-hermetic build inputs.** Go toolchain and Gradle distributions fetched by `curl` with no checksum verification (`Dockerfile.remote:6`, `Dockerfile.member:10`, `Dockerfile.moderator:6`, `snap/snapcraft.yaml:77`, `build-warpdroid.yml:47-51`); base images pinned by mutable tag rather than digest; `Dockerfile.moderator:24` builds with `-mod=mod`. **Fix:** verify checksums, pin by digest, use `-mod=vendor`.

**WRP-38 — Deployment SSH hygiene.** `deploy-mainnet.yaml`, `build-deploy-testnet.yaml`: the private key is written via inline interpolation, `GITHUB_TOKEN` is piped into a remote **root** shell, and `ssh-keyscan` re-establishes trust on first use each run. Maintainer-dispatch only, so not fork-exposed. **Fix:** deploy as a non-root user, pin the host key, pass secrets via `env:`.

---

## 7. Informational Findings

**WRP-39 — The codebase-integrity challenge is dead code and cannot work as designed.** `core/challenge/` contains only `.gitkeep`. `GetCodebaseHashHex` and `walkAndHash` (`security/hashing.go:44`, `security/psk.go:55-107`) are referenced only from tests. `ChallengeEvent`, `ValidationEvent`, and `SelfHashHex` are defined (`event/event.go:500-523`) and `PUBLIC_POST_NODE_CHALLENGE` exists (`event/paths.go:33`), but none are registered in any handler list, `NodeRepo.RelaySelfHashHex` is never set, and the moderator's `StreamChallengeHandler` is self-documented as "NOT REGISTERED anywhere yet."

Beyond being unwired, the scheme could not achieve its goal: a node self-computes and self-reports its own codebase hash, so a tampered node simply reports the clean value. Without a hardware root of trust or remote attestation binding the reported hash to the executing binary, it can detect accidental divergence among honest nodes but not a motivated adversary. **Recommendation:** either remove the dead types and routes to avoid conveying assurance that does not exist, or replace with genuine attestation if the threat model requires it. Never gate an authorization decision on a self-reported hash.

**WRP-40 — DHT forced to server mode on all nodes.** `core/dht/dht.go:127` sets `dht.ModeServer` unconditionally, so NAT'd home nodes serve DHT queries and store records for arbitrary peers. Consider `ModeAuto`.

**WRP-41 — Hardcoded Echo bot credential.** `cmd/node/member/echo-member.go:64-65` hardcodes the bot password and owner ID, making that identity reproducible by anyone. Low impact for a demo bot, but it should not be a committed constant if the identity is meant to be authentic.

**WRP-42 — Anti-debugger measures are partly valuable, partly theater.** `security/anti-debugger.go`. Disabling core dumps (`PR_SET_DUMPABLE=0`, `RLIMIT_CORE=0`) is genuinely valuable — it keeps the in-memory database key and identity key out of core files — and should be kept. The `TracerPid`/`PtraceDenyAttach` checks (lines 65-79) are bypassed by renaming the debugger, using eBPF or hypervisor tooling, or statically patching the `init`, and should not be counted as a control; `log.Fatalf` on detection also creates a self-DoS path.

**WRP-43 — Idempotency coverage gaps.** `core/middleware/idempotency.go:209-215`. Deduplication applies only to routes containing `/post/`, leaving `/delete/` undeduplicated, with a 1024-entry LRU. The cache is correctly peer-scoped and bounded. Within the 10-minute TTL, reusing a `messageID` with a different body returns the first response and silently skips the second side effect. **Fix:** bind a body hash into the key; extend coverage to `/delete/`.

**WRP-44 — Broad `FileProvider` path mappings.** `warpdroid/app/src/main/res/xml/file_paths.xml` maps whole roots (`external-path path="."`, `cache-path path="."`). The provider is `exported="false"` with per-URI grants, so exposure requires the app to mint a URI, but the mapping is broader than necessary. **Fix:** scope to dedicated subdirectories.

---

## 8. Positive Observations

These controls were reviewed and found sound. They should be preserved through remediation.

- **Transport security.** All peer links use Noise with no insecure or plaintext fallback (`core/node/options.go:59`). `RemotePeer()` is cryptographically bound, so wire-level identity spoofing is not possible.
- **Canonical signing.** Moderation verdicts and report IDs use explicit length-prefixed canonical signing bytes rather than re-marshalled JSON (`event/event.go:639-670,582-603`), avoiding field-aliasing and cross-version drift. The signing *scheme* is robust; only the trust model around it is flawed.
- **Replay protection.** A 5-minute symmetric freshness window rejects stale and replayed remote messages (`core/middleware/auth.go:107-112`).
- **Idempotency cache.** Peer-scoped keys, bounded entry count and payload size, defensive copies, and single-leader collapse of concurrent duplicates (`core/middleware/idempotency.go:95-173`).
- **Reporter identity is not spoofable.** Report events stamp the reporter from the verified publishing envelope, not the body (`member-pubsub.go:141-143`); moderator ballots likewise take `ModeratorID` from the verified gossip envelope (`vote/vote.go:54-56`), preventing ballot stuffing by a single node.
- **Encryption at rest.** BadgerDB is genuinely encrypted (`database/local-store/db.go:255`) — the weakness is the KDF (WRP-07), not the absence of encryption.
- **Media metadata cryptography.** AES-256-GCM with Argon2id (64 MiB), random salt and nonce, and key zeroization after use (`security/aes.go:75-123`).
- **Video parsing is properly defensive.** Strict ISO-BMFF `ftyp` validation, a bounded box walk with no infinite loop or overflow, a MIME allowlist, and a 36 MiB cap (`core/handler/video.go:117-280`).
- **EXIF parsing is never exposed to untrusted bytes.** The `dsoprea` decoder runs only on Warpnet's own re-encoded JPEG output (`image.go:276`), so malformed-EXIF panics are unreachable from peer input.
- **No SSRF in the media path.** No user-supplied URL is fetched; media transfer is libp2p peer-ID streaming and imports carry inline base64. The only HTTP client targets GitHub with non-user URLs.
- **Relay amplification limits.** `MaxReservations`, `MaxCircuits`, per-IP and per-ASN reservation caps, and 32 MiB / 5-minute data and duration limits (`core/relay/relay.go:67-86`).
- **Client-side XSS hygiene.** No `v-html` anywhere; all user content rendered through escaping mustache; `decodeHtmlEntities` uses a detached `<textarea>` and returns `.value` as text. YouTube embedding uses a strict 11-character ID regex, `encodeURIComponent`, `youtube-nocookie.com`, and is click-gated.
- **Minimal Wails IPC surface.** Bound methods expose no file read/write, path access, or command execution. Deep links are strictly parsed on both the Go and JS sides, with `path.Clean` blocking traversal and only `Kind=user` accepted; `os/exec` usage uses fixed argv with no shell.
- **Android platform hardening.** No WebView or `addJavascriptInterface` anywhere; `allowBackup="false"`; pairing credentials in Keystore-backed `EncryptedSharedPreferences`; peer-ID pinning re-verified on every reconnect; sensitive components `exported="false"`; cleartext traffic globally forbidden apart from a correct `.onion` exception.
- **No secrets logged.** Passwords, seeds, and keys appear in no log statement; `auth.go:109` logs the username only (verified across all non-vendor, non-test Go files).
- **No committed key material.** No `.pem`, `.key`, or equivalent in the working tree or git history; `.gitignore` blocks them.
- **TLS hygiene.** No `InsecureSkipVerify` in first-party code. `core/notifications/mailer.go:99` performs normal verification.
- **Supply chain baseline.** No `pull_request_target`, no self-hosted runners, `go mod verify` and `govulncheck` in CI, macOS releases codesigned and notarized, no `replace` directives redirecting to untrusted forks.
- **Input bounds on user content.** Rune-based limits on tweets (280), chat messages (5000), and report reasons (256), plus media key size and count caps.
- **Password policy.** 8–32 characters with upper, lower, digit, and special class requirements (`cmd/node/member/auth/auth.go:246-275`).
- **Downgrade protection in self-update.** `selfupdate.go:226` requires a strictly greater version, correctly preventing rollback.

---

## 9. Remediation Roadmap

**Phase 1 — Blocking for any deployment with real user data**

1. **WRP-01** — add a central authorization gate for `/private/*` keyed on `IsPrivate()`; wire up the helper that already exists.
2. **WRP-02** — stop re-signing foreign gossip payloads with the local key; verify the original author signature; restrict `SelfPublish` to content routes; add topic validators.
3. **WRP-03** — bind actor identity to the authenticated peer in one shared helper used by every mutating handler.
4. **WRP-04** — make the AES codec fail closed, bind the dashboard to `127.0.0.1`, correct the misleading banner, add per-connection session tokens.

Findings 1–3 share a root cause and are best fixed together as a single authorization layer rather than as three patches.

**Phase 2 — High priority**

5. **WRP-07 + WRP-12** — put Argon2id with a stored random salt between the password and both the identity and database keys; source the Android device identity from a Keystore-persisted random seed.
6. **WRP-06 + WRP-08** — introduce a pinned moderator trust root and rotate all seed-derived infrastructure identities. These must ship together; neither is effective alone.
7. **WRP-10 + WRP-11** — clamp `limit` and `optionsNum` at the database boundary. These are one-line fixes for unauthenticated remote crashes and should not wait for Phase 2 scheduling if resources allow.
8. **WRP-09** — sign release artifacts and verify signatures before installing updates.
9. **WRP-13** — add read deadlines and per-route size limits; bound outbound response reads.
10. **WRP-14** — reduce the Snap workflow to least privilege and SHA-pin its actions.
11. **WRP-15** — filter gossip-learned addresses through `IsPublicMultiAddress` before dialing.

**Phase 3 — Hardening**

12. WRP-16 through WRP-28: rate limiting, connection-layer block enforcement, secret removal from compose, container and monitoring hardening, SHA-pinned actions, CSPRNG for tokens and audit sampling.
13. WRP-29 through WRP-44: signature domain separation, constant-time comparisons, pairing token lifecycle, CSP, build hermeticity, and removal of the dead challenge code.

**Cross-cutting recommendation.** WRP-05 (the public PSK) is not independently fixable in a meaningful way, and attempting to "fix" it by making the PSK secret would be the wrong lesson. The correct posture is to **document the network as publicly joinable and design every authorization decision on that assumption**. Several findings in this report exist because the PSK was implicitly treated as a membership boundary. Making that assumption explicit in the threat model is the single change most likely to prevent the next instance of this bug class.

---

## Appendix A — Automated Tooling Results

### A.1 `govulncheck`

Eight vulnerabilities affect code reachable from this module. Seven stem from the pinned Go toolchain (`go 1.26.3`) and are fixed in 1.26.4–1.26.6:

| ID | Package | Fixed in |
|---|---|---|
| GO-2026-6218 | `net/url` | go1.26.6 |
| GO-2026-6090 | `crypto/tls` | go1.26.6 |
| GO-2026-5972 | `encoding/asn1` | go1.26.6 |
| GO-2026-5856 | `crypto/tls` | go1.26.5 |
| GO-2026-5039 | `net/textproto` | go1.26.4 |
| GO-2026-5037 | `crypto/x509` | go1.26.4 |
| GO-2026-5026 | `net/http` | go1.26.6 |
| GO-2024-3218 | `go-libp2p-kad-dht v0.41.0` | no fix available |

**Recommendation:** bump the toolchain to go1.26.6, which resolves seven of the eight in a one-line change. Track GO-2024-3218 upstream.

*Operational note:* a toolchain-version mismatch causes `govulncheck` to fail package loading rather than report findings. Confirm the CI step fails loudly on that condition rather than passing silently.

### A.2 `gosec`

128 files, 29,951 lines, 14 findings. Manual triage of the six HIGH-confidence results:

| Rule | Location | Triage |
|---|---|---|
| G115 (integer overflow) | `core/handler/user.go:116-118` | **False positive** — follower/tweet counts cannot approach int64 bounds |
| G703 (path traversal) | `config/config.go:264` | **False positive** — `MkdirAll` on the OS-provided home directory |
| G702 (command injection) | `core/selfupdate/executable.go:104` | **False positive** — `syscall.Exec` re-executes the node's own binary |
| G404 (weak RNG) | `cmd/node/moderator/moderator/moderator.go:162` | **True positive** → WRP-28 |
| G204 (subprocess) | `cmd/node/member/deeplink/register_linux.go:84,94` | Reviewed — fixed argv, no shell, arguments not attacker-controlled |
| G304/G301/G306 | `core/selfupdate/*`, `deeplink/register_linux.go` | Reviewed — no exploitable path; file permissions worth tightening |

Static analysis found none of the four Critical findings. All four are authorization and trust-model defects, which pattern-based scanners do not model — a useful reminder that a clean `gosec` run is not evidence of a secure design.

### A.3 Secret scanning

No private key material (`.pem`, `.key`, `.p12`, `.jks`) in the working tree or in git history. One hardcoded credential in tracked configuration (WRP-21) and one hardcoded bot password (WRP-41).

### A.4 Test suite

`go test` passes across `security`, `core/middleware`, `event`, `database`, and `database/local-store`. The findings in this report are design-level defects, not regressions — the code behaves as written.

---

## Appendix B — Threat Model Summary

| Attacker | Capability assumed | Findings enabled |
|---|---|---|
| **Any Internet host** | Derive the public PSK and join the overlay (WRP-05) | WRP-01, WRP-02, WRP-03, WRP-06, WRP-10, WRP-11, WRP-13, WRP-15, WRP-16, WRP-23 |
| **Network-adjacent host** | Reach a hosted node's dashboard port | WRP-04, WRP-18, WRP-25 |
| **Offline attacker** | Knows a target's peer ID and username, or holds a stolen DB volume | WRP-07, WRP-08, WRP-22 |
| **Supply-chain attacker** | Compromises a release credential or third-party action | WRP-09, WRP-14, WRP-26, WRP-37 |
| **Co-located mobile app** | Reads public `Build` fields; launches exported activities | WRP-12, WRP-34 |
| **Local attacker** | Code execution on the user's machine | WRP-32, WRP-33, WRP-42 |

---

*This report describes unremediated vulnerabilities and should be handled accordingly until the Phase 1 items are addressed.*
