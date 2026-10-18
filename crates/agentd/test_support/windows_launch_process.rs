#[path = "windows_launch_command.rs"]
mod command;
use super::{attributes::Attributes, job_api, JobError};
use std::{
    mem::size_of,
    os::windows::io::BorrowedHandle,
    ptr::{null, null_mut},
};
use windows_sys::Win32::{
    Foundation::{HANDLE, WAIT_OBJECT_0},
    System::{
        JobObjects::IsProcessInJob,
        Threading::{
            CreateProcessW, GetExitCodeProcess, ResumeThread, TerminateProcess,
            WaitForSingleObject, CREATE_NO_WINDOW, CREATE_SUSPENDED, CREATE_UNICODE_ENVIRONMENT,
            EXTENDED_STARTUPINFO_PRESENT, PROCESS_INFORMATION, STARTF_USESTDHANDLES,
            STARTUPINFOEXW, STARTUPINFOW,
        },
    },
};

pub(super) struct Handles {
    pub(super) process: HANDLE,
    pub(super) thread: HANDLE,
}

impl Drop for Handles {
    fn drop(&mut self) {
        // Test boundary, not cleanup proof: each successful close clears its owned reference.
        for handle in [&mut self.thread, &mut self.process] {
            job_api::close(*handle).expect("synthetic process reference release unavailable");
            *handle = null_mut();
        }
    }
}

pub(super) fn create(attributes: &mut Attributes<'_>) -> Result<Handles, JobError> {
    create_input(attributes, command::Input::single()?)
}

pub(super) fn create_root(
    attributes: &mut Attributes<'_>,
    stdio: [BorrowedHandle<'_>; 3],
) -> Result<Handles, JobError> {
    create_input(attributes, command::Input::root(stdio)?)
}

fn create_input(
    attributes: &mut Attributes<'_>,
    mut input: command::Input<'_>,
) -> Result<Handles, JobError> {
    let inherit = input.stdio.is_some();
    let stdio = input.raw_stdio();
    let startup = STARTUPINFOEXW {
        StartupInfo: STARTUPINFOW {
            cb: size_of::<STARTUPINFOEXW>() as u32,
            dwFlags: if inherit { STARTF_USESTDHANDLES } else { 0 },
            hStdInput: stdio[0],
            hStdError: stdio[1],
            hStdOutput: stdio[2],
            ..Default::default()
        },
        lpAttributeList: attributes.pointer(),
    };
    let mut info = PROCESS_INFORMATION::default();
    // SAFETY: explicit trusted self-image, writable UTF-16 command, double-NUL environment;
    // live initialized attribute/value buffers; matching extended structure size and flag.
    // Inheritance is disabled for W1; W2 allows only its three explicitly listed pipe/NUL handles.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exceptions in #247/#249")]
    let status = unsafe {
        CreateProcessW(
            input.application.as_ptr(),
            input.command.as_mut_ptr(),
            null(),
            null(),
            i32::from(inherit),
            CREATE_SUSPENDED
                | EXTENDED_STARTUPINFO_PRESENT
                | CREATE_UNICODE_ENVIRONMENT
                | CREATE_NO_WINDOW,
            input.environment.as_ptr().cast(),
            null(),
            &startup.StartupInfo,
            &mut info,
        )
    };
    job_api::checked(status)?;
    Ok(Handles {
        process: info.hProcess,
        thread: info.hThread,
    })
}

pub(super) fn member(process: HANDLE, job: HANDLE) -> Result<bool, JobError> {
    let mut member = 0;
    // SAFETY: private live process and exact retained Job; writable BOOL observation.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
    let status = unsafe { IsProcessInJob(process, job, &mut member) };
    job_api::checked(status)?;
    Ok(member != 0)
}

pub(super) fn resume(thread: HANDLE) -> Result<(), JobError> {
    // SAFETY: exclusively retained primary thread, created suspended; one latched attempt.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
    let previous = unsafe { ResumeThread(thread) };
    verified_resume(previous)
}

pub(super) fn verified_resume(previous: u32) -> Result<(), JobError> {
    if previous == 1 {
        Ok(())
    } else {
        Err(JobError::Unavailable)
    }
}

pub(super) fn wait(process: HANDLE, millis: u32) -> Result<u32, JobError> {
    // SAFETY: private retained process reference; bounded wait, no PID lookup or infinite wait.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
    let status = unsafe { WaitForSingleObject(process, millis) };
    if status != WAIT_OBJECT_0 {
        return Err(JobError::Unavailable);
    }
    let mut code = 0;
    // SAFETY: same signalled live reference and writable DWORD result.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
    let status = unsafe { GetExitCodeProcess(process, &mut code) };
    job_api::checked(status)?;
    Ok(code)
}

pub(super) fn terminate(process: HANDLE) -> Result<(), JobError> {
    // SAFETY: exclusive control reference to our fixed fixture; no externally selected process.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
    let status = unsafe { TerminateProcess(process, 99) };
    job_api::checked(status)
}
