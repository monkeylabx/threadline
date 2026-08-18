import Dispatch
import Foundation
import ThreadlineIOSHost

// Standalone (non-XCTest) driver for the T010-A Swift -> Rust FFI facade.
// Mirrors the contract points the XCTest suite covers so the boundary can be
// exercised on a host that has the Swift toolchain but not full Xcode.

final class Recorder: @unchecked Sendable {
    private let lock = NSLock()
    private var checks = 0
    private var failures: [String] = []

    func check(_ condition: Bool, _ label: String) {
        lock.lock()
        checks += 1
        if !condition { failures.append(label) }
        lock.unlock()
        print(condition ? "  PASS  \(label)" : "  FAIL  \(label)")
    }

    var summary: (checks: Int, failures: [String]) {
        lock.lock()
        defer { lock.unlock() }
        return (checks, failures)
    }
}

final class Box<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var storage: T

    init(_ value: T) { storage = value }

    var value: T {
        get { lock.lock(); defer { lock.unlock() }; return storage }
        set { lock.lock(); storage = newValue; lock.unlock() }
    }

    func mutate(_ body: (inout T) -> Void) {
        lock.lock()
        body(&storage)
        lock.unlock()
    }
}

let recorder = Recorder()
let delivery = DispatchQueue(label: "threadline.harness.delivery")

func check(_ condition: Bool, _ label: String) { recorder.check(condition, label) }
func section(_ title: String) { print("\n== \(title) ==") }

func resourceCounts() -> [ThreadlineBridgeResource: UInt64] {
    var counts: [ThreadlineBridgeResource: UInt64] = [:]
    for kind in [ThreadlineBridgeResource.client, .request, .stream, .buffer] {
        counts[kind] = ThreadlineIOSHostSkeleton.resourceCount(kind)
    }
    return counts
}

func run() {
    // MARK: 1. Contract version and baseline

    section("contract version and resource baseline")
    let contractVersion = ThreadlineIOSHostSkeleton.bridgeContractVersion
    check(contractVersion == 1, "bridge contract version is 1 (got \(contractVersion))")

    let baseline = resourceCounts()
    check(baseline.values.allSatisfy { $0 == 0 }, "resource registry starts empty (\(baseline))")

    // MARK: 2. Request happy path

    section("request: call, result buffer, commit")
    do {
        let client = try ThreadlineClient()
        let done = DispatchSemaphore(value: 0)
        let outcome = Box<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)

        let request = try client.startRequest(deliveryQueue: delivery) { result in
            outcome.value = result
            done.signal()
        }
        check(done.wait(timeout: .now() + 10) == .success, "request completion delivered")

        switch outcome.value {
        case .success(let value):
            check(value.committed, "request reported committed")
            check(!value.data.isEmpty, "request returned a non-empty result buffer")
            print("        payload = \(String(data: value.data, encoding: .utf8) ?? "<non-utf8>")")
        case .failure(let status):
            check(false, "request succeeded (got failure \(status))")
        case nil:
            check(false, "request produced an outcome")
        }

        check((try? request.isCommitted()) == true, "isCommitted() reports true after commit")
        request.release()
        client.release()
    } catch {
        check(false, "request happy path threw \(error)")
    }

    // MARK: 3. Cancellation before commit

    section("request: cancel before commit")
    do {
        let client = try ThreadlineClient()
        let done = DispatchSemaphore(value: 0)
        let outcome = Box<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)

        let request = try client.startRequest(
            fault: .delayed,
            delayMilliseconds: 3_000,
            deliveryQueue: delivery
        ) { result in
            outcome.value = result
            done.signal()
        }

        let cancelStatus = request.cancel()
        check(cancelStatus == .ok, "cancel() accepted while pending (got \(cancelStatus))")
        check(done.wait(timeout: .now() + 10) == .success, "canceled request still completes")

        if case .failure(let status)? = outcome.value {
            check(status == .canceled, "cancellation maps to .canceled (got \(status))")
        } else {
            check(false, "canceled request reported a failure outcome (got \(String(describing: outcome.value)))")
        }

        request.release()
        client.release()
    } catch {
        check(false, "cancel path threw \(error)")
    }

    // MARK: 4. Cancel after commit

    section("request: cancel inside the commit window is already_committed")
    do {
        let client = try ThreadlineClient()
        let done = DispatchSemaphore(value: 0)
        // fault .none with a delay commits immediately, then holds in the
        // Committed phase for the delay before succeeding.
        let request = try client.startRequest(
            fault: .none,
            delayMilliseconds: 3_000,
            deliveryQueue: delivery
        ) { _ in done.signal() }

        var committed = false
        for _ in 0..<300 where !committed {
            committed = (try? request.isCommitted()) ?? false
            if !committed { usleep(5_000) }
        }
        check(committed, "request reached the committed phase")

        let status = request.cancel()
        check(
            status == .alreadyCommitted,
            "cancel in the commit window maps to .alreadyCommitted (got \(status))"
        )

        check(done.wait(timeout: .now() + 10) == .success, "committed request still completes")
        request.release()
        client.release()
    } catch {
        check(false, "commit-window cancel path threw \(error)")
    }

    // Cancel after the request has already reached a terminal phase.
    section("request: cancel after terminal completion")
    do {
        let client = try ThreadlineClient()
        let done = DispatchSemaphore(value: 0)
        let request = try client.startRequest(deliveryQueue: delivery) { _ in done.signal() }
        _ = done.wait(timeout: .now() + 10)

        let status = request.cancel()
        check(
            status == .ok,
            "cancel after terminal completion is an idempotent no-op returning .ok (got \(status))"
        )
        request.release()
        client.release()
    } catch {
        check(false, "terminal cancel path threw \(error)")
    }

    // MARK: 5. Panic isolation and unknown error

    section("faults: panic isolation and unknown error mapping")
    for (fault, expected) in [
        (ThreadlineBridgeFault.panic, ThreadlineBridgeStatus.panic),
        (ThreadlineBridgeFault.unknownError, ThreadlineBridgeStatus.unknown),
    ] {
        do {
            let client = try ThreadlineClient()
            let done = DispatchSemaphore(value: 0)
            let outcome = Box<Result<ThreadlineRequestResult, ThreadlineBridgeStatus>?>(nil)
            let request = try client.startRequest(fault: fault, deliveryQueue: delivery) { result in
                outcome.value = result
                done.signal()
            }
            check(
                done.wait(timeout: .now() + 10) == .success,
                "\(fault) fault completed without unwinding into Swift"
            )
            if case .failure(let status)? = outcome.value {
                check(status == expected, "\(fault) maps to \(expected) (got \(status))")
            } else {
                check(false, "\(fault) produced a failure outcome")
            }
            request.release()
            client.release()
        } catch {
            check(false, "\(fault) path threw \(error)")
        }
    }

    // MARK: 6. Bounded stream, monotonic sequences, backpressure

    section("stream: bounded, monotonic, backpressure counted")
    do {
        let client = try ThreadlineClient()
        let finished = DispatchSemaphore(value: 0)
        let sequences = Box<[UInt64]>([])
        let completion = Box<ThreadlineBridgeStatus?>(nil)

        let stream = try client.startStream(
            cursor: 0,
            eventCount: 64,
            capacity: 4,
            deliveryQueue: delivery,
            onEvent: { sequence in
                sequences.mutate { $0.append(sequence) }
                usleep(200)  // slow consumer, forces the producer to hit the bound
            },
            onCompletion: { status in
                completion.value = status
                finished.signal()
            }
        )

        check(finished.wait(timeout: .now() + 30) == .success, "stream reached completion")
        check(
            completion.value == .endOfStream,
            "stream ends with .endOfStream (got \(String(describing: completion.value)))"
        )

        let observed = sequences.value
        check(observed.count == 64, "delivered all 64 events (got \(observed.count))")
        check(observed == Array(1...64), "sequences are strictly monotonic starting at cursor+1")

        let metrics = try stream.metrics()
        check(metrics.capacity == 4, "metrics report the requested capacity")
        check(metrics.maxDepth <= 4, "queue depth never exceeded capacity (max \(metrics.maxDepth))")
        check(metrics.backpressureCount > 0, "producer recorded backpressure (\(metrics.backpressureCount))")
        print("        metrics = capacity \(metrics.capacity), maxDepth \(metrics.maxDepth), backpressure \(metrics.backpressureCount), suppressedLate \(metrics.suppressedLateEvents)")

        stream.release()
        client.release()
    } catch {
        check(false, "stream path threw \(error)")
    }

    // MARK: 7. Cursor resume

    section("stream: resume from a durable cursor")
    do {
        let client = try ThreadlineClient()
        let finished = DispatchSemaphore(value: 0)
        let sequences = Box<[UInt64]>([])

        let stream = try client.startStream(
            cursor: 41,
            eventCount: 3,
            capacity: 8,
            deliveryQueue: delivery,
            onEvent: { sequence in sequences.mutate { $0.append(sequence) } },
            onCompletion: { _ in finished.signal() }
        )

        check(finished.wait(timeout: .now() + 10) == .success, "resumed stream completed")
        check(sequences.value == [42, 43, 44], "resume continues at cursor+1 (got \(sequences.value))")

        stream.release()
        client.release()
    } catch {
        check(false, "resume path threw \(error)")
    }

    // MARK: 8. Duplicate event is a protocol violation

    section("stream: duplicate event terminates with protocol_violation")
    do {
        let client = try ThreadlineClient()
        let finished = DispatchSemaphore(value: 0)
        let completion = Box<ThreadlineBridgeStatus?>(nil)
        let sequences = Box<[UInt64]>([])

        let stream = try client.startStream(
            cursor: 0,
            eventCount: 8,
            capacity: 8,
            fault: .duplicateEvent,
            deliveryQueue: delivery,
            onEvent: { sequence in sequences.mutate { $0.append(sequence) } },
            onCompletion: { status in
                completion.value = status
                finished.signal()
            }
        )

        check(finished.wait(timeout: .now() + 10) == .success, "duplicate-fault stream completed")
        check(
            completion.value == .protocolViolation,
            "duplicate event maps to .protocolViolation (got \(String(describing: completion.value)))"
        )
        let observed = sequences.value
        check(
            observed.count == Set(observed).count,
            "no duplicate sequence was exposed to the host (got \(observed))"
        )

        stream.release()
        client.release()
    } catch {
        check(false, "duplicate-event path threw \(error)")
    }

    // MARK: 9. Callback fencing after close

    section("callback fencing: no delivery after close() returns")
    do {
        let client = try ThreadlineClient()
        let state = Box<(closed: Bool, deliveriesAfterClose: Int)>((false, 0))

        let request = try client.startRequest(
            fault: .delayed,
            delayMilliseconds: 400,
            deliveryQueue: delivery
        ) { _ in
            state.mutate { if $0.closed { $0.deliveriesAfterClose += 1 } }
        }

        state.mutate { $0.closed = true }
        request.close()

        Thread.sleep(forTimeInterval: 1.5)  // let any in-flight delivery land

        check(
            state.value.deliveriesAfterClose == 0,
            "no completion delivered after close() returned (got \(state.value.deliveriesAfterClose))"
        )

        request.release()
        client.release()
    } catch {
        check(false, "callback fencing path threw \(error)")
    }

    // MARK: 10. Memory ownership: 1000 lifecycle loops

    section("memory ownership: 1000 create/start/close/release cycles")
    do {
        for _ in 0..<1_000 {
            let client = try ThreadlineClient()
            let request = try client.startRequest(
                fault: .delayed,
                delayMilliseconds: 5_000,
                deliveryQueue: delivery
            ) { _ in }
            let stream = try client.startStream(
                cursor: 0,
                eventCount: 4,
                capacity: 2,
                delayMilliseconds: 5_000,
                deliveryQueue: delivery,
                onEvent: { _ in },
                onCompletion: { _ in }
            )
            request.close()
            stream.close()
            request.release()
            stream.release()
            client.release()
        }

        let after = resourceCounts()
        check(
            after == baseline,
            "resource counts returned to baseline after 1000 cycles (baseline \(baseline), after \(after))"
        )
    } catch {
        check(false, "lifecycle loop threw \(error)")
    }

    // MARK: 11. Stale handle use

    section("stale handle: use after release is rejected, not a crash")
    do {
        let client = try ThreadlineClient()
        let done = DispatchSemaphore(value: 0)
        let request = try client.startRequest(deliveryQueue: delivery) { _ in done.signal() }
        _ = done.wait(timeout: .now() + 10)
        request.release()

        do {
            _ = try request.isCommitted()
            check(false, "released request rejects further calls")
        } catch let status as ThreadlineBridgeStatus {
            check(
                status == .closed || status == .invalidHandle,
                "released request returns a stable status (got \(status))"
            )
        }

        client.release()
        do {
            _ = try client.startRequest(deliveryQueue: delivery) { _ in }
            check(false, "released client rejects new requests")
        } catch let status as ThreadlineBridgeStatus {
            check(
                status == .closed || status == .invalidHandle,
                "released client returns a stable status (got \(status))"
            )
        }
    } catch {
        check(false, "stale handle path threw \(error)")
    }
}

run()

let summary = recorder.summary
print("\n==================================================")
print("checks: \(summary.checks)   failures: \(summary.failures.count)")
if summary.failures.isEmpty {
    print("RESULT: PASS")
    exit(0)
} else {
    print("RESULT: FAIL")
    for failure in summary.failures { print("  - \(failure)") }
    exit(1)
}
