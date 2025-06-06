---
search:
  exclude: true
---

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.24.0](https://github.com/uber-go/fx/compare/v1.23.0...v1.24.0) - 2025-05-13

### Added
- A new event `fxevent.BeforeRun` is now emitted before Fx runs a constructor, 
  decorator, or supply/replace stub.

### Changed
- Clearer error messages are now used when annotation building fails.

## [1.23.0](https://github.com/uber-go/fx/compare/v1.22.2...v1.22.3) - 2024-10-11

### Added
- Added `Runtime` to `fxevent.Run` event, which stores the runtime of
  a constructor or a decorator that's run, including functions created
  by `fx.Supply` and `fx.Replace`.

### Changed
- Overhauled the documentation website. (https://uber-go.github.io/fx/)

## [1.22.2](https://github.com/uber-go/fx/compare/v1.22.1...v1.22.2) - 2024-08-07

### Fixed
- A deadlock with the relayer in signal receivers.

### Changed
- Upgrade Dig dependency to v1.18.0

## [1.22.1](https://github.com/uber-go/fx/compare/v1.22.0...v1.22.1) - 2024-06-25

### Fixed
- Fx apps will only listen to signals when `.Run()`, `.Wait()`, or `.Done()`
  are called, fixing a regression introduced in v1.19.0.

## [1.22.0](https://github.com/uber-go/fx/compare/v1.21.1...v1.22.0) - 2024-05-30

### Added
- Add `fx.Self` which can be passed to the `fx.As` annotation to signify
  that a type should be provided as itself.
- Add `fxtest.EnforceTimeout` that can be passed to `fxtest.NewLifecycle`
  to force `Start` and `Stop` to return context errors when hook context expires.

### Changed
- `fx.Private` can now be used with `fx.Supply`.

### Fixed
- Fx apps will no longer listen to OS signals when they are stopped,
  solving blocking issues in programs that depended on OS signals
  after an Fx app stops.

## [1.21.1](https://github.com/uber-go/fx/compare/v1.21.0...v1.21.1) - 2024-04-24

### Changed
- Register Fx provides (e.g. fx.Lifecycle, fx.Shutdowner, fx.DotGraph) before
  user provides, to increase likelihood of successful custom logger creation.

## [1.21.0](https://github.com/uber-go/fx/compare/v1.20.1...v1.21.0) - 2024-03-13

### Added
- fxtest: Add WithTestLogger option that uses a `testing.TB` as the
  Fx event logger.
- An fxevent logger that can log events using a slog logger has been added.

### Changed
- Upgrade Dig dependency to v1.17.1

## [1.20.1](https://github.com/uber-go/fx/compare/v1.20.0...v1.20.1) - 2023-10-17

### Added
- Provided, Decorated, Supplied, and Replaced events now include a trace
  of module locations through which the option was given to the App.
- wasi support.

## [1.20.0](https://github.com/uber-go/fx/compare/v1.19.3...v1.20.0) - 2023-06-12

### Added
- A new event `fxevent.Run` is now emitted when Fx runs a constructor, decorator,
  or supply/replace stub.

### Changed
- `fx.Populate` now works with `fx.Annotate`.
- Upgrade Dig dependency to v1.17.0.

## [1.19.3](https://github.com/uber-go/fx/compare/v1.19.2...v1.19.3) - 2023-04-17

### Changed
- Fixed several typos in docs.
- WASM build support.
- Annotating In and Out structs with From/As annotations generated invalid results.
  The annotation check now blocks this.
- `Shutdown`: Support calling from `Invoke`.

### Deprecated
- Deprecate `ShutdownTimeout` option.

### Fixed
- Respect Shutdowner ExitCode from calling `Run`.

## [1.19.2](https://github.com/uber-go/fx/compare/v1.19.1...v1.19.2) - 2023-02-21

### Changed
- Update Dig dependency to v1.16.1.

## [1.19.1](https://github.com/uber-go/fx/compare/v1.19.0...v1.19.1) - 2023-01-10

### Changed
- Calling `fx.Stop()` after the `App` has already stopped no longer errors out.

### Fixed
- Addressed a regression in 1.19.0 release which caused apps to ignore OS signals
  after running for startTimeout duration.

## [1.19.0](https://github.com/uber-go/fx/compare/v1.18.2...v1.19.0) - 2023-01-03

### Added
- `fx.RecoverFromPanics` Option which allows Fx to recover from user-provided constructors
  and invoked functions.
- `fx.Private` that allows the constructor to limit the scope of its outputs to the wrapping
  `fx.Module`.
- `ExitCode` ShutdownOption which allows setting custom exit code at the end of app
  lifecycle.
- `Wait` which returns a channel that can be used for waiting on application shutdown.
- fxevent/ZapLogger now exposes `UseLogLevel` and `UseErrorLevel` methods to set
  the level of the Zap logs produced by it.
- Add lifecycle hook-convertible methods: `StartHook`, `StopHook`, `StartStopHook`
  that can be used with more function signatures.

### Changed
- `fx.WithLogger` can now be passed at `fx.Module` level, setting custom logger at
  `Module` scope instead of the whole `App`.

### Fixed
- `fx.OnStart` and `fx.OnStop` Annotations now work with annotated types that was
  provided by the annotated constructor.
- fxevent/ZapLogger: Errors from `fx.Supply` are now logged at `Error` level, not
  `Info`.
- A race condition in lifecycle Start/Stop methods.
- Typos in docs.

## [1.18.2](https://github.com/uber-go/fx/compare/v1.18.1...v1.18.2) - 2022-09-28

### Added
- Clarify ordering of `Invoke`s in `Module`s.

### Fixed
- Fix `Decorate` not being applied to transitive dependencies at root `App` level.

## [1.18.1](https://github.com/uber-go/fx/compare/v1.18.0...v1.18.1) - 2022-08-08

### Fixed
- Fix a nil panic when `nil` is passed to `OnStart` and `OnStop` lifecycle methods.

## [1.18.0](https://github.com/uber-go/fx/compare/v1.17.1...v1.18.0) - 2022-08-05

### Added
- Soft value groups that lets you specify value groups as best-effort dependencies.
- `fx.OnStart` and `fx.OnStop` annotations which lets you annotate dependencies to provide
  OnStart and OnStop lifecycle hooks.
- A new `fxevent.Replaced` event written to `fxevent.Logger` following an `fx.Replace`.

### Fixed
- Upgrade Dig dependency to v1.14.1 to address a couple of issues with decorations. Refer to
  Dig v1.14.1 release notes for more details.
- `fx.WithLogger` no longer ignores decorations and replacements of types that
  it depends on.
- Don't run lifecycle hooks if the context for them has already expired.
- `App.Start` and `App.Stop` no longer deadlock if the OnStart/OnStop hook
  exits the current goroutine.
- `fxevent.ConsoleLogger` no longer emits an extraneous argument for the
  Supplied event.

### Deprecated
- `fx.Extract` in favor of `fx.Populate`.

## [1.17.1](https://github.com/uber-go/fx/compare/v1.17.0...v1.17.1) - 2022-03-23

### Added
- Logging for provide/invoke/decorate now includes the associated `fx.Module` name.

## [1.17.0](https://github.com/uber-go/fx/compare/v1.16.0...v1.17.0) - 2022-02-28

### Added
- Add `fx.Module` which scopes any modifications made to the dependency graph.
- Add `fx.Decorate` and `fx.Replace` that lets you modify a dependency graph with decorators.
- Add `fxevent.Decorated` event which gets emitted upon a dependency getting decorated.

### Changed
- `fx.Annotate`: Validate that `fx.In` or `fx.Out` structs are not passed to it.
- `fx.Annotate`: Upon failure to Provide, the error contains the actual location
  of the provided constructor.

## [1.16.0](https://github.com/uber-go/fx/compare/v1.15.0...v1.16.0) - 2021-12-02

### Added
- Add the ability to provide a function as multiple interfaces at once using `fx.As`.

### Changed
- `fx.Annotate`: support variadic functions, and feeding value groups into them.

### Fixed
- Fix an issue where OnStop hooks weren't getting called on SIGINT on Windows.
- Fix a data race between app.Done() and shutdown.

## [1.15.0](https://github.com/uber-go/fx/compare/v1.14.2...v1.15.0) - 2021-11-08

### Added
- Add `fx.Annotate` to allow users to provide parameter and result tags easily without
  having to create `fx.In` or `fx.Out` structs.
- Add `fx.As` that allows users to annotate a constructor to provide its result type(s) as
  interface(s) that they implement instead of the types themselves.

### Fixed
- Fix `fxevent.Stopped` not being logged when `App.Stop` is called.
- Fix `fxevent.Started` or `fxevent.Stopped` not being logged when start or
  stop times out.

## [1.14.2](https://github.com/uber-go/fx/compare/v1.14.1...v1.14.2) - 2021-08-16

### Changed
- For `fxevent` console implementation: no longer log non-error case for `fxevent.Invoke`
  event, while for zap implementation, start logging `fx.Invoking` case without stack.

## [1.14.1](https://github.com/uber-go/fx/compare/v1.14.0...v1.14.1) - 2021-08-16

### Changed
- `fxevent.Invoked` was being logged at `Error` level even upon successful `Invoke`.
  This was changed to log at `Info` level when `Invoke` succeeded.

## [1.14.0](https://github.com/uber-go/fx/compare/v1.13.1...v1.14.0) - 2021-08-12

### Added
- Introduce the new `fx.WithLogger` option. Provide a constructor for
  `fxevent.Logger` objects with it to customize how Fx logs events.
- Add new `fxevent` package that exposes events from Fx in a structured way.
  Use this to write custom logger implementations for use with the
  `fx.WithLogger` option.
- Expose and log additional information when lifecycle hooks time out.

### Changed
- Fx now emits structured JSON logs by default. These may be parsed and
  processed by log ingestion systems.
- `fxtest.Lifecycle` now logs to the provided `testing.TB` instead of stderr.
- `fx.In` and `fx.Out` are now type aliases instead of structs.

## [1.13.1](https://github.com/uber-go/fx/compare/v1.13.0...v1.13.1) - 2020-08-19

### Fixed
- Fix minimum version constraint for dig. `fx.ValidateGraph` requires at least
  dig 1.10.

## [1.13.0](https://github.com/uber-go/fx/compare/v1.12.0...v1.13.0) - 2020-06-16

### Added
- Added `fx.ValidateGraph` which allows graph cycle validation and dependency correctness
  without running anything. This is useful if `fx.Invoke` has side effects, does I/O, etc.

## [1.12.0](https://github.com/uber-go/fx/compare/v1.11.0...v1.12.0) - 2020-04-09

### Added
- Added `fx.Supply` to provide externally created values to Fx containers
  without building anonymous constructors.

### Changed
- Drop library dependency on development tools.

## [1.11.0](https://github.com/uber-go/fx/compare/v1.10.0...v1.11.0) - 2020-04-01

### Added
- Value groups can use the `flatten` option to indicate values in a slice should
  be provided individually rather than providing the slice itself. See package
  documentation for details.

## [1.10.0](https://github.com/uber-go/fx/compare/v1.9.0...v1.10.0) - 2019-11-20

### Added
- All `fx.Option`s now include readable string representations.
- Report stack traces when `fx.Provide` and `fx.Invoke` calls fail. This
  should make these errors more debuggable.

### Changed
- Migrated to Go modules.

## [1.9.0](https://github.com/uber-go/fx/compare/v1.8.0...v1.9.0) - 2019-01-22

### Added
- Add the ability to shutdown Fx applications from inside the container. See
  the Shutdowner documentation for details.
- Add `fx.Annotated` to allow users to provide named values without creating a
  new constructor.

## [1.8.0](https://github.com/uber-go/fx/compare/v1.7.1...v1.8.0) - 2018-11-06

### Added
- Provide DOT graph of dependencies in the container.

## [1.7.1](https://github.com/uber-go/fx/compare/v1.7.0...v1.7.1) - 2018-09-26

### Fixed
- Make `fxtest.New` ensure that the app was created successfully. Previously,
  it would return the app (similar to `fx.New`, which expects the user to verify
  the error).
- Update dig container to defer acyclic validation until after Invoke. Application
  startup time should improve proportional to the size of the dependency graph.
- Fix a goroutine leak in `fxtest.Lifecycle`.

## [1.7.0](https://github.com/uber-go/fx/compare/v1.6.0...v1.7.0) - 2018-08-16

### Added
- Add `fx.ErrorHook` option to allow users to provide `ErrorHandler`s on invoke
  failures.
- `VisualizeError` returns the visualization wrapped in the error if available.

## [1.6.0](https://github.com/uber-go/fx/compare/v1.5.0...v1.6.0) - 2018-06-12

### Added
- Add `fx.Error` option to short-circuit application startup.

## [1.5.0](https://github.com/uber-go/fx/compare/v1.4.0...v1.5.0) - 2018-04-11

### Added
- Add `fx.StartTimeout` and `fx.StopTimeout` to make configuring application
  start and stop timeouts easier.
- Export the default start and stop timeout as `fx.DefaultTimeout`.

### Fixed
- Make `fxtest` respect the application's start and stop timeouts.

## [1.4.0](https://github.com/uber-go/fx/compare/v1.3.0...v1.4.0) - 2017-12-07

### Added
- Add `fx.Populate` to populate variables with values from the dependency
  injection container without requiring intermediate structs.

## [1.3.0](https://github.com/uber-go/fx/compare/v1.2.0...v1.3.0) - 2017-11-28

### Changed
- Improve readability of hook logging in addition to provide and invoke.

### Fixed
- Fix bug which caused the OnStop for a lifecycle hook to be called even if it
  failed to start.

## [1.2.0](https://github.com/uber-go/fx/compare/v1.1.0...v1.2.0) - 2017-09-06

### Added
- Add `fx.NopLogger` which disables the Fx application's log output.

## [1.1.0](https://github.com/uber-go/fx/compare/v1.0.0...v1.1.0) - 2017-08-22

### Changed
- Improve readability of start up logging.

## [1.0.0](https://github.com/uber-go/fx/compare/v1.0.0-rc2...v1.0.0) - 2017-07-31

First stable release: no breaking changes will be made in the 1.x series.

### Added
- `fx.Extract` now supports `fx.In` tags on target structs.

### Changed
- **[Breaking]** Rename `fx.Inject` to `fx.Extract`.
- **[Breaking]** Rename `fxtest.Must*` to `fxtest.Require*`.

### Removed
- **[Breaking]** Remove `fx.Timeout` and `fx.DefaultTimeout`.

## [1.0.0-rc2](https://github.com/uber-go/fx/compare/v1.0.0-rc1...v1.0.0-rc2) - 2017-07-21

- **[Breaking]** Lifecycle hooks now take a context.
- Add `fx.In` and `fx.Out` which exposes optional and named types.
  Modules should embed these types instead of relying on `dig.In` and `dig.Out`.
- Add an `Err` method to retrieve the underlying errors during the dependency
  graph construction. The same error is also returned from `Start`.
- Graph resolution now happens as part of `fx.New`, rather than at the beginning
  of `app.Start`. This allows inspection of the graph errors through `app.Err()`
  before the decision to start the app.
- Add a `Logger` option, which allows users to send Fx's logs to different
  sink.
- Add `fxtest.App`, which redirects log output to the user's `testing.TB` and
  provides some lifecycle helpers.

## [1.0.0-rc1](https://github.com/uber-go/fx/compare/v1.0.0-beta4...v1.0.0-rc1) - 2017-06-20

- **[Breaking]** Providing types into `fx.App` and invoking functions are now
  options passed during application construction. This makes users'
  interactions with modules and collections of modules identical.
- **[Breaking]** `TestLifecycle` is now in a separate `fxtest` subpackage.
- Add `fx.Inject()` to pull values from the container into a struct.

## [1.0.0-beta4](https://github.com/uber-go/fx/compare/v1.0.0-beta3...v1.0.0-beta4) - 2017-06-12

- **[Breaking]** Monolithic framework, as released in initial betas, has been
  broken into smaller pieces as a result of recent advances in `dig` library.
  This is a radical departure from the previous direction, but it needed to
  be done for the long-term good of the project.
- **[Breaking]** `Module interface` has been scoped all the way down to being
  *a single dig constructor*. This allows for very sophisticated module
  compositions. See `go.uber.org/dig` for more information on the constructors.
- **[Breaking]** `package config` has been moved to its own repository.
  see `go.uber.org/config` for more information.
- `fx.Lifecycle` has been added for modules to hook into the framework
  lifecycle events.
- `service.Host` interface which composed a number of primitives together
  (configuration, metrics, tracing) has been deprecated in favor of
  `fx.App`.

## [1.0.0-beta3](https://github.com/uber-go/fx/compare/v1.0.0-beta2...v1.0.0-beta3) - 2017-03-28

- **[Breaking]** Environment config provider was removed. If you were using
  environment variables to override YAML values, see config documentation for
  more information.
- **[Breaking]** Simplify Provider interface: remove `Scope` method from the
  `config.Provider` interface, one can use either ScopedProvider and Value.Get()
  to access sub fields.
- Add `task.MustRegister` convenience function which fails fast by panicking
  Note that this should only be used during app initialization, and is provided
  to avoid repetetive error checking for services which register many tasks.
- Expose options on task module to disable execution. This will allow users to
  enqueue and consume tasks on different clusters.
- **[Breaking]** Rename Backend interface `Publish` to `Enqueue`. Created a new
  `ExecuteAsync` method that will kick off workers to consume tasks and this is
  subsumed by module Start.
- **[Breaking]** Rename package `uhttp/client` to `uhttp/uhttpclient` for clarity.
- **[Breaking]** Rename `PopulateStruct` method in value to `Populate`.
  The method can now populate not only structs, but anything: slices,
  maps, builtin types and maps.
- **[Breaking]** `package dig` has moved from `go.uber.org/fx/dig` to a new home
  at `go.uber.org/dig`.
- **[Breaking]** Pass a tracer the `uhttp/uhttpclient` constructor explicitly, instead
  of using a global tracer. This will allow to use http client in parallel tests.

## [1.0.0-beta2](https://github.com/uber-go/fx/compare/v1.0.0-beta1...v1.0.0-beta2) - 2017-03-09

- **[Breaking]** Remove `ulog.Logger` interface and expose `*zap.Logger` directly.
- **[Breaking]** Rename config and module from `modules.rpc` to `modules.yarpc`
- **[Breaking]** Rename config key from `modules.http` to `modules.uhttp` to match
  the module name
- **[Breaking]** Upgrade `zap` to `v1.0.0-rc.3` (now go.uber.org/zap, was
  github.com/uber-go/zap)
- Remove now-unused `config.IsDevelopmentEnv()` helper to encourage better
  testing practices. Not a breaking change as nobody is using this func
  themselves according to our code search tool.
- Log `traceID` and `spanID` in hex format to match Jaeger UI. Upgrade Jaeger to
  min version 2.1.0
  and use jaeger's adapters for jaeger and tally initialization.
- Tally now supports reporting histogram samples for a bucket. Upgrade Tally to 2.1.0
- **[Breaking]** Make new module naming consistent `yarpc.ThriftModule` to
  `yarpc.New`, `task.NewModule`
  to `task.New`
- **[Breaking]** Rename `yarpc.CreateThriftServiceFunc` to `yarpc.ServiceCreateFunc`
  as it is not thrift-specific.
- Report version metrics for company-wide version usage information.
- Allow configurable service name and module name via service options.
- DIG constructors now support returning a tuple with the second argument being
  an error.

## 1.0.0-beta1 - 2017-02-20

This is the first beta release of the framework, where we invite users to start
building services on it and provide us feedback. **Warning** we are not
promising API compatibility between beta releases and the final 1.0.0 release.
In fact, we expect our beta user feedback to require some changes to the way
things work. Once we reach 1.0, we will provider proper version compatibility.
