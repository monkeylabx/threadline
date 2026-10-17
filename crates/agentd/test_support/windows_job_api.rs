use super::{JobError, JobProfile};
use std::{
    ffi::c_void,
    mem::size_of,
    ptr::{from_mut, from_ref, null, null_mut, NonNull},
};
use windows_sys::Win32::{
    Foundation::{CloseHandle, GetHandleInformation, HANDLE, HANDLE_FLAG_INHERIT},
    System::JobObjects::{
        CreateJobObjectW, JobObjectExtendedLimitInformation, QueryInformationJobObject,
        SetInformationJobObject, JOBOBJECT_BASIC_LIMIT_INFORMATION,
        JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JOB_OBJECT_LIMIT_ACTIVE_PROCESS,
        JOB_OBJECT_LIMIT_BREAKAWAY_OK, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
        JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK,
    },
};

pub(super) fn checked(status: i32) -> Result<(), JobError> {
    if status == 0 {
        Err(JobError::Unavailable)
    } else {
        Ok(())
    }
}

pub(super) fn created(handle: HANDLE) -> Result<NonNull<c_void>, JobError> {
    NonNull::new(handle).ok_or(JobError::Unavailable)
}

pub(super) fn create() -> Result<NonNull<c_void>, JobError> {
    // SAFETY: NULL attributes/name request a fresh non-inheritable, unnamed job.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #245")]
    let handle = unsafe { CreateJobObjectW(null(), null()) };
    created(handle)
}

pub(super) fn configure(handle: HANDLE) -> Result<(), JobError> {
    let limits = JOBOBJECT_EXTENDED_LIMIT_INFORMATION {
        BasicLimitInformation: JOBOBJECT_BASIC_LIMIT_INFORMATION {
            LimitFlags: JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | JOB_OBJECT_LIMIT_ACTIVE_PROCESS,
            ActiveProcessLimit: 8,
            ..Default::default()
        },
        ..Default::default()
    };
    // SAFETY: private live job; initialized matching structure stays borrowed for this call.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #245")]
    let status = unsafe {
        SetInformationJobObject(
            handle,
            JobObjectExtendedLimitInformation,
            from_ref(&limits).cast(),
            size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
        )
    };
    checked(status)
}

pub(super) fn inspect(handle: HANDLE) -> Result<JobProfile, JobError> {
    let mut limits = JOBOBJECT_EXTENDED_LIMIT_INFORMATION::default();
    // SAFETY: private live job and uniquely borrowed output with the matching size/class.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #245")]
    let status = unsafe {
        QueryInformationJobObject(
            handle,
            JobObjectExtendedLimitInformation,
            from_mut(&mut limits).cast(),
            size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
            null_mut(),
        )
    };
    checked(status)?;
    let mut flags = 0;
    // SAFETY: same owned job and writable DWORD output valid for the synchronous call.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #245")]
    let status = unsafe { GetHandleInformation(handle, &mut flags) };
    checked(status)?;
    let bits = limits.BasicLimitInformation.LimitFlags;
    Ok(JobProfile {
        process_limit: limits.BasicLimitInformation.ActiveProcessLimit,
        limits_processes: bits & JOB_OBJECT_LIMIT_ACTIVE_PROCESS != 0,
        kills_on_close: bits & JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0,
        allows_breakaway: bits
            & (JOB_OBJECT_LIMIT_BREAKAWAY_OK | JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK)
            != 0,
        inheritable: flags & HANDLE_FLAG_INHERIT != 0,
    })
}

pub(super) fn close(handle: HANDLE) -> Result<(), JobError> {
    // SAFETY: exclusive fresh-job reference, not externally closable; owner clears only on success.
    #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #245")]
    let status = unsafe { CloseHandle(handle) };
    checked(status)
}
