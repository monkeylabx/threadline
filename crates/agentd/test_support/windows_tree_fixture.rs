use super::{api, JobError, LEAF, ROOT};
use std::{
    io::{Read, Write},
    os::windows::{
        io::{AsHandle, AsRawHandle},
        process::CommandExt,
    },
    process::{Command, Stdio},
};
use windows_sys::Win32::System::Threading::CREATE_NO_WINDOW;

#[test]
fn windows_tree_root_entry() {
    if selected(ROOT) {
        root().expect("fixed cooperative root");
        std::process::exit(42);
    }
}

#[test]
fn windows_tree_descendant_entry() {
    if selected(LEAF) {
        api::park();
        std::process::exit(77);
    }
}

fn selected(name: &str) -> bool {
    std::env::args().skip(1).take(2).eq(["--exact", name])
}

fn root() -> Result<(), JobError> {
    // The descendant must inherit neither acknowledgement nor reply pipes.
    api::inherit(std::io::stdin().as_handle(), false)?;
    api::inherit(std::io::stderr().as_handle(), false)?;
    api::inherit(std::io::stdout().as_handle(), false)?;
    let child = Command::new(std::env::current_exe().map_err(|_| JobError::Unavailable)?)
        .args(["--exact", LEAF, "--quiet", "--test-threads=1"])
        .env_clear()
        .env(
            "SystemRoot",
            std::env::var_os("SystemRoot").ok_or(JobError::Unavailable)?,
        )
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .creation_flags(CREATE_NO_WINDOW)
        .spawn()
        .map_err(|_| JobError::Unavailable)?;
    let value = child.as_raw_handle() as usize as u64;
    std::io::stderr()
        .write_all(&value.to_le_bytes())
        .map_err(|_| JobError::Unavailable)?;
    let mut ack = [0];
    std::io::stdin()
        .read_exact(&mut ack)
        .map_err(|_| JobError::Unavailable)?;
    if ack != [1] {
        return Err(JobError::Rejected);
    }
    // Parent now owns its exact duplicate. Dropping std Child closes our reference only;
    // it does not terminate/wait the leaf. The fixed root then exits42.
    Ok(())
}
