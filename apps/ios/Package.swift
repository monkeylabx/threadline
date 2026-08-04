// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "ThreadlineIOSHost",
    platforms: [
        .iOS(.v17),
        .macOS(.v14),
    ],
    products: [
        .library(name: "ThreadlineIOSHost", targets: ["ThreadlineIOSHost"]),
    ],
    targets: [
        .target(name: "ThreadlineIOSHost"),
        .testTarget(
            name: "ThreadlineIOSHostTests",
            dependencies: ["ThreadlineIOSHost"]
        ),
    ]
)
