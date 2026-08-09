---
name: warpnet-debug-backend
description: Use this skill when a Warpnet bug lives in the Go node/backend — the fat/member/remote node, its handlers, storage, or libp2p layer. Symptoms and triggers include "Transaction Conflict. Please retry" in node logs, the libp2p connection flapping every ~25-30s (yamux keep-alive), "context deadline exceeded" on a specific server RPC, a handler emitting the wrong or zero-value payload on the wire, gossip/timeline delivery failing (a followed user's tweets never arrive), CRDT stat double-counting, or BadgerDB MVCC / scan-then-write conflicts. Also use to verify the wire contract from the server side (test/api_sync_test.go) — the handler's return statement is the ground truth for what a client will parse. Do NOT use this skill for pure client rendering/parsing/UI bugs (use warpnet-debug-frontend), to add a new route or feature (use warpnet-add-handler), or to stand up / log into your own node (use warpnet-claude-node).
---

# Debugging backend bugs in Warpnet (Go node)

This skill is for bugs whose cause lives in the Go node — the handlers, BadgerDB storage, the libp2p/yamux transport, or the gossip/CRDT layer. The visible breakage is often reported by a client (blank list, dropped connection, a slow RPC), but when the cause is server-side — the wire-contract DTO the handler actually emits, the yamux config, BadgerDB MVCC semantics — this is the skill.

If your task is "add a new feature", you want `warpnet-add-handler` instead. If the node is fine and the client parses/renders/behaves wrong, you want `warpnet-debug-frontend`.

## Triage tree

Start at the **symptom** and walk down. Each leaf names the section below that documents the failure mode and the fix pattern.

```
Symptom: "Transaction Conflict. Please retry" in the fat-node logs
                                                                          ─→ § BadgerDB scan-then-write conflict

Symptom: connection drops periodically, period matches ~30s (yamux keep-alive)
                                                                          ─→ § Yamux config for relay-tunneled traffic

Symptom: "context deadline exceeded" on a specific server RPC
                                                                          ─→ § BadgerDB scan-then-write conflict OR the backend handler

Symptom: client shows blank / zero-value data while the same node serves the
         Vue desktop correctly — the payload is wrong on the wire
                                                                          ─→ the handler's return statement is ground truth;
                                                                             verify the wire contract — § Wire contract not covered by tests
```

Frontend-only symptoms — blank rows/fields that are a client-parse issue, missing avatars, UI jank, battery drain, a stale committed `.aar`, or the dashboard behaving "logged out" after a restart — are not this skill: use `warpnet-debug-frontend`.

## Cross-layer bugs — pull in `warpnet-debug-frontend` too

Most Warpnet bugs are cross-stack: the symptom surfaces in a client but the cause is one layer away — or the reverse. Do not assume a bug is purely server-side. **The moment a bug touches the wire (a blank/zero-value row, a field that never populates, a payload that "parses to nothing"), load `warpnet-debug-frontend` as well and work the two skills together.** Use this skill to pin the ground truth — what the handler's `return` actually emits, `test/api_sync_test.go`, the JSON bytes on the wire — and the frontend skill to pin the parse/render — the client DTO keys, Moshi / `JSON.parse` zero-values. The bug is wherever the two disagree. When you can't cleanly localize which side owns it, run both in sequence: confirm the server emits the field, then confirm the client reads it.

## § Yamux config for relay-tunneled traffic

**Symptom.** `network: event: peer ...UVdLFy connectedness updated: Limited → NotConnected → Limited` cycle every ~25-30 seconds in the fat-node logs. The cycle period matches yamux's `KeepAliveInterval`.

**Mechanism.** yamux ships with `KeepAliveInterval=30s` and `ConnectionWriteTimeout=10s`. When the connection is tunneled through a circuit-v2 relay (warpdroid → DigitalOcean relay → home router → desktop), the round-trip jitter for a keep-alive ping can spike above 10s under any congestion. When that happens, yamux concludes the peer is dead and tears the connection down. libp2p auto-reconnects via the same relay, the 30s idle starts again, and the cycle repeats.

**Where to look.**

- `warpdroid/node/node.go` libp2p options — the yamux muxer config.
- `core/node/options.go` on the fat-node side — same.
- The default `yamux.DefaultTransport.Config` only has `KeepAliveInterval=30s` / `ConnectionWriteTimeout=10s`.

**Fix pattern.** Build a custom yamux Config on both sides:

```go
ya := yamux.DefaultTransport
ya.KeepAliveInterval = 15 * time.Second        // ping more often than the cycle was
ya.ConnectionWriteTimeout = 30 * time.Second   // pong has slack to traverse relay
libp2p.Muxer(yamux.ID, ya)
```

Both sides must agree — yamux is symmetric, either party can tear down the connection.

**Anti-pattern.** Do not "fix" this by *disabling* keep-alive. Without it, a broken connection isn't detected until the next user-initiated request, which then hangs for the full stream-open timeout (~15s) before failing. Keep keep-alive on; just give pong room to traverse the relay.

## § BadgerDB scan-then-write conflict

**Symptom.** Backend logs show repeated `middleware: handling of ... failed: Transaction Conflict. Please retry` for write routes that update one record in a per-user prefix (mark-read, follow-update, similar).

**Mechanism.** Badger's SSI tracks every key the txn *reads*. If a writer scans a prefix list (~100 sibling keys) inside the same RW txn that later writes to one key, every concurrent writer that touches *any* key in that prefix is now a conflict candidate. Two concurrent mark-reads on *different* notifications both read the same prefix and write different keys — they commit-conflict on the second commit.

**Where to look.**

- `database/<feature>-repo.go` for any method matching this pattern:

```go
func (repo *FooRepo) UpdateOne(userId, fooId string) error {
    txn, _ := repo.db.NewTxn()
    defer txn.Rollback()
    for {
        items, _, _ := txn.List(prefix, ...)       // ← reads many keys
        for _, item := range items {
            if matches(item, fooId) {
                txn.SetWithTTL(item.Key, ...)      // ← writes one key
                return txn.Commit()
            }
        }
    }
}
```

**Fix pattern.** Split into two transactions:

1. **Find-key in a discardable RW txn.** `Rollback()` it. Dropping the txn drops every key from Badger's conflict table for this caller.
2. **Targeted write in a fresh RW txn.** Read just `{targetKey}` via `txn.Get`, modify, `txn.SetWithTTL`, `txn.Commit`. The read-set and write-set are both `{targetKey}` — disjoint from concurrent writers on other keys.

Concurrent writers on the **same** key still legitimately conflict — but if the update is monotonic (e.g. setting `IsRead=true` for a notification), the loser's view-after-commit matches the winner's, so the retry just observes the already-updated record.

**Real example.** `database/notification-repo.go::MarkRead` was the canonical victim. The fix is `findNotificationKey` (scan in a separate txn) → small write txn.

**Anti-pattern.** Wrapping the broken method in a `for attempt := 0; attempt < N; attempt++ {...}` retry loop. This *hides* the contention; under real concurrency it just shifts the failure to the last retry. Fix the read-set, don't wallpaper over it.

## § Wire contract not covered by tests

**Symptom.** A wire-format bug (silent zero-value parsing, see above) reaches a user-facing surface. `test/api_sync_test.go` was green.

**Mechanism.** The original `TestAPISync_Payloads` only diffs **request** bodies: it checks that the client sends keys the backend's input struct accepts. The mirror case — client *reads* keys the backend never emits — was uncovered. Most production wire bugs in this repo fall in the mirror direction.

**Fix pattern.** `TestAPISync_ResponsePayloads` (already in `test/api_sync_test.go`) walks each routed handler's body, picks out the success-path return type, resolves it via the same alias chain, and asserts the warpdroid parse-DTO's keys are a subset. Smoke-test it by temporarily reintroducing a known bug:

```bash
# Reintroduce the WarpnetNotification bug
sed -i 's|@Json(name = "user_id") val userId: String = "",|@Json(name = "from_user_id") val fromUserId: String = "",\n    @Json(name = "tweet_id") val tweetId: String? = null,|' \
    warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt

go test ./test/ -run TestAPISync_ResponsePayloads -count=1
# Should fail with: "PRIVATE_GET_NOTIFICATION: warpdroid reads keys the backend doesn't emit: [from_user_id tweet_id]"

git checkout warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt
```

**When adding a new route**, this test is the safety net — if you add a new DTO with phantom fields, the test catches it without needing a manual device test.

## Running your own fat node in Docker (testnet)

Most of the dances above start with *"log into the fat node first."* When you don't have
the user's desktop node in front of you, stand up your own headless **remote node** — the
`Claude` account on the `NODE_SEED=claude` node, in a Docker container on `testnet`. That
runbook (image build, the dedicated volume, account registration, the avatar, browser
login, teardown) lives in the **`warpnet-claude-node`** skill; load it when you need a live
node to reproduce a symptom against.

**Rule: always debug on your own node — never on the user's node or a throwaway account.**

To exercise the UI against this node (notifications triage, the base UI test plan, interacting as a user), see the `warpnet-debug-frontend` skill.

## Cheat sheet for the most common backend debugging dance

1. **Tail the fat-node logs.** If a write fails on the server, it logs `middleware: handling of <path> ... failed: <reason>`. That line present = server-side bug (you're in the right skill; read the handler). Absent, with the client still showing broken data = client-side parse / contract bug (that's `warpnet-debug-frontend`).
2. **Run `go test ./test/ -count=1`.** Catches wire-contract drift without needing a device.
3. **Look at the actual JSON bytes.** Add a `log.Infof("DEBUG response: %s", string(b))` near the handler return; tail the logs. This dispels 90% of "the DTO must be wrong" guesses inside a minute — the handler's return statement is the ground truth of what's on the wire.
4. **Don't fix contention with retries.** Retry loops around a conflicting write, or "increase the timeout", hide rather than fix. Find the actual contention and fix the read-set.
5. **BadgerDB read-set discipline.** Never scan a prefix in the same RW txn that writes one key; split find-key (discardable txn) from the targeted write so the read-set and write-set are disjoint.

## When this skill doesn't apply

- **Client rendering / parsing / UI bugs** (blank rows that are a client-parse issue, avatars, jank, battery, stale `.aar`) → use `warpnet-debug-frontend`.
- **Standing up / logging into / tearing down your own node** (Docker, the `Claude` account, the dashboard "logged out" after a restart) → use `warpnet-claude-node`.
- **Adding a new route or feature** → use `warpnet-add-handler`.
