import XCTest
@testable import ThreadlineIOSHost

final class ThreadlineIOSHostTests: XCTestCase {
    func testBridgeContractVersionIsStable() {
        XCTAssertEqual(ThreadlineIOSHostSkeleton.bridgeContractVersion, 1)
    }
}
