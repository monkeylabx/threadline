// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "T011SwiftHarness",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "T011SwiftHarness", targets: ["T011SwiftHarness"]),
    ],
    targets: [
        .executableTarget(name: "T011SwiftHarness"),
    ]
)
