//! Inert process target until the Runtime workstream admits the supervisor.

use std::process::ExitCode;

fn main() -> ExitCode {
    eprintln!("threadline-agentd is unavailable: runtime admission is pending");
    ExitCode::FAILURE
}
