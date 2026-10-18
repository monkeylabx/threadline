//! Replay sends pending events in enqueue order and folds each Durable ACK
//! into the committed store in one local transaction.

use threadline_client_core::storage::{CommitPending, ConversationRef};

use crate::{server::SendFailure, Locald, LocaldError, SendRequest, Server};

/// What one replay pass did. `retained` counts items still pending because the
/// pass stopped (retryable failure) or the server refused them.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct ReplayReport {
    pub committed: u32,
    pub deduplicated: u32,
    pub retained: u32,
}

impl Locald {
    /// Sends every pending event of a Channel in enqueue order. A retryable
    /// failure stops the pass so order is preserved; a denial or conflict
    /// leaves that item pending and continues, so one bad item never blocks
    /// the rest forever while still never being dropped.
    pub fn replay(
        &mut self,
        server: &mut dyn Server,
        channel_id: &str,
    ) -> Result<ReplayReport, LocaldError> {
        let conversation = ConversationRef::channel(channel_id)?;
        let pending = self
            .database
            .list_pending_outbox(&self.identity.tenant_id, conversation)?;
        let mut report = ReplayReport::default();
        let mut stopped = false;
        for item in pending {
            if stopped {
                report.retained += 1;
                continue;
            }
            let request = SendRequest {
                tenant_id: self.identity.tenant_id.clone(),
                channel_id: channel_id.to_owned(),
                event_id: item.event_id.clone(),
                idempotency_key: item.idempotency_key.clone(),
                envelope_bytes: item.envelope_bytes.clone(),
            };
            match server.send(request) {
                Ok(ack) => {
                    if ack.event_id != item.event_id || ack.idempotency_key != item.idempotency_key
                    {
                        return Err(LocaldError::HistoryMismatch);
                    }
                    self.database.commit_pending(CommitPending {
                        tenant_id: &self.identity.tenant_id,
                        conversation,
                        event_id: &item.event_id,
                        committed_sequence: ack.channel_seq,
                        envelope_bytes: &ack.envelope_bytes,
                    })?;
                    if ack.deduplicated {
                        report.deduplicated += 1;
                    } else {
                        report.committed += 1;
                    }
                }
                Err(SendFailure::Retryable) => {
                    stopped = true;
                    report.retained += 1;
                }
                Err(SendFailure::Denied | SendFailure::Conflict) => {
                    report.retained += 1;
                }
            }
        }
        Ok(report)
    }
}
