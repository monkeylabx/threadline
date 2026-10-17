use super::{job_api, Job, JobError};
use std::{mem::size_of, ptr::null_mut};
use windows_sys::Win32::{
    Foundation::{GetLastError, ERROR_INSUFFICIENT_BUFFER, HANDLE},
    System::Threading::{
        DeleteProcThreadAttributeList, InitializeProcThreadAttributeList,
        UpdateProcThreadAttribute, LPPROC_THREAD_ATTRIBUTE_LIST, PROC_THREAD_ATTRIBUTE_JOB_LIST,
    },
};

pub(super) struct Attributes<'job> {
    storage: Vec<u128>,
    jobs: Box<[HANDLE; 1]>,
    _owner: &'job Job,
}

impl<'job> Attributes<'job> {
    pub(super) fn new(job: &'job Job) -> Result<Self, JobError> {
        let handle = job.handle.ok_or(JobError::Rejected)?.as_ptr();
        let mut size = 0;
        // SAFETY: NULL list is the documented sizing probe; size is writable.
        #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
        let status = unsafe { InitializeProcThreadAttributeList(null_mut(), 1, 0, &mut size) };
        // SAFETY: same-thread error observation immediately after the sizing probe.
        #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
        let error = unsafe { GetLastError() };
        if status != 0 || error != ERROR_INSUFFICIENT_BUFFER || size == 0 || size > 4096 {
            return Err(JobError::Unavailable);
        }
        let mut storage = vec![0_u128; size.div_ceil(size_of::<u128>())];
        // SAFETY: 16-byte-aligned storage covers the exact reported byte count.
        #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
        let status = unsafe {
            InitializeProcThreadAttributeList(storage.as_mut_ptr().cast(), 1, 0, &mut size)
        };
        job_api::checked(status)?;
        let mut list = Self {
            storage,
            jobs: Box::new([handle]),
            _owner: job,
        };
        // SAFETY: initialized list; boxed handle array stays pinned until list destruction.
        // Its Job borrow keeps the non-inherited owner reference live through creation.
        #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
        let status = unsafe {
            UpdateProcThreadAttribute(
                list.pointer(),
                0,
                PROC_THREAD_ATTRIBUTE_JOB_LIST as usize,
                list.jobs.as_ptr().cast(),
                size_of::<HANDLE>(),
                null_mut(),
                std::ptr::null(),
            )
        };
        job_api::checked(status)?;
        Ok(list)
    }

    pub(super) fn pointer(&mut self) -> LPPROC_THREAD_ATTRIBUTE_LIST {
        self.storage.as_mut_ptr().cast()
    }
}

impl Drop for Attributes<'_> {
    fn drop(&mut self) {
        // SAFETY: successfully initialized list, live aligned storage and boxed value.
        // Delete has no failure result and runs before either allocation is released.
        #[expect(unsafe_code, reason = "Fixture-only Win32 FFI exception in #247")]
        unsafe {
            DeleteProcThreadAttributeList(self.pointer())
        };
    }
}
