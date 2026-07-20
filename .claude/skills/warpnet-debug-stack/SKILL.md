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

Symptom: warpdroid UI feels janky / Davey frames / 1-2 s freeze on a screen
└─ ANR or true hang? ──── YES ─→ § Stream-call serialisation bottleneck
                       ──── NO  ─→ § warpdroid UI perf (low-end device budget)

Symptom: warpdroid drains the battery / device gets warm in pocket / CpuBgTime
         climbs in BatteryStats / "the app keeps the radio on"
                                                                          ─→ § warpdroid battery budget
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

## § warpdroid UI perf (low-end device budget)

**Symptom.** warpdroid feels laggy. Opening Profile / Home / a tweet detail takes 1-2 s of frozen UI before the screen renders. `Choreographer: Skipped N frames` and `OpenGLRenderer: Davey! duration=...ms` lines in logcat. Not an ANR, not a network hang — the request and the data are fine, the *rendering* is what's slow. Often worse on a budget phone than on a flagship.

**Context.** warpdroid targets low-end Android: 4-core ARM, 2-3 GB RAM, 60 Hz (16 ms frame budget). Many things that are invisible on a flagship are visible jank here. The codebase carries Tusky's XML-era legacy plus a partial Compose migration — both layers have patterns that get expensive at this hardware tier.

**Where to look (logcat, with the app in debug build — StrictMode is already configured):**

1. `Choreographer: Skipped N frames` — N × 16 ms is the freeze duration. >5 frames is user-visible.
2. `Davey! duration=Xms` — a frame that took >700 ms. The accompanying `FrameTimelineVsyncId=...` block names the offending render pass.
3. `StrictMode policy violation; ~duration=N ms: android.os.strictmode.DiskReadViolation` — synchronous IO on the main thread. The full stack names the offender. Common ones in this repo: `MaterialDrawer.ImageHolder.applyTo → ImageView.setImageURI` on `warpnet://` (~70 ms), `EncryptedSharedPreferences` first-read (~300-450 ms at cold start), `EmojiPackList.<init>` (~100 ms).
4. `Compiler allocated N KB to compile <fully-qualified composable name>` — first-compose JIT cost for a Compose function. Common spikes: `TweetCard` (~5.5 KB), `TweetButtons` (~4.5 KB). Adds 200-500 ms each to the first LazyList measure.
5. `Quality: stackInfo` lines — periodic main-thread sampling. Names the exact composable / layout pass that was hot when the frame stalled.

**Common causes in this repo:**

- **`ConstraintLayout` in Compose inside a `LazyList` item.** Solver + double-measure pass multiplies per row. `TweetButtons` used to be a 5-button `ConstraintLayout` chain — replaced with `Row + Arrangement.SpaceBetween` cut Profile-open Davey from 2 s to <500 ms. If a new screen uses `ConstraintLayout` and it's measured per LazyList item, that's almost certainly the cause.
- **`ImageView.setImageURI(warpnet://...)`** via MaterialDrawer's `iconUrl` or any naïve `Uri`-binding call. `setImageURI` ultimately calls `Drawable.createFromPath` → blocking `FileInputStream.open`. Use Glide via `WarpdroidAsyncImage` or load into a known `ImageView` reference manually (`Glide.with(view).load(url).into(view)`).
- **`Glide.with(activity)` instead of `Glide.with(applicationContext)` or `Glide.with(imageView)`.** Activity-scoped Glide requests crash `IllegalArgumentException` when the activity is destroyed mid-load (back-press during decode). See the `WarpdroidAsyncImage` runCatching wrap.
- **First-compose JIT.** Inherent to Compose, but cumulative cost grows with composable size and nesting. Mitigations: keep hot composables small, split into smaller leaves so JIT amortises across recompositions, simplify `TweetCard` / `TweetButtons` / `Avatar` if they regress. Long-term fix is a Baseline Profile (not currently set up).
- **`ANIMATE_GIF_AVATARS` pref ignored on a new Glide call site.** When animated, Glide returns a `Drawable` (heavier); when off, `Bitmap`. Inconsistent behaviour vs the rest of the app is a UX bug, but using `.asDrawable()` everywhere also burns memory on the budget tier. Mirror `loadDrawerAvatar`'s split.

**Diagnostic workflow:**

1. Capture logcat for a 5-10 s window around the slow interaction.
2. Filter for `Davey|Choreographer|StrictMode|Compiler allocated|Quality.*stackInfo|ANR_LOG`.
3. Time-correlate the lines. Davey frame at T → look for StrictMode / JIT / stackInfo at T-100ms to T+100ms.
4. The stack trace in `StrictMode` violations is exact — it names the line. The composable in `Compiler allocated` is exact. Don't guess.
5. After a fix, re-capture and confirm the relevant line is gone or its duration dropped below the frame budget.

**Don't chase symptoms with cosmetic fixes** (spinners, skeletons) before identifying which of the categories above is the actual cause. A spinner that hides a 2 s freeze means the freeze still happens — battery still drained, scroll still stutters when the LazyList recycles.

## § warpdroid battery budget

**Symptom.** Users report warpdroid drains the battery noticeably faster than other social apps. Device is warm in the pocket. `BatteryStats` / OEM `AppStats` show high `CpuBgTime`, `WakeLockBgTime`, or `BgWifiRx/TxBytes` for `site.warpnet.warpdroid` over an idle hour. Or: a code review finds a new `ForegroundService`, a polling loop, a `WakeLock`, or a new periodic worker without constraints.

**Context.** warpdroid is a thin client to the user's own desktop fat node. Almost every "always-on" architecture pattern from regular Android social apps is wrong here:

- The fat node is always-on, so the device does not need to be.
- Notifications come from the fat node via a `WorkManager`-scheduled pull (`NotificationWorker`, 15 min minimum), not from a persistent socket on the device.
- The libp2p host is paused on `Lifecycle.onStop` and resumed on `onStart` — by design, when the user isn't looking at warpdroid, the radio isn't holding the relay-tunneled keep-alive.

The relay-tunneled libp2p connection is the single most expensive thing the app does. yamux's keep-alive cycle plus the circuit-v2 relay's own pings keep the cellular/wifi radio out of doze. Leaving the host alive in the background is a ~3-5%-per-hour drain on a budget device.

**Logcat fingerprints (OEM AppStats lines, emitted every few minutes by `com.oplus.persist.system` / `com.miui.powerkeeper` / etc.):**

- `CpuFgTime` / `CpuBgTime` — fg should dominate; bg climbing means background work is running.
- `WakeLockFgTime` / `WakeLockBgTime` — must be 0 across the board. Non-zero = something is holding a `PowerManager.WakeLock`.
- `JobFgCount` / `JobBgCount` — `NotificationWorker` + `PairRefreshWorker` together should fire ~4 times/hour at most. More = a new periodic worker landed without a 15-min floor.
- `BgWifiRxBytes` / `BgWifiTxBytes` — if these climb while the device is idle and the screen is off, there's a polling loop still firing.
- `GpsTime` / `SensorTime` / `BgCameraTime` — must be 0. warpdroid does not use any of these. Non-zero = a leaked listener.

**Common causes in this repo (or things to grep for in a new change):**

- **`ForegroundService` introduced "to keep the connection alive".** Wrong by architecture. The lifecycle pause/resume of `binding.Pause()`/`binding.Resume()` is the design — don't add a service that fights it. Grep `Manifest.xml` for `<service` and any class extending `Service` / `LifecycleService`. The only legitimate service in warpdroid right now is `SendTweetBroadcastReceiver`'s upload, which is single-shot.
- **`Handler.postDelayed(refresh, 30_000)` or `LaunchedEffect { while(true) { delay(30s); ... } }`.** Polling loops keep the app's process awake. Grep for `postDelayed`, `while (true)`, `repeat(`, `flow.collect` with a hand-rolled `delay`. The Compose `LaunchedEffect` ones are especially insidious because they auto-cancel only on screen leave — if the user keeps the screen open they run forever.
- **A new `PeriodicWorkRequest` under 15 min or without constraints.** `WorkManager.MIN_PERIODIC_INTERVAL_MILLIS` is 15 min and is the absolute floor — anything shorter is coerced silently. New workers must have `setRequiredNetworkType(NetworkType.CONNECTED)`, `setRequiresBatteryNotLow(true)`, and `BackoffPolicy.LINEAR` (or `EXPONENTIAL`). Match `PairRefreshWorker.kt` and `NotificationWorker.kt`.
- **A `PowerManager.WakeLock`, `WifiManager.WifiLock`, or `View.setKeepScreenOn(true)` call.** There is no legitimate reason for any of these in warpdroid. Grep `WakeLock`, `WifiLock`, `setKeepScreenOn`, `FLAG_KEEP_SCREEN_ON`. If you find one, remove it and fix the underlying state machine that wanted it.
- **`AlarmManager.setExactAndAllowWhileIdle` or anything that wakes the device.** warpdroid doesn't need exact-time alarms. The only legitimate batched alarms come from `WorkManager`'s `SystemJobScheduler`. Grep `AlarmManager`.
- **A leaked `BroadcastReceiver` or `ContentObserver`.** Receivers registered in `Application` or a long-lived singleton without an `unregister` keep their callback's process alive. Grep `registerReceiver` and pair every match with a corresponding `unregisterReceiver`.
- **A `ConnectivityManager.NetworkCallback` not unregistered.** Same story.
- **A new permission in `AndroidManifest.xml` for GPS / Bluetooth / mic / camera-always-on.** warpdroid currently uses camera only on `PairingActivity` (QR scan), and that's intentionally torn down after the scan. New permissions need an explicit design discussion — every permission is a battery vector.
- **`Ed25519IdentityStore.derive` called more than once per process.** It's ~600 ms of single-core CPU. Once is fine (the auto-pair path). A loop, or a per-message call, will both drain battery and block the main thread.

**Diagnostic workflow:**

1. Get a baseline. With a known-good build, leave the device idle for 1 hour with the app installed but not foregrounded. Capture the `Battery AppStats: Uid = <warpdroid uid>` lines from logcat at start and end. Record `CpuBgTime`, `WakeLockBgTime`, `BgWifiRxBytes`, `BgWifiTxBytes`, `JobBgCount`.
2. Repeat on the suspect build, same conditions.
3. Compare. Any field rising by more than a few percent is a regression. Find what changed in the last branch that runs in the background or holds a system resource.
4. Use `adb shell dumpsys batterystats --reset` then `adb shell dumpsys batterystats site.warpnet.warpdroid` after 30+ min to get a structured per-component breakdown if the AppStats lines aren't enough.
5. For wakelock leaks specifically: `adb shell dumpsys power | grep -A 5 warpdroid` will list any wakelocks currently held.

**Architectural reminder:** warpdroid is a thin client. It does not need to be "alive" between user interactions. Any pattern that fights that — keeping the libp2p host warm, pre-fetching, eager caching, push-via-persistent-socket — is wrong by design. When in doubt, do less in the background.

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

## Running your own fat node in Docker (testnet)

Most of the dances above start with *"log into the fat node first."* When you don't have
the user's desktop node in front of you, stand up your own headless **business node**
(`cmd/node/business` — the same binary `warpnet-testnet-verify` uses) in a Docker
container on `testnet`. It boots the whole stack (BadgerDB, auth, libp2p host, every
handler) and serves the dashboard `/ws` bridge on a port you can drive or open in a
browser. This gives you a live node to reproduce a symptom against without touching the
user's machine.

> Sandbox note: outbound egress here is limited to TCP 80/443, so a lone node stays
> `network_state: Disconnected` / `peers_online: 0` — it can't reach the real testnet
> bootstrap peers on `:4011/:4022/:4033`. That does **not** block single-node handler
> debugging: every `self-stream` route is answered locally regardless of peers. For
> cross-node reproduction use the local swarm in `warpnet-testnet-verify`.

**Rule: always debug on your own node — the `Claude` account on the `NODE_SEED=claude`
node — never on the user's node or a throwaway account.** The point is a stable, known
fixture: same `node_id`, same `Claude` user, same avatar, same data every session, so
symptoms are reproducible and you never mutate the user's real state while poking at a
bug. To make that fixture survive container restarts and image rebuilds, the node **must**
mount its **own dedicated Docker volume** for `/root/.warpdata` (the BadgerDB store where
the account, profile, avatar, tweets, and node identity live). Without a volume the DB
lives in the container's writable layer and is lost on `docker rm` — you'd re-register and
lose the avatar every rebuild. The identity is deterministic (`username+password+network`
→ same key) so a wipe still yields the same `node_id`, but the profile/avatar and any
seeded content only persist on the volume. One volume, one account, reused everywhere.

### 1. Build the image from the working tree

The build uses `Dockerfile.business` (Go toolchain from `go.dev` over 443, vendored
modules, embeds `frontend/dist`). Run from the repo root:

```bash
docker build -f Dockerfile.business -t warpnet-business:claude .
```

### 2. Run the container on testnet

`deploy/docker-compose-testnet.yml` is the reference for the env vars each node takes.
The `NODE_SEED` env fixes the node's deterministic libp2p ID (`config.go`:
`node.seed` ← `NODE_SEED`); `NODE_SERVER_PASSWORD` is the dashboard `/ws` AES secret
(and is **required** — an empty one is `log.Fatal`). Use `NODE_SEED=claude`, and mount the
node's **own** named volume at `/root/.warpdata` so the `Claude` account persists:

```bash
docker volume create warpnet-claude-testnet-data   # idempotent; the node's dedicated store

docker run -d --name warpnet-claude-testnet \
  -e NODE_NETWORK=testnet \
  -e NODE_SEED=claude \
  -e NODE_SERVER_PASSWORD='Claude1234$' \
  -e NODE_SERVER_PORT=4999 \
  -e NODE_PORT=4001 \
  -e LOGGING_LEVEL=info \
  -e LOGGING_FORMAT=json \
  -v warpnet-claude-testnet-data:/root/.warpdata \
  -p 4999:4999 \
  warpnet-business:claude

# wait for the HTTP server, then confirm liveness (always 200 — see below)
until curl -sf -o /dev/null localhost:4999/healthz; do sleep 1; done
```

Because the volume is dedicated and named, you can `docker rm` / rebuild the image and
`docker run` again with the same `-v warpnet-claude-testnet-data:/root/.warpdata` — the
`Claude` account, avatar, and any seeded data come straight back. Register the account (§3)
**once**; every later session reuses it.

Or, to keep it alongside the other testnet services, add a service to
`deploy/docker-compose-testnet.yml` (build locally rather than pulling a ghcr image):

```yaml
  claude-testnet:
    container_name: claude-testnet
    build:
      context: ..
      dockerfile: Dockerfile.business
    network_mode: host
    restart: always
    environment:
      - NODE_PORT=4077
      - NODE_NETWORK=testnet
      - NODE_SEED=claude
      - NODE_SERVER_PORT=4999
      - NODE_SERVER_PASSWORD=Claude1234$
      - LOGGING_LEVEL=info
      - LOGGING_FORMAT=json
    volumes:
      - warpnet-claude-testnet-data:/root/.warpdata   # dedicated, persists the Claude account

# and at the file's top level:
volumes:
  warpnet-claude-testnet-data:
```

**Liveness ≠ readiness.** `/healthz` and `/readyz` are hard-coded to 200; the libp2p node
and its handlers don't exist until someone logs in over `/ws`. Any routed call before
login returns `{"code":500,"message":"not attached server node"}`.

### 3. Register the `Claude` account and set the avatar

The account identity is deterministic from `username + password + network`. Log in over
`/ws` to register it (first login on a fresh DB creates it):

- **username:** `Claude`
- **password:** `Claude1234$` (public; passes the policy — upper/lower/digit/special, 8–32 chars)

The avatar is a two-step flow that mirrors the Vue client
(`frontend/src/service/service.js`: `uploadImages` → `editMyProfile`):

1. `POST /private/post/image/0.0.0` with `{image1:"data:image/png;base64,<logo>", image2..4:""}`
   → returns `{key1,...}` (the handler re-encodes to JPEG and stores it; `core/handler/media.go`).
2. `POST /private/post/user/0.0.0` with `{username:"Claude", avatar_key:"<key1>"}` — sets
   `domain.User.AvatarKey` on the owner profile.

Use the repo's own mark, `cmd/node/member/icon.png`, as the logo (or any PNG/JPG).

The `/ws` bridge accepts **plaintext** JSON frames (`AESCodec.Decode` falls back to
plaintext on decrypt failure — no need to reimplement the browser's AES layer). Drive it
with a throwaway Go probe using the vendored `gorilla/websocket` (delete it after — never
commit it):

```bash
mkdir -p cmd/wsprobe && cat > cmd/wsprobe/main.go <<'EOF'
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Body        json.RawMessage `json:"body"`
	MessageId   string          `json:"message_id"`
	Destination string          `json:"path"`
	Timestamp   time.Time       `json:"timestamp"`
	Version     string          `json:"version"`
	Signature   string          `json:"signature"`
}

func main() {
	logoPath := "cmd/node/member/icon.png"
	raw, err := os.ReadFile(logoPath)
	if err != nil {
		fmt.Println("read logo:", err)
		return
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)

	c, _, err := websocket.DefaultDialer.Dial("ws://localhost:4999/ws", nil)
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer c.Close()

	call := func(dest string, body any) json.RawMessage {
		b, _ := json.Marshal(body)
		out, _ := json.Marshal(Msg{Body: b, MessageId: dest, Destination: dest, Timestamp: time.Now(), Version: "0.0.0"})
		_ = c.WriteMessage(websocket.TextMessage, out)
		_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, data, err := c.ReadMessage()
		if err != nil {
			fmt.Printf("[%s] read err: %v\n", dest, err)
			return nil
		}
		fmt.Printf("[%s] %s\n", dest, string(data))
		var m Msg
		_ = json.Unmarshal(data, &m)
		return m.Body
	}

	call("is-first-run", nil)
	call("/private/post/login/0.0.0", map[string]string{"username": "Claude", "password": "Claude1234$"})
	time.Sleep(4 * time.Second) // let the libp2p node attach

	imgResp := call("/private/post/image/0.0.0", map[string]string{"image1": dataURL})
	var img struct {
		Key1 string `json:"key1"`
	}
	_ = json.Unmarshal(imgResp, &img)
	if img.Key1 == "" {
		fmt.Println("no avatar key returned")
		return
	}
	call("/private/post/user/0.0.0", map[string]string{"username": "Claude", "avatar_key": img.Key1})
}
EOF

go run -mod=vendor ./cmd/wsprobe
rm -rf cmd/wsprobe   # ALWAYS remove — never commit the probe
```

A non-empty `key1` in the image reply and a returned user with `avatar_key` set means the
avatar is live. Open `http://localhost:4999` in the session browser and log in as
`Claude` / `Claude1234$` to see it.

### 4. Teardown

```bash
docker rm -f warpnet-claude-testnet          # stop/remove the container...
# ...but KEEP the volume — it holds the Claude account + avatar for next session:
docker volume ls | grep warpnet-claude-testnet-data

# Only if you deliberately want a clean first-run/register again:
# docker volume rm warpnet-claude-testnet-data
```

Remove the **container** freely; keep the **volume**. Re-running the `docker run` in §2 with
the same `-v warpnet-claude-testnet-data:/root/.warpdata` brings the account straight back —
no re-register, avatar intact. Wipe the volume only when you explicitly want a fresh
first-run.

Confirm `git status --porcelain` is clean — the probe package must never land in a commit.

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
