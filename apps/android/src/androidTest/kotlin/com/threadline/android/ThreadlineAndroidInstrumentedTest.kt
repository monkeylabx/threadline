package com.threadline.android

import android.os.Handler
import android.os.Looper
import java.util.Collections
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import junit.framework.TestCase

class ThreadlineAndroidInstrumentedTest : TestCase() {
    private val directExecutor = Executor { command -> command.run() }
    private val mainExecutor = Executor { command -> Handler(Looper.getMainLooper()).post(command) }

    fun testFacadeExecutesInsideAndroidEmulatorOnMainDispatcherWithoutBlockingCaller() {
        assertEquals(1u, ThreadlineAndroidSkeleton.bridgeContractVersion)
        val client = ThreadlineClient()
        val completed = CountDownLatch(1)
        val callbackWasOnMain = AtomicBoolean(false)
        val result = AtomicReference<Result<ThreadlineRequestResult>>()

        val started = System.nanoTime()
        val request = client.startRequest(
            fault = ThreadlineBridgeFault.DELAYED,
            delayMilliseconds = 200,
            deliveryExecutor = mainExecutor,
        ) {
            callbackWasOnMain.set(Looper.myLooper() == Looper.getMainLooper())
            result.set(it)
            completed.countDown()
        }
        val elapsedMilliseconds = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started)
        assertTrue("start must not block the instrumentation thread", elapsedMilliseconds < 100)

        assertTrue(completed.await(3, TimeUnit.SECONDS))
        assertTrue(callbackWasOnMain.get())
        assertTrue(result.get().getOrThrow().data.contentEquals("threadline-ok".encodeToByteArray()))
        request.release()
        client.release()
    }

    fun testFaultFixtureCancellationAndStableErrorMappingInsideEmulator() {
        val client = ThreadlineClient()

        val canceled = CountDownLatch(1)
        val canceledOutcome = AtomicReference<Result<ThreadlineRequestResult>>()
        val beforeCommit = client.startRequest(
            fault = ThreadlineBridgeFault.DELAYED,
            delayMilliseconds = 100,
            deliveryExecutor = directExecutor,
        ) {
            canceledOutcome.set(it)
            canceled.countDown()
        }
        assertEquals(ThreadlineBridgeStatus.OK, beforeCommit.cancel())
        assertEquals(ThreadlineBridgeStatus.OK, beforeCommit.cancel())
        assertTrue(canceled.await(2, TimeUnit.SECONDS))
        assertEquals(
            ThreadlineBridgeStatus.CANCELED,
            (canceledOutcome.get().exceptionOrNull() as ThreadlineBridgeException).status,
        )
        beforeCommit.release()

        val nativeClient = ThreadlineNative.nativeClientCreate()
        assertTrue(nativeClient > 0)
        val afterCommit = ThreadlineNative.nativeRequestStart(
            nativeClient,
            ThreadlineBridgeFault.NONE.code,
            200,
        )
        assertTrue(afterCommit > 0)
        val commitDeadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(2)
        while (
            ThreadlineNative.nativeRequestState(afterCommit) != 12 &&
            System.nanoTime() < commitDeadline
        ) {
            Thread.yield()
        }
        assertEquals(12, ThreadlineNative.nativeRequestState(afterCommit))
        assertEquals(
            ThreadlineBridgeStatus.ALREADY_COMMITTED.code,
            ThreadlineNative.nativeRequestCancel(afterCommit),
        )
        assertEquals(
            ThreadlineBridgeStatus.OK.code,
            ThreadlineNative.nativeRequestWait(afterCommit, 2_000),
        )
        assertEquals(0, ThreadlineNative.nativeRequestRelease(afterCommit))
        assertEquals(0, ThreadlineNative.nativeClientRelease(nativeClient))

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

        assertEquals(
            -ThreadlineBridgeStatus.INVALID_HANDLE.code.toLong(),
            ThreadlineNative.nativeRequestStart(Long.MAX_VALUE, 0, 0),
        )
        val invalidArgumentClient = ThreadlineNative.nativeClientCreate()
        assertTrue(invalidArgumentClient > 0)
        assertEquals(
            -ThreadlineBridgeStatus.INVALID_ARGUMENT.code.toLong(),
            ThreadlineNative.nativeStreamStart(
                invalidArgumentClient,
                0,
                1,
                0,
                0,
                0,
            ),
        )
        assertEquals(0, ThreadlineNative.nativeClientRelease(invalidArgumentClient))

        client.release()
    }

    fun testFaultFixtureStreamsBackpressureResumeAndLateSuppressionInsideEmulator() {
        val client = ThreadlineClient()
        val completed = CountDownLatch(1)
        val terminal = AtomicReference<ThreadlineBridgeStatus>()
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
                terminal.set(it)
                completed.countDown()
            },
        )
        assertTrue(completed.await(3, TimeUnit.SECONDS))
        assertEquals((41L..48L).toList(), events.toList())
        assertEquals(ThreadlineBridgeStatus.END_OF_STREAM, terminal.get())
        val metrics = stream.metrics()
        assertEquals(2, metrics.capacity)
        assertTrue(metrics.maxDepth <= metrics.capacity)
        assertTrue(metrics.backpressureCount > 0)
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

        val duplicateDone = CountDownLatch(1)
        val duplicateStatus = AtomicReference<ThreadlineBridgeStatus>()
        val duplicateEvents = Collections.synchronizedList(mutableListOf<Long>())
        val duplicate = client.startStream(
            eventCount = 5,
            capacity = 5,
            fault = ThreadlineBridgeFault.DUPLICATE_EVENT,
            deliveryExecutor = directExecutor,
            onEvent = { duplicateEvents += it },
            onCompletion = {
                duplicateStatus.set(it)
                duplicateDone.countDown()
            },
        )
        assertTrue(duplicateDone.await(2, TimeUnit.SECONDS))
        assertEquals(listOf(1L, 2L), duplicateEvents.toList())
        assertEquals(ThreadlineBridgeStatus.PROTOCOL_VIOLATION, duplicateStatus.get())
        duplicate.release()

        val firstLateEvent = CountDownLatch(1)
        val lateCompletions = AtomicInteger()
        val late = client.startStream(
            eventCount = 1,
            capacity = 1,
            delayMilliseconds = 20,
            fault = ThreadlineBridgeFault.LATE_EVENT,
            deliveryExecutor = directExecutor,
            onEvent = { firstLateEvent.countDown() },
            onCompletion = { lateCompletions.incrementAndGet() },
        )
        assertTrue(firstLateEvent.await(2, TimeUnit.SECONDS))
        late.close()
        val lateDeadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(2)
        while (late.metrics().suppressedLateEvents == 0L && System.nanoTime() < lateDeadline) {
            Thread.yield()
        }
        assertEquals(1L, late.metrics().suppressedLateEvents)
        Thread.sleep(50)
        assertEquals(0, lateCompletions.get())
        late.release()

        client.release()
    }

    fun testEmulatorRunsOneThousandCreateStartCloseReleaseLoops() {
        val baselineClients = ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.CLIENT)
        val baselineRequests = ThreadlineAndroidSkeleton.resourceCount(ThreadlineBridgeResource.REQUEST)

        repeat(1_000) {
            val client = ThreadlineNative.nativeClientCreate()
            val request = ThreadlineNative.nativeRequestStart(client, 0, 0)
            assertEquals(0, ThreadlineNative.nativeRequestWait(request, 2_000))
            assertEquals(0, ThreadlineNative.nativeRequestClose(request))
            assertEquals(0, ThreadlineNative.nativeRequestRelease(request))
            assertEquals(0, ThreadlineNative.nativeRequestRelease(request))
            assertEquals(0, ThreadlineNative.nativeClientClose(client))
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
    }
}
