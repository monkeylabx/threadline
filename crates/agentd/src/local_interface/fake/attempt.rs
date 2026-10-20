use crate::local_interface::Intake;

/// Only a validated known outcome can disarm cancellation uncertainty.
pub(super) struct Attempt<'a> {
    intake: &'a mut Intake,
    completed: bool,
}

impl<'a> Attempt<'a> {
    pub fn new(intake: &'a mut Intake) -> Self {
        Self {
            intake,
            completed: false,
        }
    }

    pub fn complete(mut self) {
        self.intake.finish_turn();
        self.completed = true;
    }
}

impl Drop for Attempt<'_> {
    fn drop(&mut self) {
        if !self.completed {
            self.intake.mark_uncertain();
        }
    }
}
