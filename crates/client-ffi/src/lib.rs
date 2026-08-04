//! Version-only FFI facade for the M0 host harness.

#![deny(unsafe_code)]

/// Returns the version of the empty FFI contract.
///
/// The facade intentionally exposes no business, message, or cryptographic API.
#[allow(
    unsafe_code,
    reason = "the reviewed C facade requires one stable exported symbol"
)]
#[unsafe(no_mangle)]
pub extern "C" fn threadline_client_ffi_contract_version() -> u32 {
    1
}

#[cfg(test)]
mod tests {
    use super::threadline_client_ffi_contract_version;

    #[test]
    fn ffi_contract_starts_at_version_one() {
        assert_eq!(threadline_client_ffi_contract_version(), 1);
    }
}
