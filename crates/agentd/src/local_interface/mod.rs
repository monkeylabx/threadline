//! NOT PRODUCTION: volatile local Run interaction for a trusted fake harness.
//! No RPC listener, OS bootstrap, engine launch or durable Run transition exists.
//! Authority facts must come from the host, never from serialized UI fields.

mod activity;
mod admission;
mod binding;
mod fake;
mod intake;
mod types;

pub use activity::{Activity, ActivityUpdate, Watch};
pub use fake::{CleanupConclusion, FakeRun, OwnerReceipt, OwnerReporter, Shutdown};
pub use intake::Intake;
pub use types::{
    AuthoritySource, GrantState, InputError, InputReceipt, LookupError, Principal, RunAuthority,
    RunInput, TrustedBinding,
};

#[cfg(test)]
mod tests;
