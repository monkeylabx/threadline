use super::super::Child;
use super::{api, pipes, process, reader, Job, JobError};
use api::Reference;
use std::io::{PipeWriter, Write};
// Field order is intentional: root termination/wait closes its reply writer before reader teardown.
pub(super) struct Startup<'job> {
    pub(super) root: Child<'job>,
    reader: reader::Reader,
    ack: PipeWriter,
    pub(super) descendant: Option<Reference>,
}
impl<'job> Startup<'job> {
    pub(super) fn new(job: &'job Job) -> Result<Self, JobError> {
        let (ends, reply, ack) = pipes::RootEnds::create()?;
        let root = Child::create_with(job, |attributes| {
            attributes.inherit(ends.handles())?;
            process::create_root(attributes, ends.handles())
        })?;
        drop(ends); // No parent copy of a root-side pipe may keep EOF pending.
        Ok(Self {
            root,
            reader: reader::Reader::start(reply),
            ack,
            descendant: None,
        })
    }

    fn connect(&mut self) -> Result<Reference, JobError> {
        self.connect_with(self.root.job, process::member, |ack| {
            ack.write_all(&[1]).map_err(|_| JobError::Unavailable)
        })
    }

    pub(super) fn connect_with(
        &mut self,
        expected: &Job,
        member: impl FnOnce(
            windows_sys::Win32::Foundation::HANDLE,
            windows_sys::Win32::Foundation::HANDLE,
        ) -> Result<bool, JobError>,
        acknowledge: impl FnOnce(&mut PipeWriter) -> Result<(), JobError>,
    ) -> Result<Reference, JobError> {
        self.root.resume()?;
        let value = self.reader.receive()?;
        self.descendant = Some(Reference(api::duplicate(self.root.handles.process, value)?));
        self.verify(expected, member)?;
        acknowledge(&mut self.ack)?;
        if self.root.wait(10_000)? != 42 {
            return Err(JobError::Unavailable);
        }
        self.descendant.take().ok_or(JobError::Rejected)
    }

    fn verify(
        &self,
        expected: &Job,
        member: impl FnOnce(
            windows_sys::Win32::Foundation::HANDLE,
            windows_sys::Win32::Foundation::HANDLE,
        ) -> Result<bool, JobError>,
    ) -> Result<(), JobError> {
        let process = self.descendant.as_ref().ok_or(JobError::Rejected)?.0;
        let job = expected.handle.ok_or(JobError::Rejected)?.as_ptr();
        if !member(process, job)? {
            return Err(JobError::Rejected);
        }
        Ok(())
    }
}

struct KillOnFailure<'job> {
    job: &'job Job,
    armed: bool,
}
impl Drop for KillOnFailure<'_> {
    fn drop(&mut self) {
        if self.armed {
            api::terminate(self.job.handle.expect("live borrowed job").as_ptr())
                .expect("synthetic failed-start termination unavailable");
        }
    }
}

pub(super) fn retain(job: &Job) -> Result<Reference, JobError> {
    job.handle.ok_or(JobError::Rejected)?;
    let mut guard = KillOnFailure { job, armed: true };
    let mut startup = Startup::new(job)?;
    let descendant = startup.connect()?;
    drop(startup); // Root references close before any final accounting observation.
    guard.armed = false;
    Ok(descendant)
}
