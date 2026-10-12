use super::TransportError;
use std::{
    future::Future,
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc,
    },
    time::Duration,
};

#[derive(Clone)]
pub(super) struct Connection {
    closed: Arc<AtomicBool>,
    deadline: Duration,
}

impl Connection {
    pub(super) fn new(deadline: Duration) -> Result<Self, TransportError> {
        if deadline.is_zero() {
            return Err(TransportError::InvalidDeadline);
        }
        Ok(Self {
            closed: Arc::new(AtomicBool::new(false)),
            deadline,
        })
    }

    fn check(&self) -> Result<(), TransportError> {
        if self.closed.load(Ordering::SeqCst) {
            return Err(TransportError::Closed);
        }
        Ok(())
    }

    fn close(&self) {
        self.closed.store(true, Ordering::SeqCst);
    }

    pub(super) async fn run<T>(
        &self,
        operation: impl Future<Output = Result<T, TransportError>>,
    ) -> Result<T, TransportError> {
        self.check()?;
        let guard = Operation {
            connection: self,
            finished: false,
        };
        let result = tokio::time::timeout(self.deadline, operation)
            .await
            .unwrap_or(Err(TransportError::Timeout));
        guard.finish(result)
    }
}

// Dropping a polled I/O future may leave a partial frame; it must never be resumed.
struct Operation<'a> {
    connection: &'a Connection,
    finished: bool,
}

impl Operation<'_> {
    fn finish<T>(mut self, result: Result<T, TransportError>) -> Result<T, TransportError> {
        self.finished = true;
        if result.is_err() {
            self.connection.close();
            return result;
        }
        self.connection.check()?;
        result
    }
}

impl Drop for Operation<'_> {
    fn drop(&mut self) {
        if !self.finished {
            self.connection.close();
        }
    }
}
