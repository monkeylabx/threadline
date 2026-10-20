use crate::local_interface::EffectFence;
use crate::{ContainmentState, StopDriver, StopError};
use std::{cell::RefCell, collections::VecDeque, rc::Rc};

#[derive(Default)]
pub struct State {
    pub calls: Vec<&'static str>,
    pub queued: bool,
    pub pending_approval: bool,
    pub synthetic_effects: usize,
}

impl State {
    // Trusted synthetic Core allow; no protected operation exists in this fake.
    pub fn allow_pending(&mut self, fence: &EffectFence) -> bool {
        if fence.closed() || !self.pending_approval || !self.queued {
            return false;
        }
        self.pending_approval = false;
        self.queued = false;
        self.synthetic_effects += 1;
        true
    }
}

pub struct Driver {
    pub state: Rc<RefCell<State>>,
    pub observations: VecDeque<Result<ContainmentState, StopError>>,
    pub fail_revocation: bool,
    pub fail_termination: bool,
}

impl Default for Driver {
    fn default() -> Self {
        Self {
            state: Rc::new(RefCell::new(State {
                queued: true,
                pending_approval: true,
                ..State::default()
            })),
            observations: [Ok(ContainmentState::SealedAndEmpty)].into(),
            fail_revocation: false,
            fail_termination: false,
        }
    }
}

impl StopDriver for Driver {
    fn revoke_effects(&mut self) -> Result<(), StopError> {
        let mut state = self.state.borrow_mut();
        state.calls.push("revoke");
        if std::mem::take(&mut self.fail_revocation) {
            return Err(StopError::Unavailable);
        }
        state.queued = false;
        state.pending_approval = false;
        Ok(())
    }

    fn interrupt(&mut self) -> Result<(), StopError> {
        self.state.borrow_mut().calls.push("interrupt");
        Ok(())
    }

    fn terminate(&mut self) -> Result<(), StopError> {
        self.state.borrow_mut().calls.push("terminate");
        if std::mem::take(&mut self.fail_termination) {
            return Err(StopError::Rejected);
        }
        Ok(())
    }

    fn observe_containment(&mut self) -> Result<ContainmentState, StopError> {
        self.state.borrow_mut().calls.push("observe");
        self.observations
            .pop_front()
            .expect("unexpected cleanup retry")
    }
}

#[test]
fn synthetic_allow_has_an_observable_effect_only_while_pending() {
    let driver = Driver::default();
    let owner = super::flow::owner();
    let fence = owner.effect_fence();
    let mut state = driver.state.borrow_mut();
    assert!(state.allow_pending(&fence));
    assert!(!state.allow_pending(&fence));
    assert_eq!(state.synthetic_effects, 1);
}
