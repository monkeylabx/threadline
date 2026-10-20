use std::sync::{
    atomic::{AtomicBool, Ordering},
    Arc,
};

/// Read-only retirement latch for this Run's trusted queued-effect dispatcher.
/// An open fence is not authorization; current effect/Approval checks still apply.
#[derive(Clone)]
pub struct EffectFence(Arc<AtomicBool>);

impl EffectFence {
    pub(super) fn new() -> Self {
        Self(Arc::new(AtomicBool::new(false)))
    }

    pub fn closed(&self) -> bool {
        self.0.load(Ordering::Acquire)
    }

    pub(super) fn close(&self) {
        self.0.store(true, Ordering::Release);
    }
}
