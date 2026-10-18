use super::super::{JobError, CHILD};
use std::{
    ffi::OsString,
    os::windows::{
        ffi::OsStrExt,
        io::{AsRawHandle, BorrowedHandle},
    },
};

pub(super) struct Input<'io> {
    pub(super) application: Vec<u16>,
    pub(super) command: Vec<u16>,
    pub(super) environment: Vec<u16>,
    pub(super) stdio: Option<[BorrowedHandle<'io>; 3]>,
}

impl<'io> Input<'io> {
    pub(super) fn single() -> Result<Self, JobError> {
        Self::fixed(CHILD)
    }

    pub(super) fn root(stdio: [BorrowedHandle<'io>; 3]) -> Result<Self, JobError> {
        let mut input = Self::fixed(super::super::tree::ROOT)?;
        input.stdio = Some(stdio);
        Ok(input)
    }

    pub(super) fn raw_stdio(&self) -> [windows_sys::Win32::Foundation::HANDLE; 3] {
        self.stdio
            .map(|handles| handles.map(|handle| handle.as_raw_handle()))
            .unwrap_or([std::ptr::null_mut(); 3])
    }

    fn fixed(case: &'static str) -> Result<Self, JobError> {
        let image = std::env::current_exe().map_err(|_| JobError::Unavailable)?;
        let application: Vec<u16> = image.as_os_str().encode_wide().chain([0]).collect();
        let mut arguments = OsString::from("\"");
        arguments.push(image);
        arguments.push(format!("\" --exact {case} --quiet --test-threads=1"));
        let command: Vec<u16> = arguments.encode_wide().chain([0]).collect();
        let mut environment = OsString::from("SystemRoot=");
        environment.push(std::env::var_os("SystemRoot").ok_or(JobError::Unavailable)?);
        let environment: Vec<u16> = environment.encode_wide().chain([0, 0]).collect();
        Ok(Self {
            application,
            command,
            environment,
            stdio: None,
        })
    }
}
