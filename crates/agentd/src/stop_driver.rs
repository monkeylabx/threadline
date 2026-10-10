/// Redacted adapter failures. Raw worker output and OS errors must stay out of reports.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StopError {
    Unavailable,
    Rejected,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ContainmentState {
    LiveMembers,
    /// Independent proof that the owned container is sealed, empty, and reaped.
    /// No new member may join after this observation. Parent exit is insufficient.
    SealedAndEmpty,
}

/// Stop effects for one exclusively owned worker containment boundary.
///
/// Implementations must use stable container ownership, never a reusable bare PID.
/// Each call must return within an enforced adapter deadline. Termination and
/// revocation must be idempotent; observation must independently inspect containment.
/// An unsealed or unobservable container must not report `SealedAndEmpty`.
/// This contract has only a fake implementation; OS/ACP adapters need separate review.
pub trait StopDriver {
    /// Revoke queued effects, tool authority, and pending approval execution.
    fn revoke_effects(&mut self) -> Result<(), StopError>;
    /// Best-effort ACP interruption. A response is never cleanup evidence.
    fn interrupt(&mut self) -> Result<(), StopError>;
    /// Terminate the owned containment boundary, including descendants.
    fn terminate(&mut self) -> Result<(), StopError>;
    fn observe_containment(&mut self) -> Result<ContainmentState, StopError>;
}
