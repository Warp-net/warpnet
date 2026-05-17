---
name: warpnet-debug-stack
description: Use this skill when an existing Warpnet feature behaves wrong end-to-end and the cause might span layers — symptoms reported on warpdroid or the Vue desktop UI ("notifications are empty / blank rows", "avatars don't load", "timeline freezes", "the connection keeps flapping every 30s", "context deadline exceeded", "Transaction Conflict. Please retry", "images don't appear", "replies show blank rows", "drawer shows @warpnet.local"), or about the cross-language wire contract ("DTO mismatch", "Moshi defaults to blank", "the test passed but the device shows empty"). Triggers include phrases like "fix the X bug on warpdroid", "X is broken on Android but works on desktop" / "works on desktop but broken on Android", "diagnose why this is failing", "the request never reaches the server", "the response parses to blanks". Do NOT use this skill to add a new route or feature — for that use [warpnet-add-handler]. Do NOT use this skill for pure-Tusky UI tweaks that don't cross the Warpnet boundary.
---

# Debugging cross-stack bugs in Warpnet

Most Warpnet bugs are not in the code that visibly broke. The visible breakage is a client-side symptom (blank list, frozen tab, missing avatar, connection drop) but the cause lives one or two layers below: the wire-contract DTO, the libp2p/yamux config, BadgerDB MVCC semantics, the gomobile binding lifecycle, or the Glide / Moshi pipelines that translate bytes into UI state.

This skill is the triage tree and the runbook for those bugs. If your task is "add a new feature", you want `warpnet-add-handler` instead. This skill is for **untangling an existing feature that doesn't work**.

## Triage tree

Start at the **symptom** and walk down. Each leaf names the section below that documents the failure mode and the fix pattern.

```
Symptom: rows appear blank / empty / "No notifications yet" / "No replies"
└─ does the Vue desktop show the same data with the same node? ────── YES ─→ § Silent zero-value DTO parsing
                                                                ──── NO  ─→ check backend Storer / handler
                                                                            (this skill is the wrong tool)

Symptom: connection drops periodically, period matches ~30s
                                                                          ─→ § Yamux config for relay-tunneled traffic

Symptom: "Transaction Conflict. Please retry" in backend logs
                                                                          ─→ § BadgerDB scan-then-write conflict

Symptom: every request hangs ~15s before timing out, even simple ones
                                                                          ─→ § Stream-call serialisation bottleneck

Symptom: image/avatar shows placeholder; Glide logs FileNotFoundException
         with a hex-looking path
                                                                          ─→ § Glide can't dial a content-addressed blob

Symptom: Kotlin code change has no effect on device
                                                                          ─→ § Stale .aar (gomobile binding not regenerated)

Symptom: "context deadline exceeded" appears intermittently on a specific RPC
└─ only on warpdroid? ─── YES ─→ § Stream-call serialisation bottleneck OR § Yamux config
                       ─── NO  ─→ § BadgerDB scan-then-write conflict OR backend handler

Symptom: tests pass but device behaves stale
                                                                          ─→ § Stale .aar OR § Wire contract not covered by tests
```

When in doubt, log into the **fat node** first (`stdout` of the desktop binary). The middleware logs every failed handler call by route, so an empty client list paired with no server-side errors almost always means the client is parsing a successful response wrong (silent zero-values).

## § Silent zero-value DTO parsing

**Symptom.** Client shows empty lists or rows with blank fields, but the desktop Vue UI on the same node shows the data correctly. No error in logs.

**Mechanism.** Moshi (warpdroid) / `JSON.parse` (Vue) silently default any missing field to the zero value. When a client DTO declares fields the backend doesn't actually emit, every payload parses successfully into a default-valued object, then downstream code skips it (`mapNotNull { author ?: return null }`) or renders it as blank.

**Where to look.**

1. The **handler return statement** in `core/handler/<feature>.go`. Not the DTO file, not the client adapter — the literal `return event.X{...}` or `return event.X(value), nil` line. This is the ground truth of what's on the wire.
2. Resolve aliases. `event.FooResponse = domain.Foo` means the wire shape is `domain.Foo`, not whatever the client thinks `FooResponse` is.
3. The Kotlin DTO in `warpdroid/warpnet-transport/.../dto/WarpnetDtos.kt` or the Vue field reads after `await this.sendToNode(...)` in `frontend/src/service/service.js`.

**Real examples in this repo:**

- `WarpnetNotification` claimed `from_user_id` and `tweet_id`. `domain.Notification` only has `{id, type, text, user_id, is_read, created_at}`. Moshi parsed `from_user_id` to `""`; `resolveUser("")` returned null; every notification got `mapNotNull`-filtered out and the screen said "No notifications yet" while the desktop showed a full list.
- `RepliesResponse.replies` was `List<WarpnetTweet>` but the wire carries `[]domain.ReplyNode = {reply, children}`. Every reply parsed into a default `WarpnetTweet` (blank `id`/`text`/`user_id`) and the children tree was silently discarded.
- `TweetStatsResponse.tweets_count` was declared but the wire never sent it; the field stayed at `0` forever.

**Fix pattern.**

1. Read the handler's actual return type.
2. Align the client DTO field-for-field: every JSON tag on the Go struct, no extras.
3. Drop any client-side logic that depends on the phantom fields (e.g. the `resolveUser(fromUserId)` step in the notification mapper).
4. Add a regression-protecting subtest in `test/api_sync_test.go` (see `TestAPISync_ResponsePayloads`). If the test doesn't already cover this route, audit-extend it.

**Prevention.** `test/api_sync_test.go::TestAPISync_ResponsePayloads` walks each handler's success-path return, resolves the response struct via the same event→domain alias chain, and asserts that the client DTO's keys are a subset of the backend's wire keys. Re-run it whenever any of the three layers (`event.go`, `domain/warpnet.go`, `WarpnetDtos.kt`) changes:

```
go test ./test/ -run TestAPISync_ResponsePayloads -count=1
```

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

## § Stream-call serialisation bottleneck

**Symptom.** UI feels frozen on warpdroid — timeline refresh blocks every other action, opening notifications spins, sending a message hangs. The logs show `failed to open stream: context deadline exceeded` after exactly the stream timeout (15-30s), once per call, in sequence.

**Mechanism.** libp2p's `host.NewStream` and yamux multiplex concurrent substreams over a single connection — both are thread-safe. Older Warpnet code (and any "let's be careful" rewrites) wrapped `binding.stream(...)` in a mutex on both the Go binding (`c.mu.Lock()` in `clientNode.stream`) and the Kotlin client (`mutex.withLock { binding.stream(...) }` in `WarpnetClient.request`). A single slow call then blocks every other RPC for the duration of its timeout, and the symptoms compound across UI screens.

**Where to look.**

- `warpdroid/node/node.go` — `stream()` should hold the lock for the minimum needed to read `desktopPeerID`, then release it before `c.host.NewStream` and the I/O.
- `warpdroid/warpnet-transport/.../WarpnetClient.kt` — `request()` should not wrap `binding.stream` in `mutex.withLock`.

**Fix pattern.**

```go
// Go side
func (c *clientNode) stream(protocolID string, data []byte) ([]byte, error) {
    c.mu.RLock()
    desktopPeerID := c.desktopPeerID
    c.mu.RUnlock()
    if desktopPeerID == "" { return nil, fmt.Errorf("not connected") }
    // ... rest of the method runs lock-free; libp2p + yamux handle concurrency
}
```

```kotlin
// Kotlin side — no withLock around binding.stream
suspend fun request(protocolId: String, bodyJson: String): String =
    withContext(Dispatchers.IO) {
        if (_state.value !is ConnectionState.Connected) throw NotConnected()
        // ... build envelope ...
        binding.stream(protocolId, requestJson)
    }
```

**Connect / disconnect** still need the write lock to swap `desktopPeerID`, but they're rare.

## § Glide can't dial a content-addressed blob

**Symptom.** Profile / drawer / timeline avatars are blank. Glide logs `FileNotFoundException: /b8f69bbd…ENOENT (No such file or directory)`. The "path" is a 64-hex content hash.

**Mechanism.** `WarpnetMapper` was pasting the raw avatar blob key straight into `Account.avatar` / `account.header`. Glide's default String-URI loader sees a string that looks like a relative path, prepends `/`, calls `ContentResolver.openInputStream` on it, and fails. There's no local file — the blob lives on the fat node, behind `PUBLIC_GET_IMAGE`.

**Fix pattern.**

1. Add a `getImageBytes(userId, key)` repo method that calls `PUBLIC_GET_IMAGE` and decodes the `"<mime>,<base64>"` payload (Vue inlines the same string into an `<img src>` data URL).
2. In `WarpnetMapper`, emit a synthetic URL: `"warpnet://avatar/$userId/$key"` (empty string when key is blank).
3. Register a Glide `ModelLoader<String, ByteBuffer>` that:
   - matches strings starting with `warpnet://avatar/`,
   - parses out `(userId, key)`,
   - calls `repo.getImageBytes(...)` in a `runBlocking { ... }` (Glide invokes `DataFetcher.loadData` on its background executor, so blocking is fine),
   - hands the bytes to Glide via `ByteBuffer.wrap(bytes)`.
4. `registry.prepend(...)` in the `AppGlideModule` so the custom loader runs **before** Glide's built-in content-URI resolver.

The repo singleton reaches Glide via a Hilt `@EntryPoint`:

```kotlin
@EntryPoint
@InstallIn(SingletonComponent::class)
interface WarpnetGlideEntryPoint {
    fun warpnetRepository(): WarpnetRepository
}

// inside AppGlideModule.registerComponents:
val ep = EntryPoints.get(context.applicationContext, WarpnetGlideEntryPoint::class.java)
registry.prepend(String::class.java, ByteBuffer::class.java, Factory(ep.warpnetRepository()))
```

Once registered, every existing avatar call site (drawer header, `loadAvatar()` helper, timeline view holders, fullscreen viewer) works transparently. No call-site changes needed.

## § Stale .aar (gomobile binding not regenerated)

**Symptom.** A Go-side change in `warpdroid/node/*.go` has no effect on the device. Or: CI build succeeds but the new Java/Kotlin signature references `node.Node.someMethod` that the AAR doesn't have.

**Mechanism.** `warpdroid/warpnet-transport/libs/warpnet.aar` is **committed** in source control. CI does **not** run `gomobile bind`. Every change to `warpdroid/node/*.go` requires the developer to regenerate the AAR locally via `warpdroid/node/build-native.sh` and commit the resulting `.aar` and `.jar`.

**Fix pattern.**

1. Run `cd warpdroid/node && ./build-native.sh` locally. Requires Go 1.26+, gomobile, Android NDK.
2. Commit `warpdroid/warpnet-transport/libs/warpnet.aar` and `warpnet-sources.jar` alongside your `.go` change. Binary-only diff for the .aar is normal.

**Stop-gap when AAR can't be rebuilt in CI.** If your Kotlin change references a new Go-side export but the committed AAR is stale (common during reviews), give the interface method a **default implementation** that uses an existing AAR method:

```kotlin
interface WarpnetBinding {
    // …
    fun connectedness(): String =
        if (isConnected()) "Connected" else "NotConnected"   // ← uses old export
}
```

Then `DefaultBinding` doesn't have to override until the new AAR lands. CI compiles, runtime works (degraded), and the override drops in cleanly once the AAR is regenerated.

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

## § Stream retries — what's safe to retry

**Pattern.** Network failures fall in two categories: those that mean **the request never reached the server**, and those that mean **the server may have processed it**. Retrying the first is always safe. Retrying the second can double-apply a mutation.

**Retryable (request never reached server):**

- `"failed to open stream"` — NewStream errored before any bytes flowed.
- `"not connected to desktop node"` — pre-flight failure.
- `"context deadline exceeded"` on stream-open phase.
- `"stream reset"` / `"muxer closed"` — connection torn down before write.

**Not retryable (server may have processed):**

- `"stream: reading response from ..."` — write succeeded, response failed mid-flight. Handler may have run.
- `"stream: writing"` — partial write. Status unknown.
- `"context deadline exceeded"` after the body was fully sent.

**Implementation.** In `WarpnetClient.request()`, walk the retryable categories, retry with bounded back-off (500ms, 1.5s, 4s, 8s is reasonable), rebuild and re-sign the envelope on each attempt (so the timestamp on the wire matches the freshly-sent body):

```kotlin
private fun isRetryableTransportError(msg: String): Boolean {
    if (msg.contains("stream: reading response", ignoreCase = true)) return false
    return msg.contains("failed to open stream", ignoreCase = true) ||
        msg.contains("not connected to desktop node", ignoreCase = true) ||
        msg.contains("context deadline exceeded", ignoreCase = true) ||
        msg.contains("stream reset", ignoreCase = true) ||
        msg.contains("muxer closed", ignoreCase = true)
}
```

Combine with the POST idempotency middleware (`core/middleware/idempotency.go`) for true at-most-once delivery on mutating endpoints.

## § Lifecycle on Kotlin, not on Go

When a feature needs ongoing state — auto-reconnect, polling, badge updates, link-quality monitoring — resist the urge to add it to the Go binding. The gomobile interface is string-in / string-out; every Go callback across JNI is fragile, and the binding's global singleton state is a single point of failure.

**Pattern.** Go exposes a **snapshot** function. Kotlin polls it and owns the state machine.

Example: instead of a Go-side `network.Notifiee` that fires reconnect goroutines, the Go binding gained one new export:

```go
func Connectedness() string  // "Connected" | "Limited" | "NotConnected"
```

The Kotlin `ConnectionMonitor` polls every 2s, exposes a `StateFlow<LinkState>`, and runs the reconnect loop (back-off `1s, 2s, 5s, 10s, 30s`) using existing `connectAny(addrs)` on the client. The reconnect addresses come from the app layer via a `suspend () -> List<String>` provider, so the transport module stays decoupled from `PairedNodeStore`.

This keeps:
- the Go binding thin (one new function, no goroutines, no state),
- lifecycle decisions in Kotlin where the Android-lifecycle hooks already live (`ProcessLifecycleOwner`, Hilt scope, coroutines),
- testability — the `ConnectionMonitor` is plain Kotlin with an injected fake binding.

**Anti-pattern.** Push notifications from Go to Kotlin via long-blocking calls (`nextEvent() string` that blocks until something happens). Works in theory, fragile across JNI thread boundaries, hard to cancel cleanly on shutdown.

## Cheat sheet for the most common debugging dance

1. **Tail the fat-node logs.** If a write fails on the server, it logs `middleware: handling of <path> ... failed: <reason>`. Absent that line and the client still shows broken data = client-side parse / contract bug. Present that line = server-side bug (this skill is the wrong tool; read the handler).
2. **Run `go test ./test/ -count=1`.** Catches wire-contract drift without needing a device.
3. **Diff against Vue.** Vue is the older client and usually has the canonical behaviour. If Vue shows X and warpdroid shows blank-X, the bug is on warpdroid's parse or render side, not on the server.
4. **Look at the actual JSON bytes.** Add a `log.Infof("DEBUG response: %s", string(b))` near the handler return; tail the logs. This dispels 90% of "the DTO must be wrong" guesses inside a minute.
5. **Don't fix symptoms with retries.** Retry loops, `try/catch { ignore }`, "increase the timeout" — these hide rather than fix. Find the actual contention or contract bug.

## When this skill doesn't apply

- **Adding a new route or feature** → use `warpnet-add-handler`.
- **Pure-Tusky UI changes** (a screen, a Compose layout, an XML resource) that don't cross the Warpnet boundary → no skill needed, treat as a normal Android task.
- **Pure DB schema changes** with no caller yet → not a debugging task; design with the future caller in mind.
- **Moderation, admin, pair handshake bugs** → these have extra ceremony around challenge/PSK; read `core/handler/admin.go`, `pair.go`, `moderation.go` first. The general triage tree still applies but the fix surface is narrower.
