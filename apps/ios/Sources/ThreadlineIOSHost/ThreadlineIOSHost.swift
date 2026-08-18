import Dispatch
import Foundation

@_silgen_name("threadline_client_ffi_contract_version")
private func nativeContractVersion() -> UInt32

@_silgen_name("threadline_client_create")
private func nativeClientCreate(_ output: UnsafeMutablePointer<UInt64>?) -> Int32

@_silgen_name("threadline_client_close")
private func nativeClientClose(_ handle: UInt64) -> Int32

@_silgen_name("threadline_client_release")
private func nativeClientRelease(_ handle: UInt64) -> Int32

@_silgen_name("threadline_request_start")
private func nativeRequestStart(
    _ clientHandle: UInt64,
    _ fault: Int32,
    _ delayMilliseconds: UInt64,
    _ output: UnsafeMutablePointer<UInt64>?
) -> Int32

@_silgen_name("threadline_request_cancel")
private func nativeRequestCancel(_ handle: UInt64) -> Int32

@_silgen_name("threadline_request_is_committed")
private func nativeRequestIsCommitted(
    _ handle: UInt64,
    _ committed: UnsafeMutablePointer<UInt8>?
) -> Int32

@_silgen_name("threadline_request_wait")
private func nativeRequestWait(
    _ handle: UInt64,
    _ timeoutMilliseconds: UInt64,
    _ committed: UnsafeMutablePointer<UInt8>?
) -> Int32

private struct NativeBuffer {
    var data: UnsafePointer<UInt8>?
    var count: Int
    var handle: UInt64

    init() {
        data = nil
        count = 0
        handle = 0
    }
}

@_silgen_name("threadline_request_result")
private func nativeRequestResult(
    _ handle: UInt64,
    _ output: UnsafeMutablePointer<NativeBuffer>?
) -> Int32

@_silgen_name("threadline_request_close")
private func nativeRequestClose(_ handle: UInt64) -> Int32

@_silgen_name("threadline_request_release")
private func nativeRequestRelease(_ handle: UInt64) -> Int32

@_silgen_name("threadline_buffer_release")
private func nativeBufferRelease(_ handle: UInt64) -> Int32

@_silgen_name("threadline_stream_start")
private func nativeStreamStart(
    _ clientHandle: UInt64,
    _ cursor: UInt64,
    _ eventCount: UInt32,
    _ capacity: UInt32,
    _ delayMilliseconds: UInt64,
    _ fault: Int32,
    _ output: UnsafeMutablePointer<UInt64>?
) -> Int32

@_silgen_name("threadline_stream_next")
private func nativeStreamNext(
    _ handle: UInt64,
    _ timeoutMilliseconds: UInt64,
    _ sequence: UnsafeMutablePointer<UInt64>?
) -> Int32

private struct NativeStreamMetrics {
    var capacity: UInt32 = 0
    var maxDepth: UInt32 = 0
    var backpressureCount: UInt64 = 0
    var suppressedLateEvents: UInt64 = 0
}

@_silgen_name("threadline_stream_get_metrics")
private func nativeStreamMetrics(
    _ handle: UInt64,
    _ output: UnsafeMutablePointer<NativeStreamMetrics>?
) -> Int32

@_silgen_name("threadline_stream_cancel")
private func nativeStreamCancel(_ handle: UInt64) -> Int32

@_silgen_name("threadline_stream_close")
private func nativeStreamClose(_ handle: UInt64) -> Int32

@_silgen_name("threadline_stream_release")
private func nativeStreamRelease(_ handle: UInt64) -> Int32

@_silgen_name("threadline_debug_resource_count")
private func nativeDebugResourceCount(_ kind: Int32) -> UInt64

public enum ThreadlineBridgeStatus: Int32, Error, Sendable {
    case ok = 0
    case invalidArgument = 1
    case invalidHandle = 2
    case closed = 3
    case canceled = 4
    case panic = 5
    case alreadyCommitted = 6
    case timedOut = 7
    case backpressure = 8
    case protocolViolation = 9
    case endOfStream = 10
    case unknown = 255

    fileprivate static func decode(_ code: Int32) -> Self {
        Self(rawValue: code) ?? .unknown
    }
}

public enum ThreadlineBridgeFault: Int32, Sendable {
    case none = 0
    case delayed = 1
    case panic = 2
    case unknownError = 3
    case duplicateEvent = 4
    case lateEvent = 5
}

public enum ThreadlineBridgeResource: Int32, Sendable {
    case client = 1
    case request = 2
    case stream = 3
    case buffer = 4
}

public struct ThreadlineRequestResult: Sendable, Equatable {
    public let data: Data
    public let committed: Bool
}

public struct ThreadlineStreamMetrics: Sendable, Equatable {
    public let capacity: UInt32
    public let maxDepth: UInt32
    public let backpressureCount: UInt64
    public let suppressedLateEvents: UInt64
}

public enum ThreadlineIOSHostSkeleton {
    public static var bridgeContractVersion: UInt32 {
        nativeContractVersion()
    }

    public static func resourceCount(_ kind: ThreadlineBridgeResource) -> UInt64 {
        nativeDebugResourceCount(kind.rawValue)
    }
}

private final class CallbackGate: @unchecked Sendable {
    private let condition = NSCondition()
    private var closed = false
    private var activeCallbacks = 0
    private var activeThreads: [ObjectIdentifier: Int] = [:]

    func perform(_ body: () -> Void) {
        let thread = ObjectIdentifier(Thread.current)
        condition.lock()
        guard !closed else {
            condition.unlock()
            return
        }
        activeCallbacks += 1
        activeThreads[thread, default: 0] += 1
        condition.unlock()

        body()

        condition.lock()
        activeCallbacks -= 1
        if let count = activeThreads[thread], count > 1 {
            activeThreads[thread] = count - 1
        } else {
            activeThreads.removeValue(forKey: thread)
        }
        condition.broadcast()
        condition.unlock()
    }

    func close() {
        let thread = ObjectIdentifier(Thread.current)
        condition.lock()
        closed = true
        let callbacksOnThisThread = activeThreads[thread] ?? 0
        while activeCallbacks > callbacksOnThisThread {
            condition.wait()
        }
        condition.unlock()
    }
}

public final class ThreadlineClient: @unchecked Sendable {
    private let stateLock = NSLock()
    private let callbackGate = CallbackGate()
    private var nativeHandle: UInt64

    public init() throws {
        var handle: UInt64 = 0
        let status = ThreadlineBridgeStatus.decode(nativeClientCreate(&handle))
        guard status == .ok, handle != 0 else {
            throw status == .ok ? ThreadlineBridgeStatus.unknown : status
        }
        nativeHandle = handle
    }

    deinit {
        release()
    }

    public func startRequest(
        fault: ThreadlineBridgeFault = .none,
        delayMilliseconds: UInt64 = 0,
        deliveryQueue: DispatchQueue = .main,
        completion: @escaping @Sendable (Result<ThreadlineRequestResult, ThreadlineBridgeStatus>) -> Void
    ) throws -> ThreadlineRequest {
        let clientHandle = try openHandle()
        var requestHandle: UInt64 = 0
        let status = ThreadlineBridgeStatus.decode(
            nativeRequestStart(clientHandle, fault.rawValue, delayMilliseconds, &requestHandle)
        )
        guard status == .ok, requestHandle != 0 else {
            throw status == .ok ? ThreadlineBridgeStatus.unknown : status
        }
        return ThreadlineRequest(
            nativeHandle: requestHandle,
            clientGate: callbackGate,
            deliveryQueue: deliveryQueue,
            completion: completion
        )
    }

    public func startStream(
        cursor: UInt64 = 0,
        eventCount: UInt32,
        capacity: UInt32,
        delayMilliseconds: UInt64 = 0,
        fault: ThreadlineBridgeFault = .none,
        deliveryQueue: DispatchQueue = .main,
        onEvent: @escaping @Sendable (UInt64) -> Void,
        onCompletion: @escaping @Sendable (ThreadlineBridgeStatus) -> Void
    ) throws -> ThreadlineStream {
        let clientHandle = try openHandle()
        var streamHandle: UInt64 = 0
        let status = ThreadlineBridgeStatus.decode(
            nativeStreamStart(
                clientHandle,
                cursor,
                eventCount,
                capacity,
                delayMilliseconds,
                fault.rawValue,
                &streamHandle
            )
        )
        guard status == .ok, streamHandle != 0 else {
            throw status == .ok ? ThreadlineBridgeStatus.unknown : status
        }
        return ThreadlineStream(
            nativeHandle: streamHandle,
            clientGate: callbackGate,
            deliveryQueue: deliveryQueue,
            onEvent: onEvent,
            onCompletion: onCompletion
        )
    }

    public func close() {
        callbackGate.close()
        let handle = currentHandle()
        if handle != 0 {
            _ = nativeClientClose(handle)
        }
    }

    public func release() {
        callbackGate.close()
        stateLock.lock()
        let handle = nativeHandle
        nativeHandle = 0
        stateLock.unlock()
        if handle != 0 {
            _ = nativeClientClose(handle)
            _ = nativeClientRelease(handle)
        }
    }

    private func currentHandle() -> UInt64 {
        stateLock.lock()
        defer { stateLock.unlock() }
        return nativeHandle
    }

    private func openHandle() throws -> UInt64 {
        let handle = currentHandle()
        guard handle != 0 else {
            throw ThreadlineBridgeStatus.closed
        }
        return handle
    }
}

public final class ThreadlineRequest: @unchecked Sendable {
    private let stateLock = NSLock()
    private let callbackGate = CallbackGate()
    private var nativeHandle: UInt64

    fileprivate init(
        nativeHandle: UInt64,
        clientGate: CallbackGate,
        deliveryQueue: DispatchQueue,
        completion: @escaping @Sendable (Result<ThreadlineRequestResult, ThreadlineBridgeStatus>) -> Void
    ) {
        self.nativeHandle = nativeHandle
        let requestGate = callbackGate
        DispatchQueue.global(qos: .userInitiated).async {
            var committed: UInt8 = 0
            let status = ThreadlineBridgeStatus.decode(
                nativeRequestWait(nativeHandle, 30_000, &committed)
            )
            let result: Result<ThreadlineRequestResult, ThreadlineBridgeStatus>
            if status == .ok {
                var buffer = NativeBuffer()
                let bufferStatus = ThreadlineBridgeStatus.decode(
                    nativeRequestResult(nativeHandle, &buffer)
                )
                if bufferStatus == .ok, buffer.handle != 0 {
                    defer { _ = nativeBufferRelease(buffer.handle) }
                    if buffer.count == 0 {
                        result = .success(
                            ThreadlineRequestResult(data: Data(), committed: committed != 0)
                        )
                    } else if let data = buffer.data {
                        result = .success(
                            ThreadlineRequestResult(
                                data: Data(bytes: data, count: buffer.count),
                                committed: committed != 0
                            )
                        )
                    } else {
                        result = .failure(.protocolViolation)
                    }
                } else {
                    result = .failure(bufferStatus == .ok ? .unknown : bufferStatus)
                }
            } else {
                result = .failure(status)
            }

            deliveryQueue.async {
                clientGate.perform {
                    requestGate.perform {
                        completion(result)
                    }
                }
            }
        }
    }

    deinit {
        release()
    }

    @discardableResult
    public func cancel() -> ThreadlineBridgeStatus {
        let handle = currentHandle()
        guard handle != 0 else { return .closed }
        return .decode(nativeRequestCancel(handle))
    }

    public func isCommitted() throws -> Bool {
        let handle = currentHandle()
        guard handle != 0 else { throw ThreadlineBridgeStatus.closed }
        var committed: UInt8 = 0
        let status = ThreadlineBridgeStatus.decode(
            nativeRequestIsCommitted(handle, &committed)
        )
        guard status == .ok else { throw status }
        return committed != 0
    }

    public func close() {
        callbackGate.close()
        let handle = currentHandle()
        if handle != 0 {
            _ = nativeRequestClose(handle)
        }
    }

    public func release() {
        callbackGate.close()
        stateLock.lock()
        let handle = nativeHandle
        nativeHandle = 0
        stateLock.unlock()
        if handle != 0 {
            _ = nativeRequestClose(handle)
            _ = nativeRequestRelease(handle)
        }
    }

    private func currentHandle() -> UInt64 {
        stateLock.lock()
        defer { stateLock.unlock() }
        return nativeHandle
    }
}

public final class ThreadlineStream: @unchecked Sendable {
    private let stateLock = NSLock()
    private let callbackGate = CallbackGate()
    private var nativeHandle: UInt64

    fileprivate init(
        nativeHandle: UInt64,
        clientGate: CallbackGate,
        deliveryQueue: DispatchQueue,
        onEvent: @escaping @Sendable (UInt64) -> Void,
        onCompletion: @escaping @Sendable (ThreadlineBridgeStatus) -> Void
    ) {
        self.nativeHandle = nativeHandle
        let streamGate = callbackGate
        DispatchQueue.global(qos: .userInitiated).async {
            while true {
                var sequence: UInt64 = 0
                let status = ThreadlineBridgeStatus.decode(
                    nativeStreamNext(nativeHandle, 30_000, &sequence)
                )
                if status == .ok {
                    let eventSequence = sequence
                    let delivered = DispatchSemaphore(value: 0)
                    deliveryQueue.async {
                        clientGate.perform {
                            streamGate.perform {
                                onEvent(eventSequence)
                            }
                        }
                        delivered.signal()
                    }
                    delivered.wait()
                    continue
                }

                deliveryQueue.async {
                    clientGate.perform {
                        streamGate.perform {
                            onCompletion(status)
                        }
                    }
                }
                return
            }
        }
    }

    deinit {
        release()
    }

    public func metrics() throws -> ThreadlineStreamMetrics {
        let handle = currentHandle()
        guard handle != 0 else { throw ThreadlineBridgeStatus.closed }
        var metrics = NativeStreamMetrics()
        let status = ThreadlineBridgeStatus.decode(nativeStreamMetrics(handle, &metrics))
        guard status == .ok else { throw status }
        return ThreadlineStreamMetrics(
            capacity: metrics.capacity,
            maxDepth: metrics.maxDepth,
            backpressureCount: metrics.backpressureCount,
            suppressedLateEvents: metrics.suppressedLateEvents
        )
    }

    @discardableResult
    public func cancel() -> ThreadlineBridgeStatus {
        let handle = currentHandle()
        guard handle != 0 else { return .closed }
        return .decode(nativeStreamCancel(handle))
    }

    public func close() {
        callbackGate.close()
        let handle = currentHandle()
        if handle != 0 {
            _ = nativeStreamClose(handle)
        }
    }

    public func release() {
        callbackGate.close()
        stateLock.lock()
        let handle = nativeHandle
        nativeHandle = 0
        stateLock.unlock()
        if handle != 0 {
            _ = nativeStreamClose(handle)
            _ = nativeStreamRelease(handle)
        }
    }

    private func currentHandle() -> UInt64 {
        stateLock.lock()
        defer { stateLock.unlock() }
        return nativeHandle
    }
}
