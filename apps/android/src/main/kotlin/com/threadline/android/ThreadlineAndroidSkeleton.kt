package com.threadline.android

import java.util.IdentityHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.atomic.AtomicLong

enum class ThreadlineBridgeStatus(val code: Int) {
    OK(0),
    INVALID_ARGUMENT(1),
    INVALID_HANDLE(2),
    CLOSED(3),
    CANCELED(4),
    PANIC(5),
    ALREADY_COMMITTED(6),
    TIMED_OUT(7),
    BACKPRESSURE(8),
    PROTOCOL_VIOLATION(9),
    END_OF_STREAM(10),
    UNKNOWN(255);

    companion object {
        fun decode(code: Int): ThreadlineBridgeStatus =
            entries.firstOrNull { it.code == code } ?: UNKNOWN
    }
}

enum class ThreadlineBridgeFault(val code: Int) {
    NONE(0),
    DELAYED(1),
    PANIC(2),
    UNKNOWN_ERROR(3),
    DUPLICATE_EVENT(4),
    LATE_EVENT(5),
}

enum class ThreadlineBridgeResource(val code: Int) {
    CLIENT(1),
    REQUEST(2),
    STREAM(3),
    BUFFER(4),
}

class ThreadlineBridgeException(
    val status: ThreadlineBridgeStatus,
) : RuntimeException("native bridge status: $status")

data class ThreadlineRequestResult(
    val data: ByteArray,
    val committed: Boolean,
)

data class ThreadlineStreamMetrics(
    val capacity: Int,
    val maxDepth: Int,
    val backpressureCount: Long,
    val suppressedLateEvents: Long,
)

private fun requireNativeHandle(value: Long): Long {
    if (value > 0) return value
    val status = if (value < 0) {
        ThreadlineBridgeStatus.decode((-value).toInt())
    } else {
        ThreadlineBridgeStatus.UNKNOWN
    }
    throw ThreadlineBridgeException(status)
}

private fun requireNativeMetric(value: Long): Long {
    if (value >= 0) return value
    throw ThreadlineBridgeException(ThreadlineBridgeStatus.decode((-value).toInt()))
}

internal object ThreadlineNative {
    init {
        System.loadLibrary("threadline_client_ffi")
    }

    external fun nativeBridgeContractVersion(): Int
    external fun nativeClientCreate(): Long
    external fun nativeClientClose(handle: Long): Int
    external fun nativeClientRelease(handle: Long): Int
    external fun nativeRequestStart(clientHandle: Long, fault: Int, delayMilliseconds: Long): Long
    external fun nativeRequestCancel(handle: Long): Int
    external fun nativeRequestState(handle: Long): Int
    external fun nativeRequestWait(handle: Long, timeoutMilliseconds: Long): Int
    external fun nativeRequestCommitted(handle: Long): Boolean
    external fun nativeRequestResultLength(handle: Long): Int
    external fun nativeRequestResultByte(handle: Long, index: Int): Int
    external fun nativeRequestClose(handle: Long): Int
    external fun nativeRequestRelease(handle: Long): Int
    external fun nativeStreamStart(
        clientHandle: Long,
        cursor: Long,
        eventCount: Int,
        capacity: Int,
        delayMilliseconds: Long,
        fault: Int,
    ): Long
    external fun nativeStreamNext(handle: Long, timeoutMilliseconds: Long): Long
    external fun nativeStreamCapacity(handle: Long): Int
    external fun nativeStreamMaxDepth(handle: Long): Int
    external fun nativeStreamBackpressureCount(handle: Long): Long
    external fun nativeStreamSuppressedLateEvents(handle: Long): Long
    external fun nativeStreamCancel(handle: Long): Int
    external fun nativeStreamClose(handle: Long): Int
    external fun nativeStreamRelease(handle: Long): Int
    external fun nativeDebugResourceCount(kind: Int): Long
}

object ThreadlineAndroidSkeleton {
    val bridgeContractVersion: UInt
        get() = ThreadlineNative.nativeBridgeContractVersion().toUInt()

    fun resourceCount(kind: ThreadlineBridgeResource): Long =
        ThreadlineNative.nativeDebugResourceCount(kind.code)
}

internal class CallbackGate {
    private val monitor = Object()
    private var closed = false
    private var activeCallbacks = 0
    private val activeThreads = IdentityHashMap<Thread, Int>()

    fun perform(block: () -> Unit) {
        val thread = Thread.currentThread()
        synchronized(monitor) {
            if (closed) return
            activeCallbacks += 1
            activeThreads[thread] = (activeThreads[thread] ?: 0) + 1
        }

        try {
            block()
        } finally {
            synchronized(monitor) {
                activeCallbacks -= 1
                val count = activeThreads[thread] ?: 1
                if (count > 1) {
                    activeThreads[thread] = count - 1
                } else {
                    activeThreads.remove(thread)
                }
                monitor.notifyAll()
            }
        }
    }

    fun close() {
        val thread = Thread.currentThread()
        synchronized(monitor) {
            closed = true
            val callbacksOnThisThread = activeThreads[thread] ?: 0
            while (activeCallbacks > callbacksOnThisThread) {
                monitor.wait()
            }
        }
    }
}

class ThreadlineClient : AutoCloseable {
    private val callbackGate = CallbackGate()
    private val nativeHandle = AtomicLong(requireNativeHandle(ThreadlineNative.nativeClientCreate()))

    fun startRequest(
        fault: ThreadlineBridgeFault = ThreadlineBridgeFault.NONE,
        delayMilliseconds: Long = 0,
        deliveryExecutor: Executor,
        completion: (Result<ThreadlineRequestResult>) -> Unit,
    ): ThreadlineRequest {
        val clientHandle = openHandle()
        val requestHandle = requireNativeHandle(
            ThreadlineNative.nativeRequestStart(
                clientHandle,
                fault.code,
                delayMilliseconds,
            ),
        )
        return ThreadlineRequest(
            requestHandle,
            callbackGate,
            deliveryExecutor,
            completion,
        )
    }

    fun startStream(
        cursor: Long = 0,
        eventCount: Int,
        capacity: Int,
        delayMilliseconds: Long = 0,
        fault: ThreadlineBridgeFault = ThreadlineBridgeFault.NONE,
        deliveryExecutor: Executor,
        onEvent: (Long) -> Unit,
        onCompletion: (ThreadlineBridgeStatus) -> Unit,
    ): ThreadlineStream {
        val clientHandle = openHandle()
        val streamHandle = requireNativeHandle(
            ThreadlineNative.nativeStreamStart(
                clientHandle,
                cursor,
                eventCount,
                capacity,
                delayMilliseconds,
                fault.code,
            ),
        )
        return ThreadlineStream(
            streamHandle,
            callbackGate,
            deliveryExecutor,
            onEvent,
            onCompletion,
        )
    }

    override fun close() {
        callbackGate.close()
        val handle = nativeHandle.get()
        if (handle != 0L) {
            ThreadlineNative.nativeClientClose(handle)
        }
    }

    fun release() {
        callbackGate.close()
        val handle = nativeHandle.getAndSet(0)
        if (handle != 0L) {
            ThreadlineNative.nativeClientClose(handle)
            ThreadlineNative.nativeClientRelease(handle)
        }
    }

    private fun openHandle(): Long {
        val handle = nativeHandle.get()
        if (handle == 0L) {
            throw ThreadlineBridgeException(ThreadlineBridgeStatus.CLOSED)
        }
        return handle
    }
}

class ThreadlineRequest internal constructor(
    handle: Long,
    clientGate: CallbackGate,
    deliveryExecutor: Executor,
    completion: (Result<ThreadlineRequestResult>) -> Unit,
) : AutoCloseable {
    private val callbackGate = CallbackGate()
    private val nativeHandle = AtomicLong(handle)

    init {
        val requestGate = callbackGate
        Thread({
            val status = ThreadlineBridgeStatus.decode(
                ThreadlineNative.nativeRequestWait(handle, 30_000),
            )
            val result = if (status == ThreadlineBridgeStatus.OK) {
                readResult(handle)
            } else {
                Result.failure(ThreadlineBridgeException(status))
            }
            deliveryExecutor.execute {
                clientGate.perform {
                    requestGate.perform {
                        completion(result)
                    }
                }
            }
        }, "threadline-request-$handle").apply {
            isDaemon = true
            start()
        }
    }

    fun cancel(): ThreadlineBridgeStatus {
        val handle = nativeHandle.get()
        if (handle == 0L) return ThreadlineBridgeStatus.CLOSED
        return ThreadlineBridgeStatus.decode(ThreadlineNative.nativeRequestCancel(handle))
    }

    override fun close() {
        callbackGate.close()
        val handle = nativeHandle.get()
        if (handle != 0L) {
            ThreadlineNative.nativeRequestClose(handle)
        }
    }

    fun release() {
        callbackGate.close()
        val handle = nativeHandle.getAndSet(0)
        if (handle != 0L) {
            ThreadlineNative.nativeRequestClose(handle)
            ThreadlineNative.nativeRequestRelease(handle)
        }
    }

    private fun readResult(handle: Long): Result<ThreadlineRequestResult> {
        val length = ThreadlineNative.nativeRequestResultLength(handle)
        if (length < 0) {
            return Result.failure(
                ThreadlineBridgeException(ThreadlineBridgeStatus.decode(-length)),
            )
        }
        val data = ByteArray(length)
        for (index in 0 until length) {
            val value = ThreadlineNative.nativeRequestResultByte(handle, index)
            if (value < 0) {
                return Result.failure(
                    ThreadlineBridgeException(ThreadlineBridgeStatus.decode(-value)),
                )
            }
            data[index] = value.toByte()
        }
        return Result.success(
            ThreadlineRequestResult(
                data = data,
                committed = ThreadlineNative.nativeRequestCommitted(handle),
            ),
        )
    }
}

class ThreadlineStream internal constructor(
    handle: Long,
    clientGate: CallbackGate,
    deliveryExecutor: Executor,
    onEvent: (Long) -> Unit,
    onCompletion: (ThreadlineBridgeStatus) -> Unit,
) : AutoCloseable {
    private val callbackGate = CallbackGate()
    private val nativeHandle = AtomicLong(handle)

    init {
        val streamGate = callbackGate
        Thread({
            while (true) {
                val value = ThreadlineNative.nativeStreamNext(handle, 30_000)
                if (value > 0) {
                    val delivered = CountDownLatch(1)
                    deliveryExecutor.execute {
                        try {
                            clientGate.perform {
                                streamGate.perform {
                                    onEvent(value)
                                }
                            }
                        } finally {
                            delivered.countDown()
                        }
                    }
                    delivered.await()
                    continue
                }

                val status = if (value == 0L) {
                    ThreadlineBridgeStatus.END_OF_STREAM
                } else {
                    ThreadlineBridgeStatus.decode((-value).toInt())
                }
                deliveryExecutor.execute {
                    clientGate.perform {
                        streamGate.perform {
                            onCompletion(status)
                        }
                    }
                }
                return@Thread
            }
        }, "threadline-stream-$handle").apply {
            isDaemon = true
            start()
        }
    }

    fun metrics(): ThreadlineStreamMetrics {
        val handle = nativeHandle.get()
        if (handle == 0L) {
            throw ThreadlineBridgeException(ThreadlineBridgeStatus.CLOSED)
        }
        val capacity = requireNativeMetric(
            ThreadlineNative.nativeStreamCapacity(handle).toLong(),
        ).toInt()
        val maxDepth = requireNativeMetric(
            ThreadlineNative.nativeStreamMaxDepth(handle).toLong(),
        ).toInt()
        val backpressureCount = requireNativeMetric(
            ThreadlineNative.nativeStreamBackpressureCount(handle),
        )
        val suppressedLateEvents = requireNativeMetric(
            ThreadlineNative.nativeStreamSuppressedLateEvents(handle),
        )
        return ThreadlineStreamMetrics(
            capacity = capacity,
            maxDepth = maxDepth,
            backpressureCount = backpressureCount,
            suppressedLateEvents = suppressedLateEvents,
        )
    }

    fun cancel(): ThreadlineBridgeStatus {
        val handle = nativeHandle.get()
        if (handle == 0L) return ThreadlineBridgeStatus.CLOSED
        return ThreadlineBridgeStatus.decode(ThreadlineNative.nativeStreamCancel(handle))
    }

    override fun close() {
        callbackGate.close()
        val handle = nativeHandle.get()
        if (handle != 0L) {
            ThreadlineNative.nativeStreamClose(handle)
        }
    }

    fun release() {
        callbackGate.close()
        val handle = nativeHandle.getAndSet(0)
        if (handle != 0L) {
            ThreadlineNative.nativeStreamClose(handle)
            ThreadlineNative.nativeStreamRelease(handle)
        }
    }
}
