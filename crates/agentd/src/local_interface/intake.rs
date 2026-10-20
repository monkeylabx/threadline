use super::{admission, AuthoritySource, InputError as Error, InputReceipt as Receipt, RunInput};
use std::fmt;

#[derive(Debug, Eq, PartialEq)]
enum Phase {
    Ready,
    Active,
    Uncertain,
}

struct Record {
    input_id: String,
    text: String,
}

/// Volatile demo intake for one dispatched Run, not a source of Run authority.
/// Engine dispatch, transport and durable Core outcomes remain outside this intake.
pub struct Intake {
    run_id: String,
    receipts: Vec<Record>,
    phase: Phase,
}

impl Intake {
    pub fn new(run_id: String) -> Self {
        Self {
            run_id,
            receipts: Vec::new(),
            phase: Phase::Ready,
        }
    }

    /// Recheck current trusted authority even when returning an old receipt.
    pub fn accept(
        &mut self,
        source: &dyn AuthoritySource,
        input: &RunInput,
    ) -> Result<Receipt, Error> {
        admission::check_input(source, &input.run_id)?;
        if input.run_id != self.run_id {
            return Err(Error::PermissionDenied);
        }
        let receipt = self.decision(input)?;
        if receipt == Receipt::Accepted {
            self.receipts.push(Record {
                input_id: input.input_id.clone(),
                text: input.text.clone(),
            });
            self.phase = Phase::Active;
        }
        Ok(receipt)
    }

    /// Trusted owner hook after a known local turn outcome, never a UI RPC.
    pub fn finish_turn(&mut self) {
        if self.phase == Phase::Active {
            self.phase = Phase::Ready;
        }
    }

    /// Unknown effects cannot be cleared by a new input ID or turn completion.
    pub fn mark_uncertain(&mut self) {
        self.phase = Phase::Uncertain;
    }

    /// Trusted revocation/terminal hook; denied callers must not invoke this.
    pub fn clear(&mut self) {
        self.receipts.clear();
        self.mark_uncertain();
    }

    fn decision(&self, input: &RunInput) -> Result<Receipt, Error> {
        validate(input)?;
        if self.phase == Phase::Uncertain {
            return Err(Error::RunInputOutcomeUncertain);
        }
        // At 16 receipts, a bounded scan keeps exact bytes without eviction.
        if let Some(record) = self.receipts.iter().find(|r| r.input_id == input.input_id) {
            return if record.text == input.text {
                Ok(Receipt::Duplicate)
            } else {
                Err(Error::IdempotencyConflict)
            };
        }
        if self.phase == Phase::Active {
            return Err(Error::InvalidStateTransition);
        }
        if self.receipts.len() >= 16 {
            return Err(Error::ResourceExhausted);
        }
        Ok(Receipt::Accepted)
    }
}

fn validate(input: &RunInput) -> Result<(), Error> {
    if input.input_id.is_empty()
        || input.input_id.len() > 128
        || !input.input_id.bytes().all(|byte| byte.is_ascii_graphic())
        || input.text.is_empty()
    {
        return Err(Error::InvalidArgument);
    }
    if input.text.len() > 65_536 {
        return Err(Error::PayloadTooLarge);
    }
    Ok(())
}

impl fmt::Debug for Intake {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Intake")
            .field("accepted_inputs", &self.receipts.len())
            .field("phase", &self.phase)
            .finish_non_exhaustive()
    }
}
