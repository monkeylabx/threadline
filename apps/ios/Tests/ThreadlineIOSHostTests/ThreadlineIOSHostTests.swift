import XCTest
@testable import ThreadlineIOSHost

final class ThreadlineIOSHostTests: XCTestCase {
    func testBridgeContractVersionComesFromRustFacade() {
        XCTAssertEqual(ThreadlineIOSHostSkeleton.bridgeContractVersion, 1)
    }
}
