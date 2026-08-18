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
        // Runs the same contract points as ThreadlineIOSHostTests without
        // XCTest, so the Swift -> Rust boundary can be exercised on a machine
        // that has the Swift toolchain but not a full Xcode install.
        // Deliberately NOT exposed as a product: an extra product changes the
        // schemes xcodebuild derives for this package, which breaks
        // `xcodebuild test -scheme ThreadlineIOSHost` in the Simulator job.
        // `swift run ThreadlineFFIHarness` works without a product entry.
        .executableTarget(
            name: "ThreadlineFFIHarness",
            dependencies: ["ThreadlineIOSHost"]
        ),
    ]
)
