// swift-tools-version: 6.3

import PackageDescription

let package = Package(
    name: "ThreadlineProto",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [.library(name: "ThreadlineProto", targets: ["ThreadlineProto"])],
    dependencies: [
        .package(url: "https://github.com/apple/swift-protobuf.git", exact: "1.38.1"),
        .package(url: "https://github.com/connectrpc/connect-swift.git", exact: "1.2.3"),
    ],
    targets: [
        .target(name: "ThreadlineProto", dependencies: [
            .product(name: "SwiftProtobuf", package: "swift-protobuf"),
            .product(name: "Connect", package: "connect-swift"),
        ]),
        .testTarget(name: "ThreadlineProtoTests", dependencies: ["ThreadlineProto"]),
    ]
)
