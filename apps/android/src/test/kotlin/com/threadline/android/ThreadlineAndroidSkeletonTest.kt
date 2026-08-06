package com.threadline.android

import org.junit.Assert.assertEquals
import org.junit.Test

class ThreadlineAndroidSkeletonTest {
    @Test
    fun bridgeContractVersionComesFromRustFacade() {
        assertEquals(1u, ThreadlineAndroidSkeleton.bridgeContractVersion)
    }
}
