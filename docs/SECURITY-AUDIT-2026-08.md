# Warpnet — Security Assessment Report

| | |
|---|---|
| **Target** | Warpnet — decentralized P2P social network (Go / libp2p, Vue+Wails desktop, Android) |
| **Repository** | `github.com/Warp-net/warpnet` |
| **Revision audited** | `27ead3ce` (branch `develop`) |
| **Assessment date** | 2026-08-14 |
| **Re-test 1** | `ada3d0a0` — 2026-08-16 |
| **Re-test 2** | `28eef1ef` — 2026-08-16 |
| **Re-test 3** | `2c0802d3` — 2026-08-22 |
| **Re-test 4** | `392e17d2` — 2026-08-22 |
| **Assessment type** | White-box source code review + automated static analysis + dependency analysis |
| **Classification** | Internal — contains unremediated vulnerability details |

> **Re-test status.** Findings struck through (~~like this~~) were re-verified against the revision noted on each and are **closed**. Findings marked ⚠️ are **partially** addressed — the cited code changed but some of the stated impact remains reachable. **All four Critical findings are closed**; re-test 3 closes four more and partially addresses five. See §10 for the verification record.

---

## 1. Executive Summary

Warpnet was assessed in a white-box review covering the Go node (networking, cryptography, storage, request handlers), the Wails/Vue desktop client, the Android client, and the build and deployment supply chain.

The codebase shows genuine security engineering effort in many places. All peer links use Noise encryption with no plaintext fallback. Message envelopes are ed25519-signed using canonical length-prefixed signing bytes that resist field-aliasing. The local database is encrypted at rest. The idempotency cache is peer-scoped, bounded, and returns defensive copies. Relay resources are capped against amplification. The media-metadata scheme uses Argon2id correctly with key zeroization. On the client side there is no `v-html` anywhere, no WebView in the Android app, and Android pairing secrets live in Keystore-backed encrypted storage. There is no `InsecureSkipVerify` in first-party code, no committed private key material, and CI already runs `go mod verify` and `govulncheck`.

Against that, the assessment identified **four Critical and eleven High severity issues**. They are not independent defects; they are repeated instances of one architectural gap:

> **Warpnet consistently authenticates *who is speaking* but almost never authorizes *what they are allowed to say*.**

The signature layer is well built and correctly proves that a message was signed by the connecting peer's own key. What is missing everywhere is the next step — checking whether that peer may invoke this route, author this content, or issue this verdict. Because the network's only admission gate (the libp2p PSK) is computed from a hardcoded constant in open-source code, "any peer" means "any host on the Internet."

The four Critical findings:

1. ~~**Every `/private/*` route is reachable by any peer, with no owner check.**~~ **CLOSED.** An attacker read the victim's direct messages and notifications and overwrote their profile and settings. A `WarpRoute.IsPrivate()` helper existed in the codebase but was referenced *only by tests* — the authorization gate was designed and never wired up. It is now enforced in the auth middleware.
2. ~~**Following someone hands anyone impersonating them a remote write primitive.**~~ **CLOSED.** Gossip payloads on a followed user's topic were re-signed *with the victim's own private key* and executed as a local self-stream on an attacker-chosen route, bypassing the replay gate. The re-signing has been removed and self-streams now carry the real sender.
3. ~~**Content authorship is taken from the request body, unbound to the signing peer.**~~ **CLOSED.** An attacker set `UserId` to the victim's ID and the node created the tweet *inside the victim's account*. Authorship is now bound to the connection's remote peer.
4. ~~**The remote dashboard's password gate fails open.**~~ **CLOSED.** The AES codec accepted plaintext when decryption failed, the listener bound all interfaces while printing "localhost", and every request was signed with the owner's key. The channel is now a fail-closed Noise `XX` session in which the client proves possession of a static key enrolled only by a successful login, and the bind address is configurable with an honest banner.

**Re-test outcome.** **All four Critical findings are closed**, together with seven lower-severity findings (WRP-11, WRP-16, WRP-21, WRP-22, WRP-27, WRP-29, WRP-40). The fixes address root causes rather than symptoms and are covered by regression tests.

**Re-test 3 (2026-08-22, `2c0802d3`).** Remediation continued across the High and Medium tiers. Newly closed: **WRP-11** (poll option ceiling), **WRP-16** (per-peer per-route leaky-bucket rate limiting in middleware), **WRP-27** (CSPRNG session tokens) and **WRP-40** (`dht.ModeAuto`). Five findings moved to partial: **WRP-07** (both root secrets now run through Argon2id — the headline "unsalted SHA-256" is gone, but the salt is a deterministic public value and the identity is still password-derived, so there is still no rotation path), **WRP-10** (clamped in the store, still unclamped in two user-repository routes), **WRP-08** (moderator seeds removed from compose, relay/bootstrap seeds still committed), **WRP-17** (report ingress rate-limited on streams but not on the pubsub topic the moderators actually read) and **WRP-39** (dead hashing code removed, unwired route and fields remain).

**Re-test 4 (2026-08-22, `392e17d2`).** One PR (#458) on top of `2c0802d3`, with no other changes in between. It closes **WRP-10** and clears both defects that the re-test-3 remediation had introduced. The moderator's seed is now a real 32-byte random value rather than a zero-length slice, so the node starts and its identity is genuinely random — which also settles the moderator half of WRP-08. The `security` package builds under test again. WRP-10 is closed at the root: the capacity hint is bounded independently of the caller's limit, and `UserRepo.Search`/`WhoToFollow`, which allocate above the store and never saw the original clamp, are bounded too. The page-truncation regression that the original clamp caused is gone with it.

~~Two defects were introduced by the re-test-3 remediation itself and are called out where they occur, because both are cheap to fix and one is a hard outage:~~ **Both fixed at `392e17d2`** (#458). Recorded here because the pattern is worth keeping: neither would have reached `develop` had `go build ./... && go test ./...` been run on the remediation branch.

1. ~~**The moderator node can no longer start.**~~ `cmd/node/moderator/main.go:77` allocated its random seed as `make([]byte, 0, 32)` — length zero, so `rand.Read` filled nothing and `GenerateKeyFromSeed` returned `ErrEmptySeed`. The process logged `moderator: fail generating key` and exited. Now `make([]byte, 32)`. See WRP-08.
2. ~~**The `security` package no longer compiles under test.**~~ `GetCodebaseHashHex` was removed from `security/hashing.go` while `security/hashing_test.go` still called it, so `go vet ./security/...` and `go test ./...` failed to build. `psk_test.go` had two further stale references (`generateAnchoredEntropy`, `ErrPSKNetwrokRequired`) that the original report did not name. All removed. See WRP-39.

WRP-04 took two rounds and is worth recording as a pattern. The first round fixed the *reported defect* — the plaintext fallback — but left the *reported outcome* intact, because the replacement used Noise `NX`, which authenticates the server to the client and not the reverse; the dashboard password had also been removed without anything taking its role. The second round closed it properly by switching to `XX` and gating every privileged route on a client key that only a successful login enrolls. The lesson generalizes: encrypting a channel is not authenticating its peer, and a finding is closed when its stated impact is unreachable, not when the specific code it cited has changed.

The remaining risk is now concentrated in the untouched High-severity findings — chiefly WRP-06 (any peer can forge network-wide moderation verdicts), WRP-12 (Android device identity derived from public `Build` fields) and WRP-13 (50 MiB pre-authentication buffering with no read deadline).

Two further observations worth the reader's attention:

**~~A cryptographic inversion.~~** ~~Argon2id with a 64 MiB memory cost protects a *throwaway, deliberately-brute-forceable* media password, while the *permanent, unrevocable account identity* gets one unsalted round of SHA-256 (WRP-07). The strong primitive is already in-tree and applied to the wrong asset.~~ **Corrected at `2c0802d3`** — `security/kdf.go` now routes both the identity key and the database key through the same Argon2id parameters. The inversion still applies on Android, where the device identity key is derived purely from public `android.os.Build` fields (WRP-12).

**An integrity mechanism that cannot work.** The codebase-integrity challenge (`core/challenge/`, `GetCodebaseHashHex`) is unwired dead code, and by design could not defend against a tampered node even if wired: a node self-computes and self-reports its own hash, so a modified node simply reports the clean value (WRP-39). It should not be relied on for any trust decision. Most of the dead code is gone as of `2c0802d3`; the route and the event fields are not.

We recommend treating WRP-01 through WRP-04 as blocking for any deployment carrying real user data.

### Findings by severity

Counted at re-test 4 (`392e17d2`). "Partial" means the cited code changed but part of the stated impact is still reachable; those findings are counted as open for prioritisation.

| Severity | Original | Closed | Partial | Open | Open / partial IDs |
|---|---|---|---|---|---|
| **Critical** | 4 | **4** | 0 | **0** | — |
| **High** | 11 | 2 | 2 | **9** | WRP-05, WRP-06, ⚠️WRP-07, ⚠️WRP-08, WRP-09, WRP-12 … WRP-15 |
| **Medium** | 13 | 4 | 1 | **9** | ⚠️WRP-17, WRP-18 … WRP-20, WRP-23 … WRP-26, WRP-28 |
| **Low** | 10 | 1 | 0 | **9** | WRP-30 … WRP-38 |
| **Informational** | 6 | 1 | 1 | **5** | ⚠️WRP-39, WRP-41 … WRP-44 |
| **Total** | **44** | **12** | **4** | **32** | |

**Closed:** WRP-01, WRP-02, WRP-03, WRP-04, WRP-10, WRP-11, WRP-16, WRP-21, WRP-22, WRP-27, WRP-29, WRP-40.

**Highest-priority open findings:** WRP-06 (forgeable moderation verdicts), WRP-13 (pre-auth buffering) and WRP-12 (Android identity). All three are untouched since the original assessment.

**Cheapest outstanding fixes:** WRP-30 (`crypto/subtle` for the token compare), WRP-15 (`IsPublicMultiAddress` before dialing) and WRP-14 (least privilege plus a SHA pin in one workflow).

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

### ~~WRP-01 — Critical — All private routes are reachable by any network peer without authorization~~ ✅ CLOSED

| | |
|---|---|
| **CWE** | CWE-862: Missing Authorization |
| **CVSS 3.1** | 9.1 (`AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N`) |
| **Location** | `core/node/node.go:206-220`, `core/middleware/auth.go:98-104`, `core/stream/routes.go:46` |
| **Status** | **CLOSED at `ada3d0a0`** — verified by re-test |

> **Resolution.** `core/middleware/auth.go:82` now gates every private route:
> ```go
> if route.IsPrivate() && !p.isPrivateRouteAllowed(route, remotePeer, s.Conn().LocalPeer()) {
>     return nil, ErrUnknownClientPeer
> }
> ```
> `isPrivateRouteAllowed` admits only the loopback self-stream (`remotePeer == localPeer`), the node's own ID, or a peer present in the paired-devices store; `PRIVATE_POST_PAIR` is the single deliberate exception, and it remains gated by the session-token check in `core/handler/pair.go:61`. `IsPrivate()` is now referenced from production code rather than only from tests. Covered by regression tests in `core/middleware/auth_test.go`. The fix is applied centrally in the middleware, as recommended, rather than per handler.

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

### ~~WRP-02 — Critical — Gossip payloads are re-signed with the owner's key and executed as privileged self-streams~~ ✅ CLOSED

| | |
|---|---|
| **CWE** | CWE-269: Improper Privilege Management; CWE-345: Insufficient Verification of Data Authenticity |
| **CVSS 3.1** | 9.3 (`AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:N`) |
| **Location** | `core/pubsub/gossip.go:457-486`, `cmd/node/member/pubsub/member-pubsub.go:120-124`, `core/stream/loopback-stream.go:57-63` |
| **Status** | **CLOSED at `ada3d0a0`** — verified by re-test |

> **Resolution.** Two changes together close this, and both were needed.
>
> 1. The `ed25519.Sign(g.privKey, ...)` re-signing is **gone** from `SelfPublish` (`core/pubsub/gossip.go:457-485`). The original author's signature now travels with the message instead of being replaced by a locally-forged one.
> 2. The loopback connection no longer reports the local peer at both ends. `core/stream/loopback-stream.go:57-62` now returns distinct `localPeerID` and `remotePeerID`, and `SelfStream` is called with the actual sender taken from the envelope.
>
> Consequently the auth middleware verifies a gossiped message against the *real author's* key, the freshness gate applies (the sender is no longer loopback), and — because the attacker can now only speak as themselves — the WRP-01 private-route gate rejects any attempt to reach a privileged write route this way. `PUBLIC_POST_TWEET` delivery additionally proves the author's signature. The signature also binds the destination now (see WRP-29), so a message captured for one route cannot be replayed onto another.

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

### ~~WRP-03 — Critical — Content authorship is taken from the request body, unbound to the authenticated peer~~ ✅ CLOSED

| | |
|---|---|
| **CWE** | CWE-345: Insufficient Verification of Data Authenticity; CWE-290: Authentication Bypass by Spoofing |
| **CVSS 3.1** | 8.8 (`AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N`) |
| **Location** | `core/handler/tweet.go:156-190`, `core/handler/following.go:124-168`, `core/handler/block.go:73-95`, `core/handler/reaction.go:86-131` |
| **Status** | **CLOSED at `ada3d0a0`** — verified by re-test |

> **Resolution.** A shared helper now binds the claimed actor to the authenticated peer:
> ```go
> // core/warpnet/warpnet.go:227
> func VerifyAuthorship(s WarpStream, actorNodeId string) error {
>     if actorNodeId != "" && s != nil && s.Conn() != nil && actorNodeId == s.Conn().RemotePeer().String() {
>         return nil
>     }
>     return ErrForeignAuthor
> }
> ```
> The claimed author is resolved through the user repository and their node ID compared against `s.Conn().RemotePeer()`, so setting `ev.UserId` to a victim's ID now fails with `ErrForeignAuthor`. It fails closed: an unknown or empty actor node ID is rejected rather than admitted. Applied at 16 call sites spanning tweets, replies, reactions, retweets, follows, polls, chats, views and timeline (`core/handler/{tweet,reply,reaction,retweet,following,poll,chat,view,timeline}.go`), with regression tests in `tweet_test.go`, `reaction_test.go` and `reply_test.go`. Implementing this as one helper rather than per-handler logic is the right shape — new handlers inherit the check by calling it.

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

### ~~WRP-04 — Critical — Remote dashboard `/ws` executes any route as the owner, with a bypassable password gate on `0.0.0.0`~~ ✅ CLOSED

| | |
|---|---|
| **CWE** | CWE-287: Improper Authentication; CWE-306: Missing Authentication for Critical Function |
| **CVSS 3.1** | 9.0 (`AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H`) |
| **Affects** | `remote` build tag only (Docker/server deployments). The Wails desktop application was **not** affected. |
| **Status** | **CLOSED at `28eef1ef`** — all three components verified; adversarial re-test executed |

> **Resolution.** Closed over two rounds; each component is addressed.
>
> **(a) Fail-closed channel** (round 1, `ada3d0a0`). `AESCodec` was removed from `security/aes.go` entirely. `bridge.go` performs a mandatory handshake under a 10-second deadline and returns on failure; every frame then goes through `channel.Decrypt` with the connection closed on error. No plaintext path remains — the original proof-of-concept no longer compiles.
>
> **(b) Client authentication** (round 2, `28eef1ef`). The handshake pattern moved from `NX` to **`XX`** (`security/noise.go:101,140`), so the client presents a static key. That key is now actually checked: the connection captures `channel.RemoteStatic()` and every route other than the first-run probe and login is gated on it —
> ```go
> // cmd/node/member/remote/bridge.go:181-182, 267-277
> c := &clientConn{static: channel.RemoteStatic()}
> c.authorized.Store(b.isEnrolled(c.static))
> ...
> if !b.isAuthorized(c) { resp.Body = newUnauthorizedResp(); break }
> ```
> `isAuthorized` requires both an enrolled key and an authenticated account, and a key is enrolled **only after a successful `AuthLogin`** (`bridge.go:305-307`). Enrollment is in-memory, so it does not survive a restart — a conservative choice. `isEnrolled` rejects empty keys, closing the degenerate case.
>
> **(c) Bind address and banner.** `remote-member.go:132` now binds `net.JoinHostPort(config.Config().Node.Server.Host, port)`, configurable via `node.server.host`, and the banner is honest — it prints the real address and warns explicitly when listening on every interface.
>
> **Adversarial re-test.** The precise scenario previously exploitable — owner logged in, attacker connects with their own key and calls privileged routes — is now refused on every route, with the node never reached and the owner's session intact:
> ```
> --- PASS: TestAudit_WRP04_AttackerAfterOwnerLogin
>     PRIVATE_GET_MESSAGES / PRIVATE_POST_USER / PRIVATE_POST_PAIR / PRIVATE_POST_LOGOUT → 401
> ```
> The project's own regression tests cover the same ground, including the two cases most likely to regress: `TestBridge_LoginEnrollsOnlyTheKeyThatProvedThePassword` and `TestBridge_FailedLoginEnrollsNothing`.
>
> **Residual (tracked separately, not part of this finding).** `node.server.host` still defaults to `0.0.0.0`. That is no longer a takeover vector, since the endpoint is authenticated, but it does expose the login endpoint to the network — and login still has no rate limiting or lockout. See **WRP-18**, whose practical priority rises accordingly. Defaulting the bind to `127.0.0.1` would be reasonable hardening.

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

### WRP-05 — High — The "private network" PSK is derived entirely from public constants — **ACCEPTED AS DESIGNED at `2c0802d3`**

**CWE-798** · `security/psk.go:56-72`, `cmd/node/member/node/member-node.go:147`

> **Status at re-test 3.** The recommendation was adopted in the only form available: the PSK is now *documented* as a partitioning value rather than a secret, and the `spbFounding` / `generateAnchoredEntropy` obfuscation — which suggested a secret where there was none — has been deleted. The derivation is now the honest one-liner `SHA256(network || major)` with the comment "Preshared Secret Key is public for Warpnet goals - it's just separate networks and versions". The property itself is unchanged and remains a documented design decision, so this finding stays listed: it is the premise the rest of the report is scored against, not a defect awaiting a patch.

The libp2p private-network key is a hash of the network name and the major version:

```go
// security/psk.go:69-71 (at 2c0802d3)
majorStr := strconv.FormatUint(v.Major(), 10)
seed := append([]byte(network), []byte(majorStr)...)
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

### ~~WRP-07 — High — Identity private key and database encryption key derive from one unsalted SHA-256~~ ⚠️ **PARTIALLY CLOSED** — downgraded to **Low**

**CWE-916, CWE-759** · `database/auth-repo.go:106-126`, `security/pk.go:46-60`, `database/local-store/db.go:254-255`

> **Resolution at `2c0802d3`.** Both root secrets now go through Argon2id. `security/kdf.go` adds two derivations that reuse the existing `deriveKey` helper — the same parameters already used for media metadata (`argon2.IDKey`, t=1, 64 MiB, p=4, 32-byte output):
> ```go
> // security/kdf.go:40-55
> func DeriveIdentityKey(username, password, network string) (ed25519.PrivateKey, error) {
>     seed := deriveKey([]byte(password), derivationSalt(identityKeyContext, network, username))
>     defer Wipe(seed)
>     return GenerateKeyFromSeed(seed)
> }
> func DeriveDatabaseKey(username, password string) ([]byte, error) {
>     return deriveKey([]byte(password), derivationSalt(databaseKeyContext, username)), nil
> }
> ```
> `database/auth-repo.go:114` and `database/local-store/db.go:254` call them, and the two ad-hoc SHA-256 seeds are gone. The contexts (`warpnet/kdf/v1/identity-key`, `warpnet/kdf/v1/database-key`) give proper domain separation, so the identity key and the DB key are no longer derivable from one another. The identity seed is wiped after use. Per-guess cost moves from ~two SHA-256 operations to one 64 MiB Argon2id — roughly five orders of magnitude, which takes GPU-farm cracking of a policy-conformant password off the table and is the substance of this finding.
>
> **Residual (why this is not fully closed).**
> 1. **The salt is not random.** `derivationSalt` is `SHA256(context ‖ network ‖ username)` — every input is public, so it is domain separation, not a salt. A single Argon2id table is still valid for one known username across all installs; only the memory-hardness stops precomputation from being worthwhile, and the recommendation for a locally-persisted random salt is unimplemented. Fixing it for the DB key is cheap (nothing outside the machine needs to reproduce it); the identity key cannot take a random salt without changing the derivation model.
> 2. **The identity is still password-derived, so compromise remains unrevocable.** The preferred design — a random ed25519 identity wrapped under an Argon2id-derived KEK — was not adopted, so there is still no rotation path short of abandoning the account.
> 3. **No migration path.** Existing accounts derive different keys under the new KDF. `database/local-store/db.go:261` maps Badger's `ErrEncryptionKeyMismatch` to `ErrWrongPassword`, so a pre-`2c0802d3` user is told their password is wrong, with no re-enrollment or re-wrap flow. Confirm this is intended before shipping to installed users.
>
> Downgraded from High to **Low** on the residual: offline cracking of a conformant password is now impractical, and what remains is a missing rotation path plus a public salt.

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

### WRP-08 — High — Predictable and committed node seeds allow infrastructure impersonation — ⚠️ **PARTIALLY ADDRESSED**

**CWE-798, CWE-330** · `config/config.go:114-117`, `cmd/node/relay/main.go:80-85`, `cmd/node/moderator/main.go:76-77`, `deploy/docker-compose-testnet.yml`

> **Status at `2c0802d3`.** Addressed for moderators in intent, unaddressed for relays and bootstrap nodes, and the moderator change does not work.
>
> **(a) Moderator — the fix is a start-up failure.** `NODE_SEED` was removed from every moderator service in `deploy/docker-compose-testnet.yml` and `deploy/docker-compose-warpnet.yml`, and `cmd/node/moderator/main.go` no longer reads it. The replacement does not generate a key:
> ```go
> // cmd/node/moderator/main.go:77-78
> seed := make([]byte, 0, 32)   // length 0, capacity 32
> _, _ = rand.Read(seed)        // fills nothing
> ```
> `rand.Read` writes into `seed[:len(seed)]`, which is empty, so `GenerateKeyFromSeed` hits its `len(seed) == 0` guard (`security/pk.go:46`), returns `ErrEmptySeed`, and `main` logs `moderator: fail generating key` and returns. **The moderator node cannot start at this revision.** The one-character fix is `make([]byte, 32)`. Until then the moderator identity is neither predictable nor random — it does not exist. (This is invisible in deployment today only because the moderator services are commented out for memory reasons.)
>
> **(b) Relay and bootstrap — unchanged.** `cmd/node/relay/main.go:80` still derives from `config.Config().Node.Seed`, and `NODE_SEED=warpnet1|warpnet2|warpnet3` is still committed — and now *active* rather than commented — in `deploy/docker-compose-testnet.yml:16,34,52`. `NODE_SEED=echo-testnet` remains at line 165. The `config.go:116` default is still `"seed" + network + dbDir + host + port`. The relay's own random fallback has the same empty-slice bug (`main.go:81-83`), but it never runs because the config default is never empty. **The three bootstrap identities remain recomputable by any reader of the repository, and have not been rotated.**
>
> **Re-test 4 (`392e17d2`).** The moderator seed is now `make([]byte, 32)`, so `rand.Read` fills it and the node starts with a genuinely random identity — **the moderator half of this finding is closed.** The relay's dead fallback was fixed the same way. (b) is unchanged: `NODE_SEED=warpnet1|warpnet2|warpnet3` is still committed and active in `deploy/docker-compose-testnet.yml:16,34,52`, `NODE_SEED=echo-testnet` at line 165, and the `config.go:116` default is still built from public values.
>
> **Still to do:** move the relay and bootstrap seeds to injected deployment secrets; rotate the three currently-deployed bootstrap identities. Note that a random moderator identity per process is a deliberate trade — it removes the impersonation vector here, and it also means moderators cannot be pinned, which is the open question in WRP-06.

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

### ~~WRP-10 — High — Unclamped `limit` on public list routes causes remote memory exhaustion~~ ✅ CLOSED

**CWE-770, CWE-789** · `database/local-store/db.go:719,747`, `database/user-repo.go:485,557-558`

> **Status at `2c0802d3` — partial.** Both cited `db.go` sites were clamped to `maxLimit = 20`, closing `PUBLIC_GET_USERS`, `PUBLIC_GET_FOLLOWERS` and `PUBLIC_GET_FOLLOWINGS`. The two cited `user-repo.go` sites were not: `Search` (`PUBLIC_GET_USERS_SEARCH`) and `WhoToFollow` (`PUBLIC_GET_WHOTOFOLLOW`) build their own slice *above* the store and never saw the clamp, so `{"query":"a","limit":100000000}` still reserved ~24 GB — `domain.User` is 240 bytes, worse per unit than the original `ListItem` path.
>
> That clamp also introduced a functional regression. Capping the *request* rather than the *allocation* made the store return 20 rows to a caller that asked for 100, and five internal paginators drive their loop off "a short page means the end": `OutboxRepo.ListByNode` (offline messages), two `NotificationRepo` scans, `FilterRepo.FindKeyword` and `cancelRetweetsForEditedTweet`. All stopped after the first 20 rows. `TestOutboxRepoSuite/TestListByNodeBeyondDefaultPage` caught it.
>
> **Resolution. CLOSED at `392e17d2`** (#458), at the root rather than per call site:
> ```go
> // database/local-store/db.go:708-711, 735-737, 760-762
> const (
>     defaultLimit uint64 = 20
>     MaxPageLimit uint64 = 1000
>     maxPrealloc  uint64 = 20
> )
> limit = pageLimit(limit)
> items := make([]ListItem, 0, min(*limit, maxPrealloc))
> ```
> The capacity hint is now bounded independently of the caller's limit, which is the distinction the finding turned on: the untrusted value may bound an *iteration* — the iterator stops at real data anyway — but must never size an *allocation*. This is what the original recommendation asked for ("the pre-allocated capacity provides no benefit and can simply be a small constant"). `Search` and `WhoToFollow` cap their own capacity at `local_store.MaxPageLimit` (`user-repo.go:469,533`), closing the last two routes. The page ceiling stays, raised from 20 to 1000 so it sits above every internal page size instead of below it — a bounded 1000-row page is not the finding's impact.
>
> Verified: `go build ./...`, `go vet ./...`, `go test ./...` and `go test -race ./...` clean on the merged tree; the outbox regression test passes.

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

### ~~WRP-11 — High — Unbounded poll `optionsNum` causes remote memory exhaustion~~ ✅ CLOSED

**CWE-770** · `database/poll-repo.go:180`, `core/handler/poll.go:90,123,150`

> **Resolution.** **CLOSED at `2c0802d3`.** `core/handler/poll.go:282-284` rejects the request before any allocation:
> ```go
> if optionsNum > 20 {
>     return event.PollResultsResponse{}, warpnet.WarpError("poll: too many options")
> }
> ```
> Verified that this covers every path: `pollResults` is the only caller of `PollRepo.Results` in the tree, and both `PUBLIC_GET_POLL` (`poll.go:128`) and `PUBLIC_POST_POLL_VOTE` (`poll.go:168`) reach `Results` only through it. The clamp sits in the handler rather than in `poll-repo.go` as recommended, which leaves the repository trusting its caller — acceptable while `pollResults` is the sole entry point, but a bound inside `Results` would be more durable.

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

> **Re-confirmed open at `2c0802d3`, and note the ordering.** The read moved to `core/node/node.go:239-241` but is otherwise unchanged, there is still no `SetReadDeadline` on any inbound remote stream, and `core/stream/stream.go:251-252` still does an unbounded `buf.ReadFrom(rw)`. Importantly, the new rate limiter (WRP-16) does **not** mitigate this: `unwrap` is the outermost wrapper, so the full 50 MiB is buffered *before* `RateLimiterMiddleware` — or any other middleware — is entered. Rate limiting a request you have already read into the heap does not bound the heap. Moving the size and deadline enforcement ahead of the read, or the limiter ahead of `unwrap`, is the fix.

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

~~**WRP-16 — No application-layer rate limiting on content creation.**~~ ✅ **CLOSED at `2c0802d3`.** `core/middleware/rate-limiter.go` adds a leaky-bucket limiter keyed on `route|remotePeer`, wired into the chain ahead of auth and dispatch on all three node types (`cmd/node/{member,relay,moderator}/node/*-node.go`). Routes fall into eight named classes — writes get burst 30 / 120 per minute, uploads 10 / 30, pairing 5 / 15, reports 10 / 30 — with unlisted routes defaulting by `IsGet()`. Buckets live in a 4096-entry LRU with a 10-minute TTL, so the limiter is itself bounded, and loopback and own-node streams are exempt. This is the recommended shape: token-bucket, per authenticated peer, in middleware before dispatch. Per-*acting-user* limiting was not added, but with WRP-03 closed the actor is bound to the peer, so per-peer is now equivalent for content routes. *Not covered:* pubsub-delivered events, which never pass through a stream handler (see WRP-17), and anything before `unwrap`'s 50 MiB read (see WRP-13).

**WRP-17 — Report channel enables targeted takedowns and moderator resource exhaustion.** ⚠️ **Partially addressed at `2c0802d3`.** `core/handler/report.go:56-118`, `cmd/node/moderator/moderator.go:204-233`. `PUBLIC_POST_REPORT` is now rate-limited to burst 10 / 30 per minute per peer (`rate-limiter.go:80`), which bounds report *ingress through a member node's stream handler*. It does not bound the path that matters: `core/handler/report.go` republishes to `event.ReportsTopic`, and the moderator subscribes to that topic directly (`cmd/node/moderator/pubsub/publisher.go:104`), so an attacker publishing straight to `ReportsTopic` reaches the fleet without touching a rate-limited stream. Any user may still report any target, validation still covers only reason length and type, and there is still no dedup or threshold before a round opens. Reporter identity *is* correctly stamped by the publisher rather than taken from the body. **Fix:** apply the limit where reports are consumed — a per-reporter budget in the moderator's topic handler — plus dedup, thresholding, and a cap on concurrent rounds.

**WRP-18 — No rate limiting or lockout on login.** `cmd/node/member/auth/auth.go:103-120`, `database/local-store/db.go:245-263`. Re-confirmed open at `2c0802d3`: still no attempt counting, backoff, or lockout, and `node.server.host` still defaults to `0.0.0.0` (`config/config.go:74`). Two things changed, in opposite directions. The per-attempt cost is no longer "as fast as Badger can attempt a key" — WRP-07's Argon2id derivation now sits in front of the Badger open, so online guessing is throttled incidentally by ~5 orders of magnitude, which is the bulk of the original risk. But that same derivation allocates **64 MiB per attempt** on an unauthenticated, unthrottled, internet-facing endpoint, which converts the missing throttle from a credential-guessing problem into a memory-exhaustion one: a few dozen concurrent login attempts will OOM a 2 GiB node. **Fix:** per-connection and per-IP failed-attempt backoff and lockout — now load-shedding as much as anti-guessing — plus a cap on concurrent in-flight derivations; consider defaulting the bind to `127.0.0.1`.

**WRP-19 — Social blocks are not enforced at the connection layer.** `database/node-repo.go:769-807`, `core/discovery/discovery.go:254`. `BlocklistPermanent` is written to the node repository, but with no `ConnectionGater` installed, `IsBlocklisted` is consulted only during discovery and for tweet-level content. A blocked peer can still dial the node and open streams directly, including the WRP-01 and WRP-03 surfaces — so "blocking" an abuser does not actually stop them. **Fix:** install a `ConnectionGater` consulting `IsBlocklisted` in `InterceptSecured`/`InterceptAddrDial`, and drop existing connections on block.

**WRP-20 — Image decompression bomb: no dimension bound before decode.** `core/handler/image.go:261-265`, reached from `core/handler/import.go:101`. Only the *compressed* input is bounded (50 MiB); the decoded pixel buffer is not, so a ~1 KB PNG declaring huge dimensions allocates `width*height*4` bytes. Reachable via the paired device and Twitter-archive import rather than by arbitrary peers, which caps severity. Foreign images fetched over `PUBLIC_GET_IMAGE` are stored as opaque base64 and never decoded — correctly avoiding a remote decode path. **Fix:** call `image.DecodeConfig` first and reject images above a pixel ceiling before `image.Decode`.

~~**WRP-21 — Hardcoded dashboard password committed.**~~ ✅ **CLOSED at `ada3d0a0`.** `NODE_SERVER_PASSWORD=MySecretPassword9000$` no longer appears in `docker-compose.yaml` or in any tracked YAML — the variable was removed along with the password mechanism. *Note:* the credential is public in git history and should still be treated as burned if it was ever reused elsewhere.

~~**WRP-22 — Dashboard channel key is an unsalted SHA-256 of the password.**~~ ✅ **CLOSED at `ada3d0a0`.** `AESKeyFromPassword` and `AESCodec` were removed from `security/aes.go`; the channel key is now established by the Noise handshake rather than derived from a password. (The channel is confidential but still unauthenticated on the client side — see WRP-04.)

**WRP-23 — Every member node runs an open relay.** `core/node/options.go:62` enables `EnableRelayService` unconditionally. Any peer can reserve and relay traffic through any publicly reachable member node, consuming bandwidth and fronting traffic with the operator's IP — an abuse-attribution risk. Per-circuit caps at `core/relay/relay.go:67-86` do correctly prevent high-ratio amplification. **Fix:** enable the relay service only on designated relay nodes.

**WRP-24 — Containers run as root with host networking.** No `USER` directive in `Dockerfile.remote` or `Dockerfile.moderator`; `Dockerfile.relay`/`Dockerfile.echo` use `distroless/static-debian12` rather than the `:nonroot` variant. All compose files use `network_mode: host`. **Fix:** non-root `USER`, `:nonroot` images, explicit published ports, `cap_drop: [ALL]`, `no-new-privileges`.

**WRP-25 — Unauthenticated monitoring services on the host network.** `docker-compose.metrics.yaml:12-20` runs Grafana host-networked with no `GF_SECURITY_ADMIN_PASSWORD` (default `admin/admin`); `deploy/docker-compose-testnet.yml:141-150` exposes a Prometheus pushgateway on `:4091` with no authentication, permitting anonymous metric read and injection. **Fix:** set a strong Grafana password, bind monitoring to localhost or a private network, restrict the pushgateway.

**WRP-26 — Third-party GitHub Actions pinned to mutable tags.** `softprops/action-gh-release@v2`, `codecov/codecov-action@v5`, `docker/*@v3/@v6`, `gradle/actions/setup-gradle@v4`, and notably `golangci/golangci-lint-action@v8` **with `version: latest`** (`tests-static-check.yaml:30-33`), in jobs carrying `contents: write` and `packages: write`. **Fix:** pin to full commit SHAs; pin the linter version.

~~**WRP-27 — Session token entropy derives from the password rather than a CSPRNG.**~~ ✅ **CLOSED at `2c0802d3`.** `database/auth-repo.go:107-111` now reads 32 bytes from `crypto/rand` and base64-encodes them directly; the `username@password@network@randChar@time.Now()` seed and the SHA-256 over it are gone. The token no longer derives from the password at all, so it is independent of WRP-07. Note that this fixes the token's *entropy* only — its lifecycle is still unbounded (WRP-31).

**WRP-28 — Predictable RNG for adversarial moderator audit sampling.** `cmd/node/moderator/moderator/moderator.go:162` seeds sampling with `rand.New(rand.NewSource(time.Now().UnixNano()))`. The `//nolint:gosec` annotation reasons that sampling is "not crypto" — but sampling here *is* adversarial: a node that predicts when it will be challenged can behave selectively while misbehaving otherwise. **Fix:** use `crypto/rand` for challenge selection.

---

## 6. Low Severity Findings

~~**WRP-29 — Message signature does not bind the destination route.**~~ ✅ **CLOSED at `ada3d0a0`.** `SigningBytes()` (`event/event.go:304-311`) now appends `m.Destination` alongside the body and timestamp, so a signature is domain-separated per route and cannot be replayed onto a different one. This mattered more than its original Low rating suggested: it is part of what makes the WRP-02 fix sound, since gossip messages are now accepted off-connection on the strength of the author's own signature. `MessageId` and `NodeId` remain unsigned, which is acceptable given the destination is now covered.

**WRP-30 — Non-constant-time session token comparison.** `core/handler/pair.go:61` uses `!=`. The token is high-entropy and sits behind libp2p encryption and network jitter, so practical exploitation is unlikely. **Fix:** `crypto/subtle.ConstantTimeCompare`.

**WRP-31 — Pairing token has no expiry, single-use property, or revocation.** `core/handler/pair.go:57-64`, `database/auth-repo.go:98-100`. Re-confirmed open at `2c0802d3`. The token is now 32 CSPRNG bytes (WRP-27), but the lifecycle is unchanged: `auth-repo.go:98-100` still sets it once per process and never rotates or expires it, and `domain/warpnet.go:43-58` still renders it with the PSK into the pairing QR. A token that leaks once — a screenshot, a shoulder-surfed QR, a log line — grants permanent device pairing. **Fix:** short-lived single-use tokens with a revocation path; treat the QR as a secret in the UX.

**WRP-32 — Frontend persists a long-lived channel credential in `localStorage`.** ↻ **Re-scoped at `2c0802d3` — same defect, different secret.** The AES key is gone from `localStorage` along with the AES codec (WRP-22), and `frontend/src/lib/transport.js:87-102` now stores only the pinned server static-key fingerprint, which is public. But `frontend/src/lib/noise.js:197-208` persists the client's **x25519 static private key** there instead, and after the WRP-04 fix that key *is* the dashboard credential: the bridge enrols it on successful login and authorises every subsequent privileged route on it (`cmd/node/member/remote/bridge.go:181-182,267-277,305-307`). Any XSS in the dashboard therefore still exfiltrates a reusable credential — arguably a better one than before, since possession alone authorises routes for as long as the node process lives. **Fix:** hold the static key in memory or `sessionStorage` and re-enrol on each browser session. Server-side enrolment is already in-memory and does not survive a node restart, so a session-scoped client key costs nothing in usability. Pair with the CSP from WRP-33.

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

> ⚠️ **PARTIALLY ADDRESSED at `ada3d0a0`.** The `core/challenge/` directory is gone. Remnants still convey the illusion of a control: `PUBLIC_POST_NODE_CHALLENGE` (`event/paths.go:33`), `SelfHashHex` (`event/event.go:522`), `RelaySelfHashHex` (`database/node-repo.go:72`) and `GetCodebaseHashHex` (`security/hashing.go:44`) all remain, none of them wired. Recommend finishing the removal.
>
> ⚠️ **STILL PARTIAL at `2c0802d3` — and the removal broke the build.** `GetCodebaseHashHex` is gone from `security/hashing.go`, and `walkAndHash`, `hashFile`, `spbFounding` and `generateAnchoredEntropy` are gone from `security/psk.go`. Two problems:
> 1. ~~**`security/hashing_test.go` was not updated**~~ and still called `GetCodebaseHashHex` at lines 76, 89-90, 108-109 and 120. The package therefore failed to compile under test — `go vet ./security/...` reported `undefined: GetCodebaseHashHex`, and `go test ./...` could not build, so CI's `-race` run over the module failed at that revision. **Fixed at `392e17d2`**: the four `TestGetCodebaseHashHex*` tests are gone with the `testFS` fixture that existed only for them, and so are two further stale references this report had not spotted — `psk_test.go` was still calling `generateAnchoredEntropy` and the renamed `ErrPSKNetwrokRequired`.
> 2. **The remaining remnants are unchanged at `392e17d2`.** `PUBLIC_POST_NODE_CHALLENGE` (`event/paths.go:33`), `SelfHashHex` (`event/event.go:524`) and `RelaySelfHashHex` (`database/node-repo.go:72`) are all still present and still unwired — and the route is now referenced by the new rate limiter's table (`core/middleware/rate-limiter.go:83`), which gives a dead route the appearance of a live, deliberately-throttled one. This is what keeps the finding open.

~~**WRP-40 — DHT forced to server mode on all nodes.**~~ ✅ **CLOSED at `2c0802d3`.** `core/dht/dht.go:127` now sets `dht.ModeAuto`, so a node advertises itself as a DHT server only when libp2p judges it publicly reachable; NAT'd home nodes fall back to client mode and no longer store records for arbitrary peers.

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
- **Encryption at rest.** BadgerDB is genuinely encrypted (`database/local-store/db.go:258`), and as of `2c0802d3` the key comes from Argon2id rather than a bare SHA-256 (WRP-07).
- **Per-peer route rate limiting.** *Added at `2c0802d3`.* `core/middleware/rate-limiter.go` — leaky bucket keyed on route and remote peer, classed limits per route family, bucket store bounded by an expiring 4096-entry LRU, loopback exempt (WRP-16).
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

**Phase 1 — Blocking for any deployment with real user data** — ✅ **COMPLETE**

1. ~~**WRP-01** — add a central authorization gate for `/private/*` keyed on `IsPrivate()`.~~ ✅ Done.
2. ~~**WRP-02** — stop re-signing foreign gossip payloads with the local key; verify the original author signature.~~ ✅ Done.
3. ~~**WRP-03** — bind actor identity to the authenticated peer in one shared helper.~~ ✅ Done.
4. ~~**WRP-04** — make the codec fail closed, fix the bind and banner, authenticate the client.~~ ✅ Done (two rounds).

Findings 1–3 shared a root cause and were correctly fixed together as a single authorization layer rather than as three patches.

**Phase 0 — Regressions introduced by the Phase 2 work** — ✅ **COMPLETE at `392e17d2`** (#458)

- ~~**Moderator seed allocation** — `cmd/node/moderator/main.go:77`: `make([]byte, 0, 32)` → `make([]byte, 32)`. Without it the moderator node exits at start-up. (WRP-08)~~ ✅ Done, and the relay's identical dead fallback with it.
- ~~**Broken test build** — delete the four `TestGetCodebaseHashHex*` tests in `security/hashing_test.go`, which reference a function removed from the package. Without it `go test ./...` cannot build. (WRP-39)~~ ✅ Done, plus two stale references in `psk_test.go`.
- ~~**Page truncation** — the WRP-10 clamp capped the request rather than the allocation, so every list returned at most 20 rows and five internal paginators stopped early.~~ ✅ Done — see WRP-10.

**Phase 2 — High priority** — partially complete

5. ~~**WRP-07**~~ ✅ Argon2id now sits between the password and both keys. Residual: the salt is a public deterministic value, there is still no identity-rotation path, and existing accounts have no migration (they surface as "wrong password"). **WRP-12** remains — source the Android device identity from a Keystore-persisted random seed.
6. **WRP-06 + WRP-08** — introduce a pinned moderator trust root and rotate all seed-derived infrastructure identities. These must ship together; neither is effective alone. WRP-08 is half-done: moderator identities are now random per process, while the three bootstrap seeds remain committed and unrotated. Note the tension to resolve first — a per-process random moderator identity is the opposite of a pinnable one, so decide what WRP-06's trust root is before hardening either further.
7. ~~**WRP-10 + WRP-11**~~ ✅ Both done — `optionsNum` capped in the handler, and the list pre-allocation bounded independently of the caller's limit.
8. **WRP-09** — sign release artifacts and verify signatures before installing updates.
9. **WRP-13** — add read deadlines and per-route size limits; bound outbound response reads. Note the new rate limiter does not help here: it runs after the 50 MiB read.
10. **WRP-14** — reduce the Snap workflow to least privilege and SHA-pin its actions.
11. **WRP-15** — filter gossip-learned addresses through `IsPublicMultiAddress` before dialing.

**Phase 3 — Hardening**

12. WRP-17 through WRP-28: extend rate limiting to the pubsub report path, login throttling (now also a load-shedding concern — see WRP-18), connection-layer block enforcement, container and monitoring hardening, SHA-pinned actions, CSPRNG for audit sampling. ~~WRP-16~~ ✅ and ~~WRP-27~~ ✅ are done.
13. WRP-29 through WRP-44: constant-time comparisons, pairing token lifecycle, moving the dashboard client key out of `localStorage`, CSP, build hermeticity, and finishing removal of the dead challenge route and fields. ~~WRP-40~~ ✅ is done.

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

**Re-confirmed open at `2c0802d3`:** `go.mod` still declares `go 1.26.3`, so all eight remain applicable. The one-line bump is still outstanding, and `security/hashing_test.go` currently breaks package loading for the whole module, which is exactly the silent-pass condition noted above.

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

At the audited revision `go test` passed across `security`, `core/middleware`, `event`, `database`, and `database/local-store`. The findings in this report are design-level defects, not regressions — the code behaves as written.

~~**At `2c0802d3` the module no longer builds under test.**~~ `go build ./...` was clean, but `go vet ./security/...` failed with `undefined: GetCodebaseHashHex` (`security/hashing_test.go:76`), which blocked `go test ./...` for the whole module. **Restored at `392e17d2`**: `go build ./...`, `go vet ./...`, `go test ./...` and `go test -race ./...` are all clean on the merged tree. See WRP-39.

---

## Appendix B — Threat Model Summary

Closed findings are struck through; the capability rows themselves are unchanged, since WRP-05 still holds.

| Attacker | Capability assumed | Findings enabled |
|---|---|---|
| **Any Internet host** | Derive the public PSK and join the overlay (WRP-05) | ~~WRP-01~~, ~~WRP-02~~, ~~WRP-03~~, WRP-06, ~~WRP-10~~, ~~WRP-11~~, WRP-13, WRP-15, ~~WRP-16~~, WRP-23 |
| **Network-adjacent host** | Reach a hosted node's dashboard port | ~~WRP-04~~, WRP-18, WRP-25 |
| **Offline attacker** | Knows a target's peer ID and username, or holds a stolen DB volume | ⚠️WRP-07, ⚠️WRP-08, ~~WRP-22~~ |
| **Supply-chain attacker** | Compromises a release credential or third-party action | WRP-09, WRP-14, WRP-26, WRP-37 |
| **Co-located mobile app** | Reads public `Build` fields; launches exported activities | WRP-12, WRP-34 |
| **Local attacker** | Code execution on the user's machine | WRP-32, WRP-33, WRP-42 |

---

## 10. Remediation Verification Record

**Re-test 1:** 2026-08-16 against `ada3d0a0`, covering 68 commits since the audited revision `27ead3ce`.
**Re-test 2:** 2026-08-16 against `28eef1ef`, covering the WRP-04 remediation (`4957085b`, `6a93896a`).
**Re-test 3:** 2026-08-22 against `2c0802d3`, covering the 10 commits since `28eef1ef` (`24ad32d1`, `9f037278`, `fb714743`, `2ec79f38`, `67d94421`, `656fb962`, `37c60ebb`, `7e73b2f4`, `8404c0ef`, `2c0802d3`) and re-checking every finding still listed as open.
**Re-test 4:** 2026-08-22 against `392e17d2`, covering PR #458 (`f5baf2ab`, `8c222874`, `2413027d`) — the only change on `develop` since `2c0802d3`.

Every claim below was verified by reading the changed code; claims about exploitability were verified by execution.

### 10.1 Verification results

| ID | Original severity | Result | Basis |
|---|---|---|---|
| WRP-01 | Critical | ✅ **Closed** (r1) | Private-route gate enforced in `core/middleware/auth.go:82`; regression tests present |
| WRP-02 | Critical | ✅ **Closed** (r1) | Re-signing removed from `SelfPublish`; loopback reports the real sender |
| WRP-03 | Critical | ✅ **Closed** (r1) | `VerifyAuthorship` binds actor to `RemotePeer()` at 16 call sites |
| WRP-04 | Critical | ✅ **Closed** (r2) | Noise `XX` + login-gated key enrollment; adversarial test returns 401 on every privileged route |
| WRP-11 | High | ✅ **Closed** (r3) | `poll.go:282` rejects `optionsNum > 20`; `pollResults` is the only path to `Results` |
| WRP-16 | Medium | ✅ **Closed** (r3) | `RateLimiterMiddleware` per route+peer, wired ahead of dispatch on all three node types |
| WRP-21 | Medium | ✅ **Closed** (r1) | Committed password removed from all tracked YAML |
| WRP-22 | Medium | ✅ **Closed** (r1) | `AESKeyFromPassword`/`AESCodec` removed |
| WRP-27 | Medium | ✅ **Closed** (r3) | 32 bytes of `crypto/rand`; password no longer in the token seed |
| WRP-29 | Low | ✅ **Closed** (r1) | `Destination` now covered by `SigningBytes()` |
| WRP-40 | Info | ✅ **Closed** (r3) | `core/dht/dht.go:127` set to `dht.ModeAuto` |
| WRP-10 | High | ✅ **Closed** (r4) | Pre-allocation bounded by `maxPrealloc` independently of the caller's limit; `Search`/`WhoToFollow` capped at `MaxPageLimit` |
| WRP-07 | High | ⚠️ **Partial** (r3) | Argon2id via `security/kdf.go`; salt still public and deterministic, no rotation path, no migration for existing accounts |
| WRP-08 | High | ⚠️ **Partial** (r3, r4) | Moderator identity now random per process (r4); bootstrap seeds still committed and unrotated |
| WRP-17 | Medium | ⚠️ **Partial** (r3) | `PUBLIC_POST_REPORT` stream limited; `ReportsTopic` pubsub ingress unlimited, no dedup or threshold |
| WRP-39 | Info | ⚠️ **Partial** (r1, r3, r4) | Hashing helpers removed and the test build restored (r4); unwired route and event fields remain |

### 10.2 Confirmed still open at `392e17d2`

PR #458 touched only `cmd/node/{moderator,relay}/main.go`, `database/local-store/db.go`, `database/user-repo.go` and two test files, so every row below was re-confirmed unchanged at `392e17d2`; the evidence citations are from re-test 3.

| ID | Finding | Evidence |
|---|---|---|
| WRP-05 | PSK derived from public values | `security/psk.go:69-71` — now `SHA256(network‖major)`; documented as public by design, unchanged as a property |
| WRP-06 | No moderator trust root | `core/handler/moderation.go:103-116` still derives the key from the claimed `ModeratorID`; exhaustive search finds no allowlist |
| WRP-09 | Update checksum shares the artifact's trust domain | `core/selfupdate/*` — checksum listing only, no signature verification |
| WRP-12 | Android identity from public `Build` fields | `Ed25519IdentityStore.kt:31-67` unchanged |
| WRP-13 | 50 MiB pre-auth buffer, no read deadline | `core/node/node.go:239-241`; no `SetReadDeadline` on inbound remote streams; `core/stream/stream.go:252` still unbounded |
| WRP-14 | Snap workflow `write-all` + mutable action | `.github/workflows/snap.yml:13,30` unchanged |
| WRP-15 | Unfiltered gossip addresses dialed | `core/pubsub/gossip.go:634-636` passes `AddrInfo` straight through; `IsPublicMultiAddress` used only at `member-node.go:894` |
| WRP-18 | No login throttle or lockout | No attempt counter in `cmd/node/member/auth/auth.go`; bind still defaults to `0.0.0.0` (`config/config.go:74`) |
| WRP-19 | No `ConnectionGater` | Exhaustive search: no `ConnectionGater`, `InterceptSecured` or `InterceptAddrDial` in `core/` or `cmd/` |
| WRP-20 | No dimension bound before decode | `core/handler/image.go:373` still calls `image.Decode` with no `DecodeConfig` pre-check |
| WRP-23 | Relay service on every member | `core/node/options.go:62` unconditional `EnableRelayService` |
| WRP-24 | Root containers, host networking | No `USER` in `Dockerfile.remote`/`Dockerfile.moderator`; `network_mode: host` throughout `deploy/` |
| WRP-25 | Unauthenticated monitoring | `docker-compose.metrics.yaml` — no `GF_SECURITY_ADMIN_PASSWORD`; pushgateway on `:4091` |
| WRP-26 | Actions on mutable tags | Every `uses:` in `.github/workflows/` is tag-pinned; `golangci-lint-action@v8` still `version: latest` |
| WRP-28 | Predictable audit sampling RNG | `cmd/node/moderator/moderator/moderator.go:162` unchanged |
| WRP-30 | Non-constant-time token compare | `core/handler/pair.go:61` still uses `!=` |
| WRP-31 | Pairing token lifecycle | `database/auth-repo.go:98-100` — set once per process, no expiry or revocation |
| WRP-32 | Long-lived credential in `localStorage` | Re-scoped: `frontend/src/lib/noise.js:199-207` persists the client x25519 static private key |
| WRP-33 | No CSP | `frontend/public/index.html` has no `Content-Security-Policy` meta |
| WRP-34 | `REDIRECT_URL` extra honoured | `MainActivity.kt:548` unchanged |
| WRP-35 | `/` accepted in untrusted IDs | `database/local-store/prefix-tree.go` still concatenates without rejecting `/` |
| WRP-36 | One-hour connmgr grace period | `core/warpnet/warpnet.go:301-307` unchanged, limiter TODO still present |
| WRP-37 | Non-hermetic builds | Unverified `curl` of the Go tarball in `Dockerfile.{remote,member,moderator}`; `Dockerfile.moderator:24` still `-mod=mod` |
| WRP-38 | Deployment SSH hygiene | `deploy-mainnet.yaml:22-32`, `build-deploy-testnet.yaml:150-160` unchanged — `ssh-keyscan` TOFU, token into a root shell |
| WRP-41 | Hardcoded Echo credential | `cmd/node/member/echo-member.go:64-66` unchanged |
| WRP-42 | Anti-debugger theater | `security/anti-debugger.go:70` `TracerPid` check and `log.Fatalf` paths unchanged |
| WRP-43 | Idempotency covers only `/post/` | `core/middleware/idempotency.go:228` unchanged |
| WRP-44 | Broad `FileProvider` mappings | `file_paths.xml` still maps `external-path` and `cache-path` at `.` |

### 10.3 Assessment of the remediation

The three closed Critical findings were fixed at the root cause rather than patched at the symptom, which is the outcome worth noting. The private-route gate went into the middleware instead of into individual handlers; the authorship check became one shared helper rather than sixteen copies; and the gossip fix corrected the underlying identity confusion in the loopback stream instead of merely filtering routes. Each is covered by regression tests and the full suite passes. The signature-binding change (WRP-29) was a necessary companion to the WRP-02 fix, and shipping them together was correct.

**WRP-04 and the two-round pattern.** Round 1 fixed the defect the report cited (the plaintext fallback) while leaving the impact it described (unauthenticated owner authority) reachable, because the replacement authenticated the server to the client rather than the client to the server, and the dashboard password was retired without a successor. Round 2 closed it correctly. Two things made the difference and are worth carrying forward: the finding was scoped to an *outcome* rather than a line of code, so the gap was visible; and the re-test was an executed attack rather than a reading of the diff. A channel that is encrypted is not thereby authenticated, and this is an easy substitution to make under time pressure.

**The unfixed remainder is now the leading risk.** With Phase 1 complete, the highest-impact open findings are WRP-06 (any peer can forge moderation verdicts network-wide), WRP-13 (50 MiB buffered per stream before authentication, with no read deadline) and WRP-12 (Android device identity reconstructable from public `Build` fields).

### 10.4 Assessment of re-test 3

Four more findings closed and five moved to partial in six days, across the KDF, rate limiting, poll bounds, DHT mode and node seeds. Three observations.

**The Argon2id change is the right fix and resolves the inversion this report opened on.** Reusing the existing `deriveKey` helper rather than introducing a second KDF is the correct instinct, and the `warpnet/kdf/v1/...` context strings give real domain separation between the identity and database keys. What is missing is not cryptographic but operational: existing accounts derive different keys under the new scheme and land on `ErrWrongPassword`, so decide deliberately whether this ships as a breaking change or with a re-wrap flow.

**Two of the five partials are partial because the fix stopped at the cited line numbers.** WRP-10 named four locations; the two in `local-store` were clamped and the two in `user-repo.go` — which the finding also named, and where the element type is 240 bytes rather than a `ListItem` — were not. WRP-17's rate limit went on the stream route named in the finding, not on the pubsub topic the description identifies as the flood vector. This is the same pattern §10.3 recorded for WRP-04 round 1, and the same remedy applies: check the stated *impact* against the patched tree, not the stated *location*.

**Two changes need re-work before they count as fixes.** The moderator seed replacement allocates a zero-length slice, so the node exits at start-up rather than getting a random identity, and the dead-code removal left `security/hashing_test.go` calling a deleted function, so the module no longer builds under test. Both are one-line fixes, and both would have been caught by running `go build ./... && go test ./...` on the branch — worth adding as a gate given that CI runs `-race` over the whole module. *(Both fixed at `392e17d2`.)*

### 10.5 Assessment of re-test 4

PR #458 closed WRP-10 and cleared all three Phase 0 items. Two things are worth recording.

**WRP-10 closed by moving the bound, not by adding another clamp.** Re-test 3's fix capped the caller's `limit`; #458 caps the *capacity hint* instead and lets the limit bound only the iteration. That is the distinction the finding rests on — the iterator stops at real data, so an untrusted limit can safely govern how far you walk, but never how much you reserve. Capping the request instead had a cost the security review had not predicted: five internal paginators page at 100 and terminate on a short page, so every one of them silently stopped after 20 rows, including the offline-message outbox. A security control that changes a shared primitive's contract needs the callers checked, not just the attacker's path; the existing `TestOutboxRepoSuite` case was what surfaced it.

**Phase 0 existed because the remediation was not built and tested.** Three defects — a node that cannot start, a module that cannot compile under test, and a store that silently truncated every page — all reached `develop` in two commits, and all three were caught by `go build ./... && go test ./...`. That is the cheapest gate available and it is worth making non-optional on remediation branches specifically, since security fixes tend to touch primitives with many callers.

---

*All Critical findings are closed as of `28eef1ef`. At `392e17d2` this report describes **32 findings not fully remediated** — 9 rated High, of which 2 are partially addressed. The regressions introduced by the earlier remediation are resolved and the module builds and tests clean. The leading open risks are WRP-06, WRP-13 and WRP-12, all untouched since the original assessment.*
