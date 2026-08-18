// swift-tools-version: 6.0

import Foundation
import PackageDescription

let ffiLibraryDirectory =
    ProcessInfo.processInfo.environment["THREADLINE_FFI_LIBRARY_DIR"]
    ?? "../../../target/debug"

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
        .target(
            name: "ThreadlineIOSHost",
            linkerSettings: [
                .unsafeFlags([
                    "-L", ffiLibraryDirectory,
                    "-lthreadline_client_ffi",
                ]),
            ]
        ),
        .testTarget(
            name: "ThreadlineIOSHostTests",
            dependencies: ["ThreadlineIOSHost"]
        ),
    ]
)
