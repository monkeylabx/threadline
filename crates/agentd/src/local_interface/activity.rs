//! Volatile synthetic views only; no raw ACP/tool payload enters this interface.
use super::{admission, binding, AuthoritySource, InputError as Error, Principal};
use std::fmt;
use threadline_client_proto::threadline::local_agent::v1::{
    LocalToolPhase, WatchRunActivityRequest as Request,
};

mod buffer;
mod watch;
use buffer::Buffer;
pub use watch::Watch;

pub enum ActivityUpdate<'a> {
    Text(&'a str),
    SyntheticTool(LocalToolPhase),
    Suppressed,
}

/// NOT PRODUCTION. The trusted harness supplies a fresh incarnation, never the UI.
pub struct Activity {
    buffer: Buffer,
    incarnation: String,
    serial: u64,
}

impl Activity {
    pub fn new(run_id: String, incarnation: String) -> Result<Self, Error> {
        if !valid_id(&run_id) || !valid_id(&incarnation) {
            return Err(Error::InvalidArgument);
        }
        Ok(Self {
            buffer: Buffer::new(run_id, format!("{incarnation}:{:016x}", 0)),
            incarnation,
            serial: 0,
        })
    }

    /// Approved aggregate text or a fixed synthetic tool label, never raw updates.
    pub fn append(&mut self, input_id: &str, update: ActivityUpdate<'_>) -> Result<(), Error> {
        if !valid_id(input_id) {
            return Err(Error::InvalidArgument);
        }
        self.buffer.append(input_id, update)
    }

    pub fn watch(
        &mut self,
        source: &dyn AuthoritySource,
        request: &Request,
    ) -> Result<Watch, Error> {
        let principal = observer(source, &request.run_id)?;
        if request.run_id != self.buffer.run_id
            || !request.stream_id.is_empty()
            || request.after_sequence != 0
        {
            return Err(Error::CursorInvalid);
        }
        self.serial = self.serial.checked_add(1).ok_or(Error::ResourceExhausted)?;
        Ok(Watch::new(self, principal, self.serial))
    }

    /// Trusted lifecycle hook; denied callers must not clear other observers' data.
    pub fn clear(&mut self) {
        self.buffer.clear();
    }
}

fn valid_id(value: &str) -> bool {
    !value.is_empty() && value.len() <= 128 && value.bytes().all(|b| b.is_ascii_graphic())
}

fn observer<'a>(source: &'a dyn AuthoritySource, run_id: &str) -> Result<Principal<'a>, Error> {
    let (principal, run) = binding::authorized_run(source, run_id)?;
    if !run.observe {
        return Err(Error::PermissionDenied);
    }
    admission::grant(run.grant)?;
    Ok(principal)
}

impl fmt::Debug for Activity {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Activity").finish_non_exhaustive()
    }
}
