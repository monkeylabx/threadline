//! Network wait and durable reconciliation have separate lock lifetimes.
use crate::{
    dto::{HostError, SyncReportDto},
    session::Session,
};
use std::sync::{
    atomic::{AtomicBool, Ordering},
    Arc, Mutex,
};
use threadline_core_client::ClientError;
use threadline_locald::Server;

pub(crate) fn reconcile<S: Server>(
    shared: &Mutex<Option<Session<S>>>,
    channel: &str,
    wait: impl FnOnce() -> Result<bool, ClientError>,
) -> Result<SyncReportDto, HostError> {
    // A failed notification is a reason to reconcile, never a cursor authority.
    let woke = wait().unwrap_or(false);
    let mut guard = shared.lock().map_err(|_| HostError::Storage)?;
    let mut report = guard.as_mut().ok_or(HostError::NoSession)?.sync(channel)?;
    report.realtime_woke = woke;
    Ok(report)
}

/// Bounds host waits even if the renderer reloads or invokes concurrently.
pub(crate) struct Permit(Arc<AtomicBool>);
impl Permit {
    pub(crate) fn acquire(gate: Arc<AtomicBool>) -> Result<Self, HostError> {
        gate.compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .map_err(|_| HostError::Unavailable)?;
        Ok(Self(gate))
    }
}
impl Drop for Permit {
    fn drop(&mut self) {
        self.0.store(false, Ordering::Release);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn only_one_host_wait_is_admitted_and_release_allows_reconnect() {
        let gate = Arc::new(AtomicBool::new(false));
        let permit = Permit::acquire(gate.clone()).unwrap();
        assert!(matches!(
            Permit::acquire(gate.clone()),
            Err(HostError::Unavailable)
        ));
        drop(permit);
        assert!(Permit::acquire(gate).is_ok());
    }
}
