use super::super::{JobError, CHILD};
use std::{ffi::OsString, os::windows::ffi::OsStrExt};

pub(super) struct Input {
    pub(super) application: Vec<u16>,
    pub(super) command: Vec<u16>,
    pub(super) environment: Vec<u16>,
}

impl Input {
    pub(super) fn single() -> Result<Self, JobError> {
        let image = std::env::current_exe().map_err(|_| JobError::Unavailable)?;
        let application: Vec<u16> = image.as_os_str().encode_wide().chain([0]).collect();
        let mut arguments = OsString::from("\"");
        arguments.push(image);
        arguments.push(format!("\" --exact {CHILD} --quiet --test-threads=1"));
        let command: Vec<u16> = arguments.encode_wide().chain([0]).collect();
        let mut environment = OsString::from("SystemRoot=");
        environment.push(std::env::var_os("SystemRoot").ok_or(JobError::Unavailable)?);
        let environment: Vec<u16> = environment.encode_wide().chain([0, 0]).collect();
        Ok(Self {
            application,
            command,
            environment,
        })
    }
}
