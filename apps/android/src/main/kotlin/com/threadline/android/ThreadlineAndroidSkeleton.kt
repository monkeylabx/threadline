package com.threadline.android

object ThreadlineAndroidSkeleton {
    init {
        System.loadLibrary("threadline_client_ffi")
    }

    private external fun nativeBridgeContractVersion(): Int

    val bridgeContractVersion: UInt
        get() = nativeBridgeContractVersion().toUInt()
}
