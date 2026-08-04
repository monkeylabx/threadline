//! Version-only FFI facade for the M0 host harness.

/// Returns the version of the empty FFI contract.
///
/// The facade intentionally exposes no business, message, or cryptographic API.
#[no_mangle]
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
