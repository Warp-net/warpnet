// swift-tools-version: 5.9
// Modified by Warpnet. See NOTICE.
// The swift-tools-version declares the minimum version of Swift required to build this package.

import PackageDescription

let package = Package(
  name: "Network",
  defaultLocalization: "en",
  platforms: [
    .iOS(.v17),
    .visionOS(.v1),
  ],
  products: [
    .library(
      name: "Network",
      targets: ["Network"]
    ),
  ],
  dependencies: [
    .package(name: "Models", path: "../Models"),
    .package(name: "WarpnetTransport", path: "../WarpnetTransport"),
  ],
  targets: [
    .target(
      name: "Network",
      dependencies: [
        .product(name: "Models", package: "Models"),
        .product(name: "WarpnetTransport", package: "WarpnetTransport"),
      ],
      swiftSettings: [
        .enableExperimentalFeature("StrictConcurrency"),
      ]
    ),
    .testTarget(
      name: "NetworkTests",
      dependencies: ["Network"]
    ),
  ]
)
