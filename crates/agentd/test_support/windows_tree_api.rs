use super::{job_api, JobError};
use std::{
    mem::size_of,
    os::windows::io::{AsRawHandle, BorrowedHandle},
    ptr::{from_mut, null_mut},
};
use windows_sys::Win32::{
    Foundation::{DuplicateHandle, SetHandleInformation, HANDLE, HANDLE_FLAG_INHERIT},
    System::{
        JobObjects::{
            JobObjectBasicAccountingInformation, QueryInformationJobObject, TerminateJobObject,
            JOBOBJECT_BASIC_ACCOUNTING_INFORMATION,
        },
        Threading::{
            GetCurrentProcess, WaitForSingleObject, PROCESS_QUERY_LIMITED_INFORMATION,
            PROCESS_SYNCHRONIZE,
        },
    },
};

pub(super) fn inherit(handle: BorrowedHandle<'_>, enabled: bool) -> Result<(), JobError> {
    // SAFETY: a live trusted pipe/NUL stdio borrow; no external selector; only the inherit flag.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #249")]
    let status = unsafe {
        SetHandleInformation(
            handle.as_raw_handle(),
            HANDLE_FLAG_INHERIT,
            if enabled { HANDLE_FLAG_INHERIT } else { 0 },
        )
    };
    job_api::checked(status)
}

pub(super) fn duplicate(root: HANDLE, value: usize) -> Result<HANDLE, JobError> {
    let mut copy = null_mut();
    // SAFETY: exact owned root, fixed fixture packet, child reference retained until ack.
    // Copy into this process with wait/query rights only; no owner/job handle goes to root.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #249")]
    let status = unsafe {
        DuplicateHandle(
            root,
            value as HANDLE,
            GetCurrentProcess(),
            &mut copy,
            PROCESS_SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION,
            0,
            0,
        )
    };
    job_api::checked(status)?;
    Ok(copy)
}

pub(super) fn terminate(job: HANDLE) -> Result<(), JobError> {
    // SAFETY: exact fresh private fixture Job, never a named or caller-selected target.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #249")]
    let status = unsafe { TerminateJobObject(job, 99) };
    job_api::checked(status)
}

pub(super) fn active(job: HANDLE) -> Result<u32, JobError> {
    let mut info = JOBOBJECT_BASIC_ACCOUNTING_INFORMATION::default();
    // SAFETY: live private Job, initialized matching output class/size and unique borrow.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #249")]
    let status = unsafe {
        QueryInformationJobObject(
            job,
            JobObjectBasicAccountingInformation,
            from_mut(&mut info).cast(),
            size_of::<JOBOBJECT_BASIC_ACCOUNTING_INFORMATION>() as u32,
            null_mut(),
        )
    };
    job_api::checked(status)?;
    Ok(info.ActiveProcesses)
}

pub(super) fn park() {
    // SAFETY: current-process pseudo-handle is borrowed, never closed. Fixed bounded wait only.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #249")]
    let status = unsafe { WaitForSingleObject(GetCurrentProcess(), 10_000) };
    assert_eq!(status, windows_sys::Win32::Foundation::WAIT_TIMEOUT);
}

pub(super) struct Reference(pub(super) HANDLE);
impl Reference {
    pub(super) fn close(&mut self) -> Result<(), JobError> {
        self.close_with(job_api::close)
    }

    pub(super) fn close_with(
        &mut self,
        close: impl FnOnce(HANDLE) -> Result<(), JobError>,
    ) -> Result<(), JobError> {
        if !self.0.is_null() {
            close(self.0)?;
            self.0 = null_mut();
        }
        Ok(())
    }
}
impl Drop for Reference {
    fn drop(&mut self) {
        self.close()
            .expect("synthetic duplicate release unavailable");
    }
}
