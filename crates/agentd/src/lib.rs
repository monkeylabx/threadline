//! Local worker primitives only; this library does not admit or launch a runtime.

pub mod acp;

mod stop;
mod stop_driver;
mod stop_report;

pub use stop::WorkerStop;
pub use stop_driver::{ContainmentState, StopDriver, StopError};
pub use stop_report::{CleanupResult, StepResult, StopReport};
