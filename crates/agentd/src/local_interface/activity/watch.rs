use super::{observer, Activity, AuthoritySource, Error, Principal, Request};
use prost::Message;
use std::fmt;

/// An owned in-process cursor capability; an ID alone never reconstructs this binding.
pub struct Watch {
    identity: [String; 5],
    run_id: String,
    incarnation: String,
    stream_id: String,
    after: u64,
    ready: bool,
    closed: bool,
}

fn identity(principal: Principal<'_>) -> [&str; 5] {
    [
        principal.tenant,
        principal.actor,
        principal.device,
        principal.session,
        principal.window,
    ]
}

impl Watch {
    pub(super) fn new(activity: &Activity, principal: Principal<'_>, serial: u64) -> Self {
        Self {
            identity: identity(principal).map(str::to_owned),
            run_id: activity.buffer.run_id.clone(),
            incarnation: activity.incarnation.clone(),
            stream_id: format!("{}:{serial:016x}", activity.incarnation),
            after: activity.buffer.floor() - 1,
            ready: true,
            closed: false,
        }
    }

    fn check(
        &self,
        source: &dyn AuthoritySource,
        activity: &Activity,
        run: &str,
    ) -> Result<(), Error> {
        let principal = observer(source, run)?;
        if self.closed
            || identity(principal) != self.identity.each_ref().map(String::as_str)
            || run != self.run_id
            || activity.buffer.run_id != self.run_id
            || activity.incarnation != self.incarnation
        {
            return Err(Error::CursorInvalid);
        }
        Ok(())
    }

    /// Reconnect requires the retained host handle as well as the existing wire cursor.
    pub fn reconnect(
        &self,
        source: &dyn AuthoritySource,
        activity: &Activity,
        request: &Request,
    ) -> Result<Self, Error> {
        self.check(source, activity, &request.run_id)?;
        if request.stream_id != self.stream_id {
            return Err(Error::CursorInvalid);
        }
        activity.buffer.check_cursor(request.after_sequence)?;
        Ok(Self {
            identity: self.identity.clone(),
            run_id: self.run_id.clone(),
            incarnation: self.incarnation.clone(),
            stream_id: self.stream_id.clone(),
            after: request.after_sequence,
            ready: true,
            closed: false,
        })
    }

    /// Fresh authority is checked even for ready and empty polls. Any failure ends this handle.
    pub fn next(
        &mut self,
        source: &dyn AuthoritySource,
        activity: &Activity,
    ) -> Result<Option<Vec<u8>>, Error> {
        let authorization = self.check(source, activity, &activity.buffer.run_id);
        self.closed = true;
        authorization?;
        activity.buffer.check_cursor(self.after)?;
        let frame = if self.ready {
            Some(activity.buffer.ready())
        } else {
            activity.buffer.next(self.after)
        };
        self.closed = false;
        let Some(mut frame) = frame else {
            return Ok(None);
        };
        frame.stream_id = self.stream_id.clone();
        if self.ready {
            self.ready = false;
        } else {
            self.after = frame.sequence;
        }
        Ok(Some(frame.encode_to_vec()))
    }
}

impl fmt::Debug for Watch {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Watch").finish_non_exhaustive()
    }
}
