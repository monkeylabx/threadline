//! Versioned FFI facade for the M0 host harness.

#![deny(unsafe_code)]

const CONTRACT_VERSION: u32 = 1;

/// Returns the version of the FFI contract.
#[allow(
    unsafe_code,
    reason = "the reviewed C facade requires one stable exported symbol"
)]
#[unsafe(no_mangle)]
pub extern "C" fn threadline_client_ffi_contract_version() -> u32 {
    CONTRACT_VERSION
}

/// JNI entry point consumed by the Android host harness.
///
/// The environment and receiver are intentionally opaque because this
/// version-only call neither reads JVM state nor retains host references.
#[allow(
    unsafe_code,
    reason = "the reviewed JNI facade requires one stable exported symbol"
)]
#[unsafe(no_mangle)]
pub extern "system" fn Java_com_threadline_android_ThreadlineAndroidSkeleton_nativeBridgeContractVersion(
    _environment: *mut core::ffi::c_void,
    _receiver: *mut core::ffi::c_void,
) -> i32 {
    CONTRACT_VERSION as i32
}

#[cfg(test)]
mod tests {
    use super::{
        threadline_client_ffi_contract_version,
        Java_com_threadline_android_ThreadlineAndroidSkeleton_nativeBridgeContractVersion,
    };

    #[test]
    fn c_and_jni_hosts_observe_the_same_contract_version() {
        assert_eq!(threadline_client_ffi_contract_version(), 1);
        assert_eq!(
            Java_com_threadline_android_ThreadlineAndroidSkeleton_nativeBridgeContractVersion(
                core::ptr::null_mut(),
                core::ptr::null_mut(),
            ),
            1,
        );
    }
}
