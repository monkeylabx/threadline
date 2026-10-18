//! NOT PRODUCTION: volatile Run input primitives for a trusted fake harness.
//! No RPC listener, OS bootstrap, engine launch or durable Run transition exists.
//! Authority facts must come from the host, never from serialized UI fields.

mod admission;
mod binding;
mod intake;
mod types;

pub use intake::Intake;
pub use types::{
    AuthoritySource, GrantState, InputError, InputReceipt, LookupError, Principal, RunAuthority,
    RunInput, TrustedBinding,
};

#[cfg(test)]
mod tests;
