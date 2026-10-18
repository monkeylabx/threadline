//! Pull applies the server's committed history after the Device cursor and
//! advances the cursor only over a contiguous run.

use threadline_client_core::storage::{
    CommitPending, CommittedEvent, ConversationRef, CursorUpdate, StorageError,
};

use crate::{server::SyncFailure, Locald, LocaldError, Server};

/// What one pull did.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct PullReport {
    pub applied: u32,
    pub next_sequence: u64,
    pub has_more: bool,
}

const MAX_PULL_EVENTS: u32 = 500;

impl Locald {
    /// Fetches events after the local cursor and applies them in order. A batch
    /// must start at cursor+1 and stay contiguous; anything else stops the
    /// pull before the hole and leaves the cursor where it was. An event that
    /// is still pending locally (our own send whose ACK was lost) is folded
    /// into the committed store instead of duplicated.
    pub fn pull(
        &mut self,
        server: &mut dyn Server,
        channel_id: &str,
        max_events: u32,
    ) -> Result<PullReport, LocaldError> {
        let conversation = ConversationRef::channel(channel_id)?;
        let cursor = self.cursor(channel_id)?;
        let batch = server
            .sync(
                &self.identity.tenant_id,
                channel_id,
                cursor,
                max_events.clamp(1, MAX_PULL_EVENTS),
            )
            .map_err(|failure| match failure {
                SyncFailure::Retryable => LocaldError::Retryable,
                SyncFailure::Denied => LocaldError::Denied,
            })?;
        let mut report = PullReport {
            next_sequence: cursor,
            has_more: batch.has_more,
            ..PullReport::default()
        };
        for event in batch.events {
            if event.channel_seq != report.next_sequence + 1 {
                return Err(LocaldError::HistoryMismatch);
            }
            self.store_synced(
                conversation,
                &event.event_id,
                event.channel_seq,
                &event.envelope_bytes,
            )?;
            self.advance_cursor(conversation, report.next_sequence, event.channel_seq)?;
            report.next_sequence = event.channel_seq;
            report.applied += 1;
        }
        Ok(report)
    }

    /// store_synced inserts a committed event, folding our own pending copy
    /// when the event is one this Device sent and never heard back about.
    fn store_synced(
        &mut self,
        conversation: ConversationRef<'_>,
        event_id: &str,
        channel_seq: u64,
        envelope_bytes: &[u8],
    ) -> Result<(), LocaldError> {
        let tenant_id = self.identity.tenant_id.clone();
        let pending = self
            .database
            .load_outbox_event(&tenant_id, conversation, event_id)?;
        if pending.is_some() {
            self.database.commit_pending(CommitPending {
                tenant_id: &tenant_id,
                conversation,
                event_id,
                committed_sequence: channel_seq,
                envelope_bytes,
            })?;
            return Ok(());
        }
        match self.database.insert_committed_event(CommittedEvent {
            tenant_id: &tenant_id,
            conversation,
            event_id,
            committed_sequence: channel_seq,
            envelope_bytes,
        }) {
            Ok(()) => Ok(()),
            // Already applied earlier (e.g. ACK arrived first): a repeat of the
            // same event is not a failure as long as the sequence agrees.
            Err(StorageError::Conflict) => {
                self.verify_existing(conversation, event_id, channel_seq)
            }
            Err(error) => Err(error.into()),
        }
    }

    fn verify_existing(
        &self,
        conversation: ConversationRef<'_>,
        event_id: &str,
        channel_seq: u64,
    ) -> Result<(), LocaldError> {
        let stored = self.database.list_committed_events(
            &self.identity.tenant_id,
            conversation,
            channel_seq.saturating_sub(1),
            1,
        )?;
        match stored.first() {
            Some(record)
                if record.event_id == event_id && record.committed_sequence == channel_seq =>
            {
                Ok(())
            }
            _ => Err(LocaldError::HistoryMismatch),
        }
    }

    fn advance_cursor(
        &mut self,
        conversation: ConversationRef<'_>,
        previous: u64,
        applied: u64,
    ) -> Result<(), LocaldError> {
        let cursor_bytes = applied.to_be_bytes();
        self.database.record_cursor(CursorUpdate {
            tenant_id: &self.identity.tenant_id,
            device_id: &self.identity.device_id,
            conversation,
            expected_previous_sequence: (previous > 0).then_some(previous),
            last_applied_sequence: applied,
            cursor_bytes: &cursor_bytes,
        })?;
        Ok(())
    }
}
