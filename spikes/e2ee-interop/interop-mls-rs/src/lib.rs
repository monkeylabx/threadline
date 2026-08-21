//! Cross-implementation interoperability harness for ADR-0003 / ADR-0004.
//!
//! The evidence lives in `tests/cross_implementation.rs`; this crate carries no
//! library code and is never linked into a Threadline artifact.

/// Version of the cross-implementation transcript this harness drives.
pub const INTEROP_HARNESS_VERSION: u32 = 1;
