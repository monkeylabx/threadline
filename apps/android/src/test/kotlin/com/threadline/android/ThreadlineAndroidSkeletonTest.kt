package com.threadline.android

import java.util.Collections
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ThreadlineAndroidSkeletonTest {
    private val directExecutor = Executor { command -> command.run() }

    @Test
    fun bridgeContractVersionComesFromRustFacade() {
        assertEquals(1u, ThreadlineAndroidSkeleton.bridgeContractVersion)
    }

    @Test
    fun asyncRequestReturnsCopiedBytesWithoutBlockingCaller() {
        val client = ThreadlineClient()
        val completed = CountDownLatch(1)
        val outcome = AtomicReference<Result<ThreadlineRequestResult>>()
        val started = System.nanoTime()
        val request = client.startRequest(
            fault = ThreadlineBridgeFault.DELAYED,
            delayMilliseconds = 200,
            deliveryExecutor = directExecutor,
        ) {
            outcome.set(it)
            completed.countDown()
        }
        val elapsedMilliseconds = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started)
        assertTrue("start must not block the host thread", elapsedMilliseconds < 100)

        assertTrue(completed.await(3, TimeUnit.SECONDS))
        val result = outcome.get().getOrThrow()
        assertArrayEquals("threadline-ok".encodeToByteArray(), result.data)
        assertTrue(result.committed)

        request.release()
        request.release()
        client.release()
        client.release()
    }

    @Test
    fun cancellationCommitPointAndStableFaultsAreDeterministic() {
        val client = ThreadlineClient()

        val canceled = CountDownLatch(1)
        val canceledOutcome = AtomicReference<Result<ThreadlineRequestResult>>()
        val before = client.startRequest(
            fault = ThreadlineBridgeFault.DELAYED,
            delayMilliseconds = 40,
            deliveryExecutor = directExecutor,
        ) {
            canceledOutcome.set(it)
            canceled.countDown()
        }
        assertEquals(ThreadlineBridgeStatus.OK, before.cancel())
        assertEquals(ThreadlineBridgeStatus.OK, before.cancel())
        assertTrue(canceled.await(2, TimeUnit.SECONDS))
        assertEquals(
            ThreadlineBridgeStatus.CANCELED,
            (canceledOutcome.get().exceptionOrNull() as ThreadlineBridgeException).status,
        )
        before.release()

        val nativeClient = ThreadlineNative.nativeClientCreate()
        val after = ThreadlineNative.nativeRequestStart(
            nativeClient,
            ThreadlineBridgeFault.NONE.code,
            40,
        )
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(2)
        while (ThreadlineNative.nativeRequestState(after) != 12 && System.nanoTime() < deadline) {
            Thread.yield()
        }
        assertEquals(12, ThreadlineNative.nativeRequestState(after))
        assertEquals(
            ThreadlineBridgeStatus.ALREADY_COMMITTED.code,
            ThreadlineNative.nativeRequestCancel(after),
        )
        assertEquals(
            ThreadlineBridgeStatus.OK.code,
            ThreadlineNative.nativeRequestWait(after, 2_000),
        )
        ThreadlineNative.nativeRequestRelease(after)
        ThreadlineNative.nativeClientRelease(nativeClient)

        for ((fault, expected) in listOf(
            ThreadlineBridgeFault.PANIC to ThreadlineBridgeStatus.PANIC,
            ThreadlineBridgeFault.UNKNOWN_ERROR to ThreadlineBridgeStatus.UNKNOWN,
        )) {
            val completed = CountDownLatch(1)
            val outcome = AtomicReference<Result<ThreadlineRequestResult>>()
            val request = client.startRequest(
                fault = fault,
                deliveryExecutor = directExecutor,
            ) {
                outcome.set(it)
                completed.countDown()
            }
            assertTrue(completed.await(2, TimeUnit.SECONDS))
            assertEquals(
                expected,
                (outcome.get().exceptionOrNull() as ThreadlineBridgeException).status,
            )
            request.release()
        }

        client.release()
    }

    @Test
    fun releaseSuppressesLateCallbacksAndRejectsStaleHandles() {
        val callbacks = AtomicInteger()
        val client = ThreadlineClient()
        val request = client.startRequest(
            fault = ThreadlineBridgeFault.DELAYED,
            delayMilliseconds = 50,
            deliveryExecutor = directExecutor,
        ) {
            callbacks.incrementAndGet()
        }
        request.close()
        request.close()
        Thread.sleep(150)
        assertEquals(0, callbacks.get())
        request.release()
        client.release()

        assertNotEquals(
            ThreadlineBridgeStatus.OK.code,
            ThreadlineNative.nativeClientClose(Long.MAX_VALUE),
        )
    }

    @Test
    fun boundedStreamIsMonotonicAndResumesFromCursor() {
        val client = ThreadlineClient()
        val completed = CountDownLatch(1)
        val status = AtomicReference<ThreadlineBridgeStatus>()
        val events = Collections.synchronizedList(mutableListOf<Long>())
        val stream = client.startStream(
            cursor = 40,
            eventCount = 8,
            capacity = 2,
            deliveryExecutor = directExecutor,
            onEvent = {
                Thread.sleep(2)
                events += it
            },
            onCompletion = {
                status.set(it)
                completed.countDown()
            },
        )
        assertTrue(completed.await(3, TimeUnit.SECONDS))
        assertEquals((41L..48L).toList(), events.toList())
        assertEquals(ThreadlineBridgeStatus.END_OF_STREAM, status.get())
        val metrics = stream.metrics()
        assertEquals(2, metrics.capacity)
        assertTrue(metrics.maxDepth <= metrics.capacity)
        assertTrue(metrics.backpressureCount > 0)
        assertEquals(0, metrics.suppressedLateEvents)
        stream.release()

        val resumedDone = CountDownLatch(1)
        val resumedEvents = Collections.synchronizedList(mutableListOf<Long>())
        val resumed = client.startStream(
            cursor = 48,
            eventCount = 2,
            capacity = 1,
            deliveryExecutor = directExecutor,
            onEvent = { resumedEvents += it },
            onCompletion = { resumedDone.countDown() },
        )
        assertTrue(resumedDone.await(2, TimeUnit.SECONDS))
        assertEquals(listOf(49L, 50L), resumedEvents.toList())
        resumed.release()
        client.release()
    }

    @Test
    fun duplicateEventFailsBeforeDuplicateDelivery() {
        val client = ThreadlineClient()
        val completed = CountDownLatch(1)
        val status = AtomicReference<ThreadlineBridgeStatus>()
        val events = Collections.synchronizedList(mutableListOf<Long>())
        val stream = client.startStream(
            eventCount = 5,
            capacity = 5,
            fault = ThreadlineBridgeFault.DUPLICATE_EVENT,
            deliveryExecutor = directExecutor,
            onEvent = { events += it },
            onCompletion = {
                status.set(it)
                completed.countDown()
            },
        )
        assertTrue(completed.await(2, TimeUnit.SECONDS))
        assertEquals(listOf(1L, 2L), events.toList())
        assertEquals(ThreadlineBridgeStatus.PROTOCOL_VIOLATION, status.get())
        stream.release()
        client.release()
    }

    @Test
    fun jniStartFailuresPreserveStableStatusCodes() {
        val client = ThreadlineNative.nativeClientCreate()
        assertTrue(client > 0)
        assertEquals(
            -ThreadlineBridgeStatus.INVALID_ARGUMENT.code.toLong(),
            ThreadlineNative.nativeStreamStart(client, 0, 1, 0, 0, 0),
        )
        assertEquals(ThreadlineBridgeStatus.OK.code, ThreadlineNative.nativeClientRelease(client))
        assertEquals(
            -ThreadlineBridgeStatus.INVALID_HANDLE.code.toLong(),
            ThreadlineNative.nativeRequestStart(client, 0, 0),
        )
    }

    @Test
    fun oneThousandLifecycleLoopsLeaveNoNativeResources() {
        val baselineClients = ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.CLIENT)
        val baselineRequests = ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.REQUEST)
        val baselineStreams = ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.STREAM)

        repeat(1_000) {
            val client = ThreadlineNative.nativeClientCreate()
            val request = ThreadlineNative.nativeRequestStart(
                client,
                ThreadlineBridgeFault.NONE.code,
                0,
            )
            ThreadlineNative.nativeRequestWait(request, 2_000)
            ThreadlineNative.nativeRequestClose(request)
            assertEquals(0, ThreadlineNative.nativeRequestRelease(request))
            assertEquals(0, ThreadlineNative.nativeRequestRelease(request))

            val stream = ThreadlineNative.nativeStreamStart(client, 0, 0, 1, 0, 0)
            assertEquals(0, ThreadlineNative.nativeStreamNext(stream, 2_000))
            ThreadlineNative.nativeStreamClose(stream)
            assertEquals(0, ThreadlineNative.nativeStreamRelease(stream))
            assertEquals(0, ThreadlineNative.nativeStreamRelease(stream))

            ThreadlineNative.nativeClientClose(client)
            assertEquals(0, ThreadlineNative.nativeClientRelease(client))
            assertEquals(0, ThreadlineNative.nativeClientRelease(client))
        }

        assertEquals(
            baselineClients,
            ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.CLIENT),
        )
        assertEquals(
            baselineRequests,
            ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.REQUEST),
        )
        assertEquals(
            baselineStreams,
            ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.STREAM),
        )
    }
}
