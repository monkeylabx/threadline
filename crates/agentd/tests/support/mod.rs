use std::{cell::RefCell, collections::VecDeque, rc::Rc};
use threadline_agentd::{ContainmentState, StopDriver, StopError, WorkerStop};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Call {
    Revoke,
    Interrupt,
    Terminate,
    Observe,
}

pub type Trace = Rc<RefCell<Vec<Call>>>;

pub struct Script {
    pub revocations: VecDeque<Result<(), StopError>>,
    pub interruptions: VecDeque<Result<(), StopError>>,
    pub terminations: VecDeque<Result<(), StopError>>,
    pub observations: VecDeque<Result<ContainmentState, StopError>>,
}

impl Default for Script {
    fn default() -> Self {
        Self {
            revocations: [Ok(())].into(),
            interruptions: [Ok(())].into(),
            terminations: [Ok(())].into(),
            observations: [Ok(ContainmentState::SealedAndEmpty)].into(),
        }
    }
}

pub struct FakeDriver {
    script: Script,
    trace: Trace,
}

// Exhausting a script is a test assertion: an unexpected effect must fail the test.
fn take<T>(trace: &Trace, call: Call, results: &mut VecDeque<T>) -> T {
    trace.borrow_mut().push(call);
    results
        .pop_front()
        .expect("unexpected repeated stop effect")
}

impl StopDriver for FakeDriver {
    fn revoke_effects(&mut self) -> Result<(), StopError> {
        take(&self.trace, Call::Revoke, &mut self.script.revocations)
    }

    fn interrupt(&mut self) -> Result<(), StopError> {
        take(&self.trace, Call::Interrupt, &mut self.script.interruptions)
    }

    fn terminate(&mut self) -> Result<(), StopError> {
        take(&self.trace, Call::Terminate, &mut self.script.terminations)
    }

    fn observe_containment(&mut self) -> Result<ContainmentState, StopError> {
        take(&self.trace, Call::Observe, &mut self.script.observations)
    }
}

pub fn fixture(script: Script) -> (WorkerStop<FakeDriver>, Trace) {
    let trace = Rc::new(RefCell::new(Vec::new()));
    let driver = FakeDriver {
        script,
        trace: Rc::clone(&trace),
    };
    (WorkerStop::new(driver), trace)
}
