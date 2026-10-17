import Foundation
import ThreadlineProto
import XCTest

final class LocalAgentTests: XCTestCase {
    func testContractWireFixtureRoundtrips() throws {
        // test/fixtures/proto/agentd-local/wire.json: SubmitRunInputRequest.
        let wire = Data([0x0a, 0x05] + Array("run-a".utf8)
            + [0x12, 0x07] + Array("input-a".utf8)
            + [0x1a, 0x06] + Array("héllo".utf8))
        let request = try SubmitRunInputRequest(serializedBytes: wire)
        XCTAssertEqual(request.runID, "run-a")
        XCTAssertEqual(request.inputID, "input-a")
        XCTAssertEqual(request.text, "héllo")
        XCTAssertEqual(try request.serializedData(), wire)
    }
}
