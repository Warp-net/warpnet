# warpfone

The iOS client of Warpnet. It starts from Ice Cubes 1.10.31, the last Apache-2.0
release of the SwiftUI Mastodon client by Thomas Ricouard (see [NOTICE](NOTICE)).

Status: the app pairs with a Warpnet node by QR code and reads timelines,
threads, profiles and notifications from it. Screens for features the node does
not have are still there and fail; removing them is the next step.

Building needs macOS with Xcode 15 or later; the deployment target is iOS 17.
The app links the Warpnet node as `Warpnet.xcframework`, which is not committed:
run `scripts/build-xcframework.sh` first (it needs Go, see the root `go.mod`).
The framework is built from `node/`, warpfone's own copy of the gomobile node.
`Packages/WarpnetTransport` is the wire layer on top of it. Its `ProtocolIds`
are generated from the node's `event/paths.go` by `scripts/generate-protocol-ids.sh`,
and its tests run with `swift test --package-path Packages/WarpnetTransport`.
`Packages/Network` keeps the Ice Cubes `Client` API, and the router in
`Sources/Network/Warpnet` answers its Mastodon endpoints from the node; its
tests run with `swift test --package-path Packages/Network`.
Debug builds read `IceCubesApp.xcconfig`, which is not committed: copy
`IceCubesApp.xcconfig.template` and set your team and bundle id prefix.
Release builds read `IceCubesApp-release.xcconfig`, where `DEVELOPMENT_TEAM`
stays empty until the project has an Apple developer team.

Releases to TestFlight go through the `Release / Warpfone` workflow; see
[RELEASE.md](RELEASE.md) for its secrets, export compliance and App Review.

The version follows the repository's `version` file. The pre-commit hook writes
it into `Version.xcconfig`, which both configurations include, so the app and
its extensions always carry the same version.
