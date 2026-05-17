---
name: warpnet-add-handler
description: Use this skill whenever the task is to add a new feature to Warpnet that crosses layers — anything described as a "new endpoint", "new route", "new RPC", "new event", "new path", or any feature that needs a frontend (Vue desktop or Android/Tusky) to talk to the local node and/or one node to talk to another. Triggers include phrases like "add /private/get/...", "add a handler for X", "expose Y to the UI", "make nodes exchange Z", "expose X to warpdroid", "add support for X in the Android client". Also use when the change is Android-only but touches a Warpnet protocol ID (new ProtocolIds entry, new DTO, new WarpnetRepository method). Do NOT use this skill for pure-Tusky-side UI changes that don't touch any Warpnet protocol, pure DB-schema changes that no one calls, or for fixing existing handlers without adding new routes.
---

# Adding a new handler to Warpnet

A handler in Warpnet is one entry in a multi-layer pipeline. Adding one means touching all of these layers in lockstep — skipping any single layer leaves the feature dead. The pipeline is fixed; treat it as a checklist, not a suggestion.

There are now **two client-side pipelines** that talk to a desktop node: the in-process Vue UI (the original member app) and the Android client (`warpdroid/`). They share the **same** server-side handler — there is one canonical handler per route, registered once in `member-node.go`. They differ only in how the request reaches the node:

```
                    ┌─────────────────────────────┐
                    │  desktop "fat" member node  │
                    │   cmd/node/member/...       │
                    │   core/handler/<feature>.go │   ← ONE canonical handler
                    │   database/<feature>-repo.go│
                    └──────────────▲──────────────┘
                                   │
                       ┌───────────┴────────────┐
                       │                        │
        in-process Wails Call          libp2p stream over Noise
                       │                        │
                       │                        │  paired desktopPeerID
   Vue app (frontend/) │                        │  (one connection)
   service.js          │                        │
   ├─ const PUBLIC_…   │                        │
   └─ sendToNode()  ───┘                        │
                                                │
                              warpdroid/warpnet-transport (Kotlin)
                                ├─ ProtocolIds  ← MUST mirror event/paths.go
                                ├─ WarpnetEnvelope ← MUST mirror event.Message
                                ├─ dto/WarpnetDtos ← Moshi DTOs mirroring event.go
                                ├─ EnvelopeSigner (NoOpSigner today — see §A4)
                                └─ WarpnetClient.request(protocolId, bodyJson)
                                          │
                              warpdroid/node (Go module, separate go.mod)
                                ├─ node.go : libp2p thin client
                                └─ mobile.go : gomobile string-only API
                                          │
                              [gomobile bind → warpnet.aar]
                              [.aar is COMMITTED, not built in CI]
                                          │
                              warpdroid/app (Tusky fork, Kotlin/Java)
                              └─ com.keylesspalace.tusky.warpnet.WarpnetRepository
                                  ├─ calls client.request(...)
                                  └─ maps WarpnetX ↔ Tusky entities (Status, Account, Notification)
```

Both pipelines:
- always wrap the inner payload in an `event.Message` envelope (`{body, message_id, node_id, path, timestamp, version, signature}`);
- use the **same** path string (e.g. `/public/post/like/0.0.0`);
- hit the **same** Go handler on the desktop side.

What changes per route:

- **All routes work with the desktop UI** — Vue can hit any route registered in `member-node.go`.
- **A subset of routes work with warpdroid today.** Only the protocol IDs explicitly listed in `warpdroid/warpnet-transport/.../ProtocolIds.kt` are wired through, and only those routes whose middleware doesn't strictly verify the envelope signature are usable end-to-end (see §A4 — the gomobile binding does not yet expose the signing key).

## Step 0 — Do not skip context discovery

Before writing a line of code, read these files in this order. Each is small enough to fit in a single view:

**Server side (always):**

1. `cmd/node/member/app.go` — see `App.Call` and the special-case for `PRIVATE_POST_LOGIN` / `PRIVATE_POST_LOGOUT`. Everything else hits the `default:` branch and goes through `node.SelfStream`.
2. `event/paths.go` — naming convention for routes.
3. `core/handler/like.go` — the cleanest small reference. ~215 lines. Read top to bottom.
4. `core/handler/like_test.go` — stub style used everywhere; no testify.
5. `database/like-repo.go` — `Storer` interface + `NewTxn`/`defer Rollback`/`Commit` pattern.
6. `cmd/node/member/node/member-node.go` lines ~370–510 — the registration block.

**Vue desktop client (only if the route is meant to be callable from the desktop UI):**

7. `frontend/src/service/service.js` — top of file for path constants, `likeTweet` (~620) for the calling pattern, `sendToNode` (~880) for the wrapper.

**Android client (only if the route is meant to be callable from warpdroid):** see also §A below.

8. `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/ProtocolIds.kt` — the path mirror.
9. `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/WarpnetEnvelope.kt` — envelope mirror.
10. `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/WarpnetClient.kt` — `request()` (~180) and `pair()` (~148) for the calling pattern.
11. `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt` — the DTO catalogue (only routes wired through warpdroid).
12. `warpdroid/app/src/main/java/com/keylesspalace/tusky/warpnet/WarpnetRepository.kt` — how Tusky-shaped entities are produced from Warpnet DTOs.
13. `warpdroid/app/src/main/java/com/keylesspalace/tusky/warpnet/WarpnetMapper.kt` — the lossy mapping rules between Warpnet and Mastodon-shaped types.

If the feature touches a resource that already exists (replies, follows, chats…), also read the matching `core/handler/<that>.go` — you will reuse its repo or DTO.

**Decide upfront which clients the route is for.** Only the desktop UI? Only warpdroid? Both? This determines which of the steps below you must do. Server-side steps (1–5, 7) are always required. Step 6 (Vue) is required only if the desktop UI should call the route. Section §A (warpdroid) is required only if the Android client should call the route.

## Step 1 — Choose the path

Routes follow `/{visibility}/{verb}/{resource}/{version}` exactly.

- **visibility** is `private` or `public`.
  - `private` = only the local UI calls this (talks to its own node only). Examples: `PRIVATE_GET_TIMELINE`, `PRIVATE_POST_TWEET`, `PRIVATE_DELETE_CHAT`, `PRIVATE_GET_STATS`.
  - `public` = nodes call this on each other over libp2p. Examples: `PUBLIC_POST_LIKE`, `PUBLIC_GET_USER`, `PUBLIC_POST_FOLLOW`.
  - If unsure: does a remote peer ever need to invoke this? Yes → `public`. No → `private`.
- **verb** is `get`, `post`, `delete`. (No `put`, no `patch` — Warpnet does not use them.)
- **resource** is short, lowercase, one word if possible. Plural for collections (`tweets`, `users`), singular for items (`tweet`, `user`).
- **version** is `0.0.0` for new routes. Bump only when you make a breaking wire change to an existing route — do not bump just because the file changed.

The same constant is declared in **two or three** places, depending on which clients use it. All declarations must hold the same string byte-for-byte:

- Go: `event/paths.go` — always. Alphabetical within its visibility block.
- JS: `frontend/src/service/service.js` — required if the Vue desktop UI calls the route. Top of file.
- Kotlin: `warpdroid/warpnet-transport/.../ProtocolIds.kt` — required if warpdroid calls the route. Grouped under one of the existing `// admin / pairing`, `// public reads`, `// public writes`, `// private reads`, `// private writes` headers.

If any pair of these strings drifts, the call silently goes to the `default:` of `App.Call` and fails with `not attached server node` or with handler-not-found. There is no compile-time link between the three.

## Step 2 — Define the DTOs

In `event/event.go`, alphabetically among the other types:

```go
// FooEvent defines model for FooEvent.
type FooEvent struct {
    TweetId domain.ID `json:"tweet_id"`
    UserId  domain.ID `json:"user_id"`
}

// FooResponse defines model for FooResponse.
type FooResponse struct {
    Count uint64 `json:"count"`
}
```

Rules:
- IDs use `domain.ID`, not `string`. Other primitive fields use plain Go types.
- JSON tags are `snake_case`. Field names in Go are `CamelCase`.
- Reuse existing types where possible. `event.UnlikeEvent = LikeEvent` is the precedent — if your delete event is shape-identical to the create event, alias it.
- If the response is just a count, reuse `LikesCountResponse`. Don't invent a parallel type.
- Put the godoc one-liner `// FooEvent defines model for FooEvent.` — every existing type has it; linters may rely on it.

## Step 3 — Add or extend the repo

If a repo for this resource already exists (`database/<resource>-repo.go`), add a method to it. If not, create a new file modelled on `like-repo.go`.

Required pattern for any write:

```go
func (repo *FooRepo) DoSomething(id, userId string) (uint64, error) {
    if id == "" {
        return 0, local_store.DBError("empty id")
    }
    if userId == "" {
        return 0, local_store.DBError("empty user id")
    }

    key := local_store.NewPrefixBuilder(FooRepoName).
        AddSubPrefix(SomeSubNamespace).
        AddRootID(id).
        Build()

    txn, err := repo.db.NewTxn()
    if err != nil {
        return 0, err
    }
    defer txn.Rollback()

    // ... txn.Get / txn.Set / txn.Delete / txn.Increment ...

    if err := txn.Commit(); err != nil {
        return 0, err
    }
    return n, nil
}
```

Non-negotiable repo rules:
- Validate empty IDs at the top with `local_store.DBError("empty ...")`. Same wording style as the other repos — it ends up in error messages users see in logs.
- Always `defer txn.Rollback()` immediately after `NewTxn`. Rollback after a successful Commit is a no-op, this is intentional.
- Always end the success path with `txn.Commit()`. Any branch that returns without committing is a bug.
- Use `local_store.IsNotFoundError(err)` to test for not-found, never `err == ...` or `errors.Is` against datastore errors.
- `repo.statsDb` is **optional** — guard with `if repo.statsDb == nil { return ... }` exactly like in `like-repo.go`. Stats failures are warnings, not errors: `log.Warnf(...)` and continue.
- Do **not** introduce a new repo dependency on something other than `LikeStorer`-shaped interfaces. Repos depend on storage, not on each other.

Define a sentinel error if callers need to distinguish "not found" from real errors:
```go
var ErrFooNotFound = local_store.DBError("foo not found")
```

Add a `_test.go` next to it. Check the existing `like-repo_test.go` for the test harness — they use a real BadgerDB in a temp dir, no mocks.

## Step 4 — Write the handler

File: `core/handler/<feature>.go`. Use `like.go` as the template literally — copy the AGPL header from any existing handler file (the exact 22-line block ending with `// SPDX-License-Identifier: AGPL-3.0-or-later`, then `package handler`).

Skeleton:

```go
// Storer interfaces are declared at the top of the file, named after the handler:

type FooStorer interface {
    DoSomething(id, userId string) (uint64, error)
    // ...minimal surface only — do not pass *FooRepo around
}

type FooStreamer interface {
    GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
    NodeInfo() warpnet.NodeInfo
}

func StreamFooHandler(
    repo FooStorer,
    streamer FooStreamer,
) warpnet.WarpHandlerFunc {
    return func(buf []byte, s warpnet.WarpStream) (any, error) {
        var ev event.FooEvent
        if err := json.Unmarshal(buf, &ev); err != nil {
            return nil, err
        }
        if ev.TweetId == "" {
            return nil, warpnet.WarpError("foo: empty tweet id")
        }
        // ...more validation, in the order the fields appear in the struct...

        n, err := repo.DoSomething(ev.TweetId, ev.UserId)
        if err != nil {
            log.Errorf("foo handler failed: %v", err)
            return nil, err
        }

        // For PUBLIC routes that propagate to the resource owner's node:
        // 1. resolve target node id via userRepo
        // 2. streamer.GenericStream(targetNodeId, event.PUBLIC_POST_FOO, ev)
        // 3. swallow ErrNodeIsOffline — return local result anyway
        // 4. unmarshal possible ResponseError from the remote and log it

        return event.FooResponse{Count: n}, nil
    }
}
```

Hard rules:
- Use the project's `json` package: `import "github.com/Warp-net/warpnet/json"` (NOT `encoding/json`). The aliased `jsoniter` is fine inside `app.go` because it's the dispatcher; in handlers use `json`.
- Logger: `log "github.com/sirupsen/logrus"`. Use `log.Errorf` for handler-level failures, `log.Warnf` for non-fatal degradation, `log.Infof` only for one-shot lifecycle events.
- Error wrapping: handler-internal errors are prefixed with the handler name in lowercase: `warpnet.WarpError("foo: empty tweet id")`. This is so log greps work.
- Handlers receive `warpnet.WarpStream` as the second arg. Most handlers ignore it. Do not close it manually — the framework owns its lifecycle.
- Return `(any, error)`. Returning a typed event struct is the norm; the framework marshals it.
- For PUBLIC handlers that call out to another node, follow the like.go propagation pattern verbatim:
  - `errors.Is(err, warpnet.ErrNodeIsOffline)` is **not an error from the caller's perspective** — return the local result.
  - After a remote call, try to unmarshal `event.ResponseError` from the response and log if it's non-empty. Don't surface it as an error to the local caller.

The Storer interfaces declared at the top of the handler file are **handler-local** — each handler declares only the methods it needs from the repo. This is intentional. Don't replace them with a shared interface.

## Step 5 — Register the handler

In `cmd/node/member/node/member-node.go`, find the `m.node.SetStreamHandlers([]warpnet.WarpStreamHandler{...})` block (~line 385). Add an entry **in the same group as related routes** — like is near unlike, follow is near unfollow. Don't sort alphabetically; sort by feature affinity. The existing block uses positional struct literals (`{event.PUBLIC_POST_LIKE, handler.StreamLikeHandler(...)}`), preceded by `//nolint:govet`. Match that style.

If your handler needs a new repo, instantiate it in the same function, near where the other repos are constructed (~line 370):
```go
fooRepo := database.NewFooRepo(db, statsDB)
```
`statsDB` is only relevant if you increment counters; otherwise just `db`.

## Step 6 — Wire the Vue frontend

Skip this step if the route is warpdroid-only. Otherwise, two edits in `frontend/src/service/service.js`:

1. **Path constant**, top of file with the others:
   ```js
   export const PUBLIC_POST_FOO = "/public/post/foo/0.0.0"
   ```
   Same string as the Go constant, byte-for-byte.

2. **Method on the `warpnetService` object**, modelled on `likeTweet` (~line 623):
   ```js
   async fooSomething(tweetId, userId) {
       const owner = this.getOwnerProfile()
       const request = {
           path: PUBLIC_POST_FOO,
           body: {
               tweet_id: tweetId,
               user_id: userId,
               owner_id: owner.user_id,
           },
       }
       const resp = await this.sendToNode(request)
       return resp.count
   }
   ```
   Body keys are `snake_case` and must match the Go struct's `json` tags exactly. `sendToNode` adds `message_id`, `node_id`, `timestamp` automatically — do not set them in `request`.

3. The Vue component imports `warpnetService` and calls `await warpnetService.fooSomething(...)`. No backend awareness in components.

## §A — Wire the warpdroid Android client

Skip this section if the route is desktop-only. Otherwise, the work falls in three submodules and (sometimes) the Go binding:

```
warpdroid/node/                 → Go libp2p thin client + gomobile binding
warpdroid/warpnet-transport/    → Kotlin Warpnet wire layer (envelope, DTOs, ProtocolIds)
warpdroid/app/...tusky/warpnet/ → Tusky-side adapter (WarpnetRepository, WarpnetMapper)
```

The thin client connects to **one** desktop peer (the user's paired fat node) over a single libp2p connection, and tunnels every request through `binding.stream(protocolId, json)`. There is no DHT lookup per call, no multi-peer fanout, no local Badger storage on the device — Android is purely a client of the user's own desktop node.

### A1. Decide if the binding needs to change

The Go binding (`warpdroid/node/`) is a *separate Go module* (`module github.com/Warp-net/android-binding`) with its own `go.mod` and its own dependency tree. It does **not** import anything from the parent `warpnet` module. It exposes only six entry points (`Initialize`, `Connect`, `Stream`, `PeerID`, `IsConnected`, `Disconnect`, `Pause`, `Resume`, `Shutdown`) — all string-in, string-out, because gomobile-bind cannot translate complex types across the JNI.

The binding needs to change **only if**:
- you need a new lifecycle entry point (e.g. exposing the private key for signing — see §A4);
- your route requires per-stream timeouts different from the binding's hard-coded 30s.

For a routine "add new Warpnet route" task, **the binding does not change**. You don't recompile `warpnet.aar`. You add the route in Kotlin, mirror DTOs, write a `WarpnetRepository` method.

### A2. Add the path constant in Kotlin

In `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/ProtocolIds.kt`, add the constant under the matching group header (`// public reads`, `// private writes`, etc.). Same string as the Go constant.

### A3. Mirror the DTOs in Moshi

In `warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt`, add `@JsonClass(generateAdapter = true)` data classes for the request payload and the response shape. Rules:

- Field names in Kotlin are `camelCase`. JSON wire names use `@Json(name = "snake_case_name")` when they differ.
- The wire names must match the Go struct's `json:"..."` tags exactly.
- Default values where the Go side treats a missing field as zero-value (string `""`, number `0`, boolean `false`). This avoids Moshi throwing on responses that happen to omit optional fields.
- Strings for IDs (`String`, not custom types) — the Go side uses `domain.ID` which is itself a string alias.
- Time fields are RFC3339 strings on the wire — keep them as `String` in Kotlin DTOs and convert at the mapper boundary if needed.
- Only add DTOs the Tusky side actually uses. The DTO file's docstring states this rule explicitly: don't bulk-mirror `event.go` — add exactly what `WarpnetRepository` needs.

If the response is a paged collection, mirror both the wrapper (`{ items, cursor }`) and the inner items.

### A4. Decide signing

Every request goes through `WarpnetClient.request(protocolId, bodyJson)`, which builds a `WarpnetEnvelope` and asks the injected `EnvelopeSigner` to sign the body. Today only `NoOpSigner` exists — it throws `WarpnetException.SigningUnavailable` because the gomobile binding does not export the Ed25519 private key.

This means at the time of writing, **routes whose middleware verifies the envelope signature do not work end-to-end through warpdroid.** Check `core/middleware/middleware.go` (around the line where signature verification happens) for which routes are gated.

What to do depends on your route:
- **Read-only public routes that the middleware does not gate** (the bulk of `PUBLIC_GET_*` calls — that's why `WarpnetRepository` covers timeline/user/replies/stats today): no signing concerns. Proceed normally.
- **Write routes or signature-verified routes**: the route will fail at runtime with `WarpnetException.SigningUnavailable` until the binding is extended. Either land your code with a TODO comment referencing `EnvelopeSigner.kt` (it already documents the problem in detail), or — if your task is specifically to fix this — extend `mobile.go` with either `Sign(body) string` or `SetIdentityKey(pem) string`, then implement `BindingSigner` in Kotlin and register it via Hilt.

Do not invent a parallel signing mechanism. The envelope's signature must match what the Go middleware verifies (Ed25519 over the body bytes, base64-encoded).

### A5. Add a `WarpnetRepository` method (Tusky-facing)

In `warpdroid/app/src/main/java/com/keylesspalace/tusky/warpnet/WarpnetRepository.kt`, add a `suspend fun` modelled on `getStatus`/`favouriteStatus`/`reblogStatus` (~145–290). The pattern:

```kotlin
suspend fun fooSomething(tweetId: String, userId: String): TuskyShapedReturn {
    val raw = client.request(
        ProtocolIds.PUBLIC_POST_FOO,
        fooEventAdapter.toJson(FooEvent(tweetId = tweetId, userId = userId)),
    )
    val resp = fooResponseAdapter.fromJson(raw)
        ?: throw IllegalStateException("foo returned empty body for $tweetId")
    return resp.toTuskyShape()
}
```

Rules:
- `client.request(...)` is the only allowed transport — never reach into `WarpnetClient` internals or call the binding directly.
- Adapters are `private val fooAdapter = moshi.adapter<FooEvent>()` declared near the top of the class (~80–100). Add yours alongside the existing ones.
- Convert to/from Tusky entities (`Status`, `Account`, `Notification`, `Relationship`, `TimelineAccount`) via extension functions in `WarpnetMapper.kt`. Don't return raw Warpnet DTOs from `WarpnetRepository` — the rest of the Tusky app only knows Mastodon-shaped types.
- Use `runCatching { ... }.getOrNull()` for optional sub-fetches that should degrade gracefully (e.g. fetching stats alongside a tweet). Hard `throw` for anything required.
- Throwing `IllegalStateException` for empty bodies is the established style.

### A6. Extend the mapper if shapes don't fit

In `WarpnetMapper.kt`, add `fun WarpnetX.toTuskyY(): Y` extensions for any new wire→Tusky conversion. The mapper file's header docstring explains the philosophy: Mastodon types are richer than Warpnet types (rich HTML content, attachments, visibility, emojis), so when translating Warpnet→Tusky pick safe defaults (public visibility, no attachments, empty emoji list). Don't invent fields that the wire didn't send.

When translating **Tusky→Warpnet** (e.g. for a `postStatus` type call), the Tusky side carries fields the Warpnet side ignores — drop them silently, don't `throw`.

### A7. The .aar is committed; gomobile is NOT in CI

This is the single most surprising thing about the warpdroid build. The CI workflow (`.github/workflows/build-warpdroid.yml`) runs:

```
./gradlew build -x test -x lint -x lintAnalyze* -x lintReport*
./gradlew :app:assembleDebug
```

It does **not** invoke `gomobile bind`. The compiled `warpnet-transport/libs/warpnet.aar` is checked into the repository. This means:

- Pure-Kotlin changes (new `ProtocolIds`, new DTO, new `WarpnetRepository` method) are CI-buildable as-is.
- Changes to `warpdroid/node/*.go` require a **manual** rebuild via `warpdroid/node/build-native.sh`, which produces `warpnet.aar`. The script must be run locally; CI will not catch a stale .aar.
- Commits that change Go code under `warpdroid/node/` **must also commit** the regenerated `warpnet-transport/libs/warpnet.aar` (and `warpnet-sources.jar`). Reviewers should expect both. A binary-only diff for the .aar is normal.
- The `warpdroid/node/` Go module has its own `go.mod` with `go 1.26.0` and pulls dependencies directly (no vendor). Building it requires Go 1.26+, gomobile, and Android NDK on the developer's machine.

### A8. Where Java/Kotlin code goes — Tusky vs warpnet-transport

The Android app is a fork of Tusky with a Warpnet integration grafted on. Two rules of thumb for where new code belongs:

- **Tusky-namespaced code** (`com.keylesspalace.tusky.*`) is the existing Mastodon client. Touch it only when the feature surface is UI/UX (e.g. a new screen that consumes `WarpnetRepository`). Stay close to existing patterns — Hilt for DI, coroutines for async, paging library for lists.
- **warpnet-transport-namespaced code** (`site.warpnet.transport.*`) is the wire layer. Anything that knows about Warpnet protocol IDs, envelopes, signatures, or libp2p semantics goes here. The Tusky side must never see a `ProtocolIds.X` or build a `WarpnetEnvelope`.

The split exists so that the Tusky fork can be re-merged with upstream Tusky changes without touching Warpnet wire concerns, and so that Warpnet wire concerns can be tested independently of Android.

### A9. Tests on the warpdroid side

Tests are JVM tests (`src/test/kotlin/`), JUnit + standard Kotlin. The transport module ships a fake `WarpnetBinding` that returns canned strings — use it instead of touching the real gomobile binding. Verify:
- the envelope wraps the body correctly (path, version, signature placeholder, body verbatim);
- the Moshi DTO round-trips with the Go-side wire format (paste a representative JSON sample from a captured Vue request to make sure tags align);
- error paths surface `WarpnetException.ProtocolError` with the correct code/message when the binding returns an error JSON;
- mapper edge cases (missing optional fields → safe defaults).

Do not write instrumented Android tests for new routes — the wire layer is plain JVM.

## Step 7 — Server-side tests

A new handler is incomplete without a `_test.go`. Style is dictated by the existing tests in `core/handler/`:

- Plain `testing` package, no testify.
- `package handler` (white-box, not `handler_test`).
- File starts with `//nolint:all` — the existing tests do this.
- Stubs are local structs with `<method>Fn func(...) (...)` fields, falling back to a default if `nil`. See `stubLikeRepo` in `like_test.go` for the canonical shape.
- Subtests with `t.Run("invalid payload", func(t *testing.T) {...})`. Always cover at least: invalid payload, each empty-field validation, the happy path, and remote-call branches if any.
- Helper `marshal(t, ...)` is already defined in `like_test.go` and reused — if you need it and it's not in package scope, copy it.

The repo also has `core/handler/notifications_e2e_test.go` for end-to-end style; only add to that level if the feature is genuinely cross-handler.

## Step 8 — Pre-flight checklist before commit

Before `git commit`, verify each of these. Missing any one is a real bug, not a style nit. Skip the warpdroid block if the route is desktop-only; skip the Vue block if the route is warpdroid-only.

**Always (server side):**

- [ ] AGPL header present at top of every new `.go` file. Copy it verbatim from `core/handler/like.go` lines 1–26 (the 22-line block plus `// Copyright ...` and `// SPDX-License-Identifier: AGPL-3.0-or-later`).
- [ ] Path constant in `event/paths.go`.
- [ ] DTO in `event/event.go` with the `// XxxEvent defines model for XxxEvent.` godoc.
- [ ] Repo method validates empty inputs, uses `NewTxn` + `defer Rollback` + `Commit`.
- [ ] Handler registered in `cmd/node/member/node/member-node.go`, near related routes, with `//nolint:govet`.
- [ ] Handler validates every input field before touching the repo.
- [ ] `_test.go` covers invalid payload + each empty-field error + happy path.
- [ ] Run `go test -short -p 8 -v ./core/handler/... ./database/...` locally (this is what `make tests` does for the whole repo).
- [ ] `go vet ./...` clean. `golangci-lint run` clean (CI runs it; config in `.golangci.yaml`).
- [ ] `vendor/` not modified. If `go mod` accidentally touched it, `git checkout vendor/`.
- [ ] `version` file: **do not bump manually** — the `pre-commit` githook auto-bumps the patch on non-`main` branches and updates `snap/snapcraft.yaml`. Make sure `make setup-hooks` has been run (`git config core.hooksPath .githooks`).

**If the route is callable from the Vue desktop UI:**

- [ ] Same path string in `frontend/src/service/service.js`, byte-identical with `event/paths.go`.
- [ ] Service method on `warpnetService` exists, parameter list matches what components will call, body keys are `snake_case` matching the Go struct's `json` tags.

**If the route is callable from warpdroid:**

- [ ] Same path string in `warpdroid/warpnet-transport/.../ProtocolIds.kt`, byte-identical with the other two.
- [ ] DTOs added to `warpdroid/warpnet-transport/.../dto/WarpnetDtos.kt` with `@Json(name = "snake_case")` for any field whose Kotlin name differs.
- [ ] `WarpnetRepository` method added, returns Tusky-shaped types (not raw Warpnet DTOs).
- [ ] If the response shape doesn't fit any existing Tusky entity, mapper extension added in `WarpnetMapper.kt` with safe defaults for fields the wire doesn't carry.
- [ ] If the route requires envelope signature verification (most write routes): added a TODO comment referencing `EnvelopeSigner.kt`'s known limitation, OR landed the binding extension first.
- [ ] If `warpdroid/node/*.go` was changed: regenerated `warpnet-transport/libs/warpnet.aar` via `warpdroid/node/build-native.sh` and committed both the .aar and the .jar.
- [ ] JVM tests added for envelope shape and DTO round-trip; instrumented tests not needed.
- [ ] `./gradlew :app:assembleDebug` succeeds locally (CI runs the same).

**Always (general):**

- [ ] Branch name follows `IssueNumber/AmazingFeature`. Commit message imperative, present tense.
- [ ] PR description names the path you added so reviewers can grep it across all three sources.

## Common mistakes specific to this codebase

- **Adding the path string only in some places.** The Go, JS, and Kotlin path constants are independent strings. The compiler does not link them. A typo or omission = silent failure with `not attached server node` in logs (Go side) or a 30-second hang then a `TransportFailure` (Android side). When adding a route used by both clients, grep all three: `event/paths.go`, `frontend/src/service/service.js`, `warpdroid/warpnet-transport/.../ProtocolIds.kt`.
- **Using `encoding/json` instead of the project's `json` package.** Tests will pass, runtime may pass, but you'll diverge from project-wide marshalling settings (the project uses `jsoniter` under the hood for performance and number handling).
- **Forgetting `//nolint:govet`** on the registration entry. The block uses positional struct literals which `govet` doesn't like. Match neighbours.
- **Putting `domain.ID` fields where `string` belongs or vice versa.** IDs in events are `domain.ID`. In handler logic they're often used as `string` — `domain.ID` is a typed string alias, the conversion is implicit.
- **Calling `streamer.GenericStream` for a `private` route.** Private routes never propagate. Only `public` routes call out to other nodes. If you wrote a private handler that needs to call a peer, you've split the wrong way — that should be a public route.
- **Treating `ErrNodeIsOffline` as a real error.** It isn't. The local write succeeded; the propagation failed; return the local result and move on. See `like.go` for the exact branch.
- **Bumping the `version` file by hand.** The pre-commit hook does it. Doing it manually creates a conflict with the hook.
- **Modifying `vendor/`** directly or via `go get` without intent. `vendor/` changes need to be a deliberate `make update-deps` commit, separate from feature work.
- **Adding a `put` or `patch` verb.** Not used in the project. Use `post` for create-or-update, `delete` for delete.

**Warpdroid-specific:**

- **Forgetting that the .aar is committed.** A pure-Kotlin change builds and runs in CI even with a stale .aar — meaning local Go changes won't be visible until you regenerate. Conversely, if you change `warpdroid/node/*.go` and forget to rebuild the .aar, your changes ship to no one. The .aar is a build artefact in source-control by convention, treat it as a deliberate commit.
- **Importing from the parent `warpnet` Go module inside `warpdroid/node/`.** They are separate Go modules. The thin client must stay self-contained: libp2p, multiaddr, dht. Pulling in `warpnet/event` or `warpnet/domain` would balloon the .aar by tens of MB and break gomobile.
- **Returning Warpnet wire DTOs from `WarpnetRepository`.** The Tusky side (`com.keylesspalace.tusky.*`) must only see Mastodon-shaped entities. Confine `WarpnetTweet`, `WarpnetUser`, etc. to the `com.keylesspalace.tusky.warpnet` package and translate at the boundary.
- **Building a `WarpnetEnvelope` outside `WarpnetClient`.** Envelope construction (message_id, version, timestamp, signature) is centralised. If your call needs a non-standard envelope, the right move is to teach `WarpnetClient` about the variant — not to reach around it.
- **Calling the gomobile binding methods directly from Tusky code.** All transport goes through `WarpnetClient`. The binding's global singleton state and the mutex in `WarpnetClient` are paired — bypassing the client risks racing pause/resume against in-flight requests.
- **Returning Tusky-shaped types from `warpnet-transport`.** The transport module must not depend on Tusky entities. The split is enforced by Gradle module boundaries — if you find yourself needing a Tusky import in `site.warpnet.transport.*`, the conversion belongs in `WarpnetMapper`, not in the transport.

## When the task is moderation, admin, or pair-related

These have extra ceremony — see `core/handler/admin.go`, `core/handler/pair.go`, `core/handler/moderation.go`. They use challenge/response signing and PSK plumbing that regular handlers don't. Read those files before treating an admin-route task as "just another handler".
