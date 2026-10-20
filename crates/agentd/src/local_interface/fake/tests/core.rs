use crate::local_interface::{CleanupConclusion, InputError as Error, OwnerReporter};

#[derive(Default)]
pub struct Core {
    pub calls: usize,
    pub available: bool,
    pub acknowledge: bool,
    pub terminal: Option<(String, CleanupConclusion)>,
}

impl OwnerReporter for Core {
    fn report(&mut self, run: &str, conclusion: CleanupConclusion) -> Result<bool, Error> {
        self.calls += 1;
        if !self.available {
            return Err(Error::Unavailable);
        }
        if !self.acknowledge {
            return Ok(false);
        }
        let exact = (run.to_owned(), conclusion);
        if let Some(previous) = &self.terminal {
            return if previous == &exact {
                Ok(true)
            } else {
                Err(Error::InvalidStateTransition)
            };
        }
        self.terminal = Some(exact);
        Ok(true)
    }
}
