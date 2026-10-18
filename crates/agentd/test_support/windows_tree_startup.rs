use super::super::Child;
use super::{api, pipes, process, reader, Job, JobError};
use api::Reference;
use std::io::{PipeWriter, Write};
// Field order is intentional: root termination/wait closes its reply writer before reader teardown.
struct Startup<'job> {
    root: Child<'job>,
    reader: reader::Reader,
    ack: PipeWriter,
}
impl<'job> Startup<'job> {
    fn new(job: &'job Job) -> Result<Self, JobError> {
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
        })
    }

    fn connect(&mut self) -> Result<Reference, JobError> {
        self.root.resume()?;
        let value = self.reader.receive()?;
        let copy = Reference(api::duplicate(self.root.handles.process, value)?);
        let job = self.root.job.handle.ok_or(JobError::Rejected)?.as_ptr();
        if !process::member(copy.0, job)? {
            return Err(JobError::Rejected);
        }
        self.ack
            .write_all(&[1])
            .map_err(|_| JobError::Unavailable)?;
        if self.root.wait(10_000)? != 42 {
            return Err(JobError::Unavailable);
        }
        Ok(copy)
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
