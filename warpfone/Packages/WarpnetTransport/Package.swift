// swift-tools-version: 5.9

import PackageDescription

let package = Package(
  name: "WarpnetTransport",
  platforms: [
    .iOS(.v17),
    .macOS(.v14),
  ],
  products: [
    .library(
      name: "WarpnetTransport",
      targets: ["WarpnetTransport"]
    ),
  ],
  targets: [
    .target(
      name: "WarpnetTransport",
      swiftSettings: [
        .enableExperimentalFeature("StrictConcurrency"),
      ]
    ),
    .testTarget(
      name: "WarpnetTransportTests",
      dependencies: ["WarpnetTransport"]
    ),
  ]
)
