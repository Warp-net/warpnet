---
name: warpnet-add-handler
description: Use this skill whenever the task is to add a new feature to Warpnet that crosses layers — anything described as a "new endpoint", "new route", "new RPC", "new event", "new path", or any feature that needs the frontend to talk to the local node and/or the local node to talk to a remote node. Triggers include phrases like "add /private/get/...", "add a handler for X", "expose Y to the UI", "make nodes exchange Z". Do NOT use this skill for pure-frontend changes, pure DB-schema changes that no one calls, or for fixing existing handlers without adding new routes.
---

# Adding a new handler to Warpnet

A handler in Warpnet is one entry in a multi-layer pipeline. Adding one means touching all of these layers in lockstep — skipping any single layer leaves the feature dead. The pipeline is fixed; treat it as a checklist, not a suggestion.

```
Vue component
   │ calls warpnetService.<verb>(...)
   ▼
frontend/src/service/service.js          ← path constant + method that calls sendToNode
   │ Wails Call({ path, body, ... })
   ▼
cmd/node/member/app.go : App.Call        ← generic dispatcher, no per-route code unless private+local-only
   │ node.SelfStream(WarpRoute(path), Message{...})
   ▼
core/handler/<feature>.go                ← StreamXxxHandler returning warpnet.WarpHandlerFunc
   │ may call streamer.GenericStream(remoteNodeId, ...)  for public peer-to-peer routes
   ▼
database/<feature>-repo.go               ← repo with Storer interface, txn-based writes
```

The handler signature is **always** `func(buf []byte, s warpnet.WarpStream) (any, error)`. The route registration is **always** in `cmd/node/member/node/member-node.go` inside `m.node.SetStreamHandlers([]warpnet.WarpStreamHandler{...})`. The path constant is **always** declared in `event/paths.go` *and* re-declared in `frontend/src/service/service.js`.

## Step 0 — Do not skip context discovery

Before writing a line of code, read these files in this order. Each is small enough to fit in a single view:

1. `cmd/node/member/app.go` — see `App.Call` and the special-case for `PRIVATE_POST_LOGIN` / `PRIVATE_POST_LOGOUT`. Everything else hits the `default:` branch and goes through `node.SelfStream`.
2. `event/paths.go` — naming convention for routes.
3. `core/handler/like.go` — the cleanest small reference. ~215 lines. Read top to bottom.
4. `core/handler/like_test.go` — stub style used everywhere; no testify.
5. `database/like-repo.go` — `Storer` interface + `NewTxn`/`defer Rollback`/`Commit` pattern.
6. `cmd/node/member/node/member-node.go` lines ~370–510 — the registration block.
7. `frontend/src/service/service.js` — top of file for path constants, `likeTweet` (~620) for the calling pattern, `sendToNode` (~880) for the wrapper.

If the feature touches a resource that already exists (replies, follows, chats…), also read the matching `core/handler/<that>.go` — you will reuse its repo or DTO.

## Step 1 — Choose the path

Routes follow `/{visibility}/{verb}/{resource}/{version}` exactly.

- **visibility** is `private` or `public`.
  - `private` = only the local UI calls this (talks to its own node only). Examples: `PRIVATE_GET_TIMELINE`, `PRIVATE_POST_TWEET`, `PRIVATE_DELETE_CHAT`, `PRIVATE_GET_STATS`.
  - `public` = nodes call this on each other over libp2p. Examples: `PUBLIC_POST_LIKE`, `PUBLIC_GET_USER`, `PUBLIC_POST_FOLLOW`.
  - If unsure: does a remote peer ever need to invoke this? Yes → `public`. No → `private`.
- **verb** is `get`, `post`, `delete`. (No `put`, no `patch` — Warpnet does not use them.)
- **resource** is short, lowercase, one word if possible. Plural for collections (`tweets`, `users`), singular for items (`tweet`, `user`).
- **version** is `0.0.0` for new routes. Bump only when you make a breaking wire change to an existing route — do not bump just because the file changed.

The same constant is declared **twice** with the same string value:
- Go: `event/paths.go`, alphabetically sorted within its visibility block.
- JS: `frontend/src/service/service.js`, top of file.

These two strings must match byte-for-byte. If they drift, the call silently goes to the `default:` of `App.Call` and fails with `not attached server node` or with handler-not-found.

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

## Step 6 — Wire the frontend

Two edits in `frontend/src/service/service.js`:

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

## Step 7 — Tests

A new handler is incomplete without a `_test.go`. Style is dictated by the existing tests in `core/handler/`:

- Plain `testing` package, no testify.
- `package handler` (white-box, not `handler_test`).
- File starts with `//nolint:all` — the existing tests do this.
- Stubs are local structs with `<method>Fn func(...) (...)` fields, falling back to a default if `nil`. See `stubLikeRepo` in `like_test.go` for the canonical shape.
- Subtests with `t.Run("invalid payload", func(t *testing.T) {...})`. Always cover at least: invalid payload, each empty-field validation, the happy path, and remote-call branches if any.
- Helper `marshal(t, ...)` is already defined in `like_test.go` and reused — if you need it and it's not in package scope, copy it.

The repo also has `core/handler/notifications_e2e_test.go` for end-to-end style; only add to that level if the feature is genuinely cross-handler.

## Step 8 — Pre-flight checklist before commit

Before `git commit`, verify each of these. Missing any one is a real bug, not a style nit.

- [ ] AGPL header present at top of every new `.go` file. Copy it verbatim from `core/handler/like.go` lines 1–26 (the 22-line block plus `// Copyright ...` and `// SPDX-License-Identifier: AGPL-3.0-or-later`).
- [ ] Path constant in `event/paths.go` **and** in `frontend/src/service/service.js`, identical string.
- [ ] DTO in `event/event.go` with the `// XxxEvent defines model for XxxEvent.` godoc.
- [ ] Repo method validates empty inputs, uses `NewTxn` + `defer Rollback` + `Commit`.
- [ ] Handler registered in `cmd/node/member/node/member-node.go`, near related routes, with `//nolint:govet`.
- [ ] Handler validates every input field before touching the repo.
- [ ] Frontend service method exists and is named, parameter list matches what components will call.
- [ ] `_test.go` covers invalid payload + each empty-field error + happy path.
- [ ] Run `go test -short -p 8 -v ./core/handler/... ./database/...` locally (this is what `make tests` does for the whole repo).
- [ ] `go vet ./...` clean. `golangci-lint run` clean (CI runs it; config in `.golangci.yaml`).
- [ ] `vendor/` not modified. If `go mod` accidentally touched it, `git checkout vendor/`.
- [ ] `version` file: **do not bump manually** — the `pre-commit` githook auto-bumps the patch on non-`main` branches and updates `snap/snapcraft.yaml`. Make sure `make setup-hooks` has been run (`git config core.hooksPath .githooks`).
- [ ] Branch name follows `IssueNumber/AmazingFeature`. Commit message imperative, present tense.
- [ ] PR description names the path you added so reviewers can grep it.

## Common mistakes specific to this codebase

- **Adding the path string only on one side.** The Go constant and the JS constant are independent strings. The compiler does not link them. A typo here = silent failure with `not attached server node` in logs.
- **Using `encoding/json` instead of the project's `json` package.** Tests will pass, runtime may pass, but you'll diverge from project-wide marshalling settings (the project uses `jsoniter` under the hood for performance and number handling).
- **Forgetting `//nolint:govet`** on the registration entry. The block uses positional struct literals which `govet` doesn't like. Match neighbours.
- **Putting `domain.ID` fields where `string` belongs or vice versa.** IDs in events are `domain.ID`. In handler logic they're often used as `string` — `domain.ID` is a typed string alias, the conversion is implicit.
- **Calling `streamer.GenericStream` for a `private` route.** Private routes never propagate. Only `public` routes call out to other nodes. If you wrote a private handler that needs to call a peer, you've split the wrong way — that should be a public route.
- **Treating `ErrNodeIsOffline` as a real error.** It isn't. The local write succeeded; the propagation failed; return the local result and move on. See `like.go` for the exact branch.
- **Bumping the `version` file by hand.** The pre-commit hook does it. Doing it manually creates a conflict with the hook.
- **Modifying `vendor/`** directly or via `go get` without intent. `vendor/` changes need to be a deliberate `make update-deps` commit, separate from feature work.
- **Adding a `put` or `patch` verb.** Not used in the project. Use `post` for create-or-update, `delete` for delete.

## When the task is moderation, admin, or pair-related

These have extra ceremony — see `core/handler/admin.go`, `core/handler/pair.go`, `core/handler/moderation.go`. They use challenge/response signing and PSK plumbing that regular handlers don't. Read those files before treating an admin-route task as "just another handler".
