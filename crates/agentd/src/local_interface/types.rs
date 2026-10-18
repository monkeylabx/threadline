use std::fmt;

/// A trusted current identity snapshot, not a local request or a credential.
#[derive(Clone, Copy)]
pub struct Principal<'a> {
    pub tenant: &'a str,
    pub actor: &'a str,
    pub device: &'a str,
    pub session: &'a str,
    pub window: &'a str,
    pub active: bool,
    pub unexpired: bool,
}

pub struct TrustedBinding<'a> {
    pub principal: Option<Principal<'a>>,
    pub peer_authenticated: bool,
    pub session: &'a str,
    pub window: &'a str,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum GrantState {
    Current,
    Revoked,
    Expired,
    Unavailable,
}

/// Read-only facts supplied by the trusted owner/Core path, never a local state authority.
pub struct RunAuthority<'a> {
    pub id: &'a str,
    pub tenant: &'a str,
    pub actor: &'a str,
    pub device: &'a str,
    pub control: bool,
    pub owner: bool,
    pub lease: bool,
    pub fencing: bool,
    pub grant: GrantState,
    pub route: bool,
    pub budget: bool,
    pub live: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum LookupError {
    Unknown,
    Unavailable,
}

/// Construct only in a trusted harness/host; never derive these facts from request fields.
pub trait AuthoritySource {
    fn binding(&self) -> TrustedBinding<'_>;
    fn run(&self, run_id: &str) -> Result<RunAuthority<'_>, LookupError>;
}

pub struct RunInput {
    pub run_id: String,
    pub input_id: String,
    pub text: String,
}

impl fmt::Debug for RunInput {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("RunInput").finish_non_exhaustive()
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum InputReceipt {
    Accepted,
    Duplicate,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum InputError {
    Unauthenticated,
    PermissionDenied,
    TenantMismatch,
    NotExecutionOwner,
    LeaseLost,
    FencingTokenStale,
    GrantRevoked,
    GrantExpired,
    Unavailable,
    ResourceExhausted,
    InvalidStateTransition,
    InvalidArgument,
    PayloadTooLarge,
    RunInputOutcomeUncertain,
    IdempotencyConflict,
}

impl fmt::Display for InputError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "local Run input {self:?}")
    }
}

impl std::error::Error for InputError {}
