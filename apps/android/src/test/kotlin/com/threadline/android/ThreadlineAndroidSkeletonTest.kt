package com.threadline.android

import org.junit.Assert.assertEquals
import org.junit.Test

class ThreadlineAndroidSkeletonTest {
    @Test
    fun bridgeContractVersionIsStable() {
        assertEquals(1u, ThreadlineAndroidSkeleton.BRIDGE_CONTRACT_VERSION)
    }
}
