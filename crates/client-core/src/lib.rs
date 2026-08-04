//! Dependency-free client-core workspace skeleton.

/// Version of the empty host contract exposed by the M0 skeleton.
pub const HOST_CONTRACT_VERSION: u32 = 1;

#[cfg(test)]
mod tests {
    use super::HOST_CONTRACT_VERSION;

    #[test]
    fn host_contract_starts_at_version_one() {
        assert_eq!(HOST_CONTRACT_VERSION, 1);
    }
}
