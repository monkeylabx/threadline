import Foundation
import XCTest
@testable import ThreadlineIOSHost

private final class LockedBox<Value>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: Value

    init(_ value: Value) {
        self.value = value
    }

    func read() -> Value {
        lock.lock()
        defer { lock.unlock() }
        return value
    }

    func update(_ body: (inout Value) -> Void) {
        lock.lock()
        body(&value)
        lock.unlock()
    }
}

private final class CallbackLifetimeToken: @unchecked Sendable {}

final class ThreadlineIOSHostTests: XCTestCase {
    func testBridgeContractVersionComesFromRustFacade() {
        XCTAssertEqual(ThreadlineIOSHostSkeleton.bridgeContractVersion, 1)
    }

    func testAsyncRequestReturnsCopiedDataOnMainQueueWithoutBlockingCaller() throws {
        let client = try ThreadlineClient()
        let completed = expectation(description: "request completed")
        let result = LockedBox<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)

        let startedAt = ContinuousClock.now
        let request = try client.startRequest(
            fault: .delayed,
            delayMilliseconds: 200
        ) { outcome in
            XCTAssertTrue(Thread.isMainThread)
            result.update { $0 = outcome }
            completed.fulfill()
        }
        XCTAssertLessThan(startedAt.duration(to: .now), .milliseconds(100))

        wait(for: [completed], timeout: 3)
        let completedResult = try result.read()!.get()
        XCTAssertEqual(
            completedResult,
            ThreadlineRequestResult(data: Data("threadline-ok".utf8), committed: true)
        )
        XCTAssertEqual(ThreadlineIOSHostSkeleton.resourceCount(.buffer), 0)

        request.release()
        request.release()
        client.release()
        client.release()
    }

    func testCancelBeforeCommitIsDeterministicAndIdempotent() throws {
        let client = try ThreadlineClient()
        let completed = expectation(description: "request canceled")
        let result = LockedBox<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)
        let request = try client.startRequest(
            fault: .delayed,
            delayMilliseconds: 50
        ) { outcome in
            result.update { $0 = outcome }
            completed.fulfill()
        }

        XCTAssertEqual(request.cancel(), .ok)
        XCTAssertEqual(request.cancel(), .ok)
        wait(for: [completed], timeout: 2)
        XCTAssertEqual(result.read(), .failure(.canceled))

        request.release()
        client.release()
    }

    func testCancelAfterCommitReturnsAlreadyCommitted() throws {
        let client = try ThreadlineClient()
        let completed = expectation(description: "committed request completed")
        let result = LockedBox<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)
        let request = try client.startRequest(delayMilliseconds: 200) { outcome in
            result.update { $0 = outcome }
            completed.fulfill()
        }

        let deadline = ContinuousClock.now.advanced(by: .seconds(2))
        while ContinuousClock.now < deadline, !(try request.isCommitted()) {
            Thread.sleep(forTimeInterval: 0.001)
        }
        let committed = try request.isCommitted()
        XCTAssertTrue(committed)
        XCTAssertEqual(request.cancel(), ThreadlineBridgeStatus.alreadyCommitted)
        wait(for: [completed], timeout: 3)
        _ = try result.read()!.get()

        request.release()
        client.release()
    }

    func testPanicAndUnknownFaultsHaveStableErrors() throws {
        for (fault, expected) in [
            (ThreadlineBridgeFault.panic, ThreadlineBridgeStatus.panic),
            (.unknownError, .unknown),
        ] {
            let client = try ThreadlineClient()
            let completed = expectation(description: "fault completed")
            let result = LockedBox<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)
            let request = try client.startRequest(fault: fault) { outcome in
                result.update { $0 = outcome }
                completed.fulfill()
            }
            wait(for: [completed], timeout: 2)
            XCTAssertEqual(result.read(), .failure(expected))
            request.release()
            client.release()
        }
    }

    func testRequestAndClientCloseSuppressLateCallbacks() throws {
        let requestCallback = expectation(description: "closed request callback")
        requestCallback.isInverted = true
        let requestClient = try ThreadlineClient()
        let request = try requestClient.startRequest(
            fault: .delayed,
            delayMilliseconds: 40
        ) { _ in
            requestCallback.fulfill()
        }
        request.close()
        request.close()
        wait(for: [requestCallback], timeout: 0.15)
        request.release()
        requestClient.release()

        let clientCallback = expectation(description: "closed client callback")
        clientCallback.isInverted = true
        let client = try ThreadlineClient()
        let child = try client.startRequest(
            fault: .delayed,
            delayMilliseconds: 40
        ) { _ in
            clientCallback.fulfill()
        }
        client.close()
        wait(for: [clientCallback], timeout: 0.15)
        child.release()
        client.release()
    }

    func testBoundedStreamIsMonotonicAndResumesFromCursor() throws {
        let client = try ThreadlineClient()
        let firstDone = expectation(description: "first stream completed")
        let firstEvents = LockedBox<[UInt64]>([])
        let firstStatus = LockedBox<ThreadlineBridgeStatus?>(nil)
        let first = try client.startStream(
            cursor: 40,
            eventCount: 8,
            capacity: 2,
            onEvent: { sequence in
                XCTAssertTrue(Thread.isMainThread)
                Thread.sleep(forTimeInterval: 0.002)
                firstEvents.update { $0.append(sequence) }
            },
            onCompletion: { status in
                firstStatus.update { $0 = status }
                firstDone.fulfill()
            }
        )

        wait(for: [firstDone], timeout: 3)
        XCTAssertEqual(firstStatus.read(), .endOfStream)
        XCTAssertEqual(firstEvents.read(), Array(41 ... 48))
        let metrics = try first.metrics()
        XCTAssertLessThanOrEqual(metrics.maxDepth, metrics.capacity)
        XCTAssertGreaterThan(metrics.backpressureCount, 0)
        first.release()

        let resumedDone = expectation(description: "resumed stream completed")
        let resumedEvents = LockedBox<[UInt64]>([])
        let resumed = try client.startStream(
            cursor: 48,
            eventCount: 2,
            capacity: 1,
            onEvent: { sequence in resumedEvents.update { $0.append(sequence) } },
            onCompletion: { status in
                XCTAssertEqual(status, .endOfStream)
                resumedDone.fulfill()
            }
        )
        wait(for: [resumedDone], timeout: 2)
        XCTAssertEqual(resumedEvents.read(), [49, 50])
        resumed.release()
        client.release()
    }

    func testSlowStreamConsumersDoNotStarveIndependentStreamCompletion() throws {
        let client = try ThreadlineClient()
        let blockedTarget = DispatchQueue(label: "threadline.tests.stream.blocked-target")
        blockedTarget.suspend()
        var blockedStreams: [ThreadlineStream] = []
        var probeStream: ThreadlineStream?

        defer {
            blockedTarget.resume()
            probeStream?.release()
            for stream in blockedStreams {
                stream.release()
            }
            client.release()
        }

        for index in 0 ..< 96 {
            let deliveryQueue = DispatchQueue(
                label: "threadline.tests.stream.blocked-\(index)",
                qos: .userInitiated,
                target: blockedTarget
            )
            let stream = try client.startStream(
                eventCount: 1,
                capacity: 1,
                deliveryQueue: deliveryQueue,
                onEvent: { _ in },
                onCompletion: { _ in }
            )
            blockedStreams.append(stream)
        }

        let blockersReadyDeadline = ContinuousClock.now.advanced(by: .seconds(5))
        while ContinuousClock.now < blockersReadyDeadline,
            blockedStreams.contains(where: { !$0.hasPendingDeliveryForDiagnostics })
        {
            Thread.sleep(forTimeInterval: 0.001)
        }
        XCTAssertTrue(
            blockedStreams.allSatisfy(\.hasPendingDeliveryForDiagnostics),
            "all blocked consumers must have an event queued before probing"
        )

        let probeQueue = DispatchQueue(
            label: "threadline.tests.stream.probe",
            qos: .userInitiated
        )
        for iteration in 0 ..< 5 {
            let probeCompleted = expectation(
                description: "independent stream completed, iteration \(iteration)"
            )
            let probeEvents = LockedBox<[UInt64]>([])
            let probeStatus = LockedBox<ThreadlineBridgeStatus?>(nil)
            probeStream = try client.startStream(
                cursor: 40,
                eventCount: 8,
                capacity: 2,
                deliveryQueue: probeQueue,
                onEvent: { sequence in
                    probeEvents.update { $0.append(sequence) }
                },
                onCompletion: { status in
                    probeStatus.update { $0 = status }
                    probeCompleted.fulfill()
                }
            )

            wait(for: [probeCompleted], timeout: 2)
            XCTAssertEqual(probeEvents.read(), Array(41 ... 48))
            XCTAssertEqual(probeStatus.read(), .endOfStream)
            probeStream?.release()
            probeStream = nil
        }
    }

    func testReleaseDropsCallbacksQueuedOnAnUndrainedDeliveryQueue() throws {
        let client = try ThreadlineClient()
        let blockedTarget = DispatchQueue(label: "threadline.tests.stream.release-target")
        blockedTarget.suspend()
        defer {
            blockedTarget.resume()
            client.release()
        }

        var token: CallbackLifetimeToken? = CallbackLifetimeToken()
        weak let weakToken = token
        let deliveryQueue = DispatchQueue(
            label: "threadline.tests.stream.release-blocked",
            target: blockedTarget
        )
        let stream = try client.startStream(
            eventCount: 1,
            capacity: 1,
            deliveryQueue: deliveryQueue,
            onEvent: { [token] _ in _ = token },
            onCompletion: { _ in }
        )

        let pendingDeadline = ContinuousClock.now.advanced(by: .seconds(5))
        while ContinuousClock.now < pendingDeadline,
            !stream.hasPendingDeliveryForDiagnostics
        {
            Thread.sleep(forTimeInterval: 0.001)
        }
        XCTAssertTrue(stream.hasPendingDeliveryForDiagnostics)

        stream.release()
        token = nil
        XCTAssertNil(weakToken)
    }

    func testDuplicateStreamEventFailsBeforeDuplicateDelivery() throws {
        let client = try ThreadlineClient()
        let completed = expectation(description: "protocol violation")
        let events = LockedBox<[UInt64]>([])
        let status = LockedBox<ThreadlineBridgeStatus?>(nil)
        let stream = try client.startStream(
            eventCount: 5,
            capacity: 5,
            fault: .duplicateEvent,
            onEvent: { sequence in events.update { $0.append(sequence) } },
            onCompletion: { terminal in
                status.update { $0 = terminal }
                completed.fulfill()
            }
        )
        wait(for: [completed], timeout: 2)
        XCTAssertEqual(events.read(), [1, 2])
        XCTAssertEqual(status.read(), .protocolViolation)
        stream.release()
        client.release()
    }

    func testLateStreamEventAfterCloseIsSuppressed() throws {
        let client = try ThreadlineClient()
        let firstEvent = expectation(description: "first event")
        let completion = expectation(description: "completion after close")
        completion.isInverted = true
        let stream = try client.startStream(
            eventCount: 1,
            capacity: 1,
            delayMilliseconds: 20,
            fault: .lateEvent,
            onEvent: { _ in firstEvent.fulfill() },
            onCompletion: { _ in completion.fulfill() }
        )

        wait(for: [firstEvent], timeout: 2)
        stream.close()
        wait(for: [completion], timeout: 0.15)
        let deadline = ContinuousClock.now.advanced(by: .seconds(2))
        while ContinuousClock.now < deadline,
            try stream.metrics().suppressedLateEvents == 0
        {
            Thread.sleep(forTimeInterval: 0.001)
        }
        let finalMetrics = try stream.metrics()
        XCTAssertEqual(finalMetrics.suppressedLateEvents, 1)

        stream.release()
        client.release()
    }

    func testOneThousandHostLifecycleLoopsLeaveNoNativeResources() throws {
        let baselineClients = ThreadlineIOSHostSkeleton.resourceCount(.client)
        let baselineRequests = ThreadlineIOSHostSkeleton.resourceCount(.request)

        for _ in 0 ..< 1_000 {
            try autoreleasepool {
                let client = try ThreadlineClient()
                let completed = DispatchSemaphore(value: 0)
                let request = try client.startRequest(
                    deliveryQueue: .global(qos: .userInitiated)
                ) { _ in
                    completed.signal()
                }
                XCTAssertEqual(completed.wait(timeout: .now() + 2), .success)
                request.close()
                request.release()
                client.close()
                client.release()
            }
        }

        XCTAssertEqual(ThreadlineIOSHostSkeleton.resourceCount(.client), baselineClients)
        XCTAssertEqual(ThreadlineIOSHostSkeleton.resourceCount(.request), baselineRequests)
        XCTAssertEqual(ThreadlineIOSHostSkeleton.resourceCount(.stream), 0)
        XCTAssertEqual(ThreadlineIOSHostSkeleton.resourceCount(.buffer), 0)
    }
}
