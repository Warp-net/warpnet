# warpfone

The iOS client of Warpnet. It starts from Ice Cubes 1.10.31, the last Apache-2.0
release of the SwiftUI Mastodon client by Thomas Ricouard (see [NOTICE](NOTICE)).

Status: upstream snapshot. The app still talks to Mastodon over HTTP. The Warpnet
transport, QR pairing and the Warpnet data layer are not wired in yet.

Building needs macOS with Xcode 15 or later; the deployment target is iOS 17.
The app links the Warpnet node as `Warpnet.xcframework`, which is not committed:
run `scripts/build-xcframework.sh` first (it needs Go, see the root `go.mod`).
The framework is built from `node/`, warpfone's own copy of the gomobile node.
Debug builds read `IceCubesApp.xcconfig`, which is not committed: copy
`IceCubesApp.xcconfig.template` and set your team and bundle id prefix.
Release builds read `IceCubesApp-release.xcconfig`, where `DEVELOPMENT_TEAM`
stays empty until the project has an Apple developer team.

The version follows the repository's `version` file. The pre-commit hook writes
it into `Version.xcconfig`, which both configurations include, so the app and
its extensions always carry the same version.
