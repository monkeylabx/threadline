use std::collections::VecDeque;
use std::panic::{catch_unwind, AssertUnwindSafe};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Condvar, Mutex, MutexGuard, OnceLock, Weak};
use std::thread;
use std::time::{Duration, Instant};

type Handle = u64;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(i32)]
pub(crate) enum Status {
    Ok = 0,
    InvalidArgument = 1,
    InvalidHandle = 2,
    Closed = 3,
    Canceled = 4,
    Panic = 5,
    AlreadyCommitted = 6,
    TimedOut = 7,
    #[allow(
        dead_code,
        reason = "reserved for push-style adapters that surface producer backpressure"
    )]
    Backpressure = 8,
    ProtocolViolation = 9,
    EndOfStream = 10,
    Unknown = 255,
}

impl Status {
    pub(crate) const fn code(self) -> i32 {
        self as i32
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(i32)]
pub(crate) enum Fault {
    None = 0,
    Delayed = 1,
    Panic = 2,
    UnknownError = 3,
    DuplicateEvent = 4,
    LateEvent = 5,
}

impl Fault {
    pub(crate) const fn from_code(code: i32) -> Option<Self> {
        match code {
            0 => Some(Self::None),
            1 => Some(Self::Delayed),
            2 => Some(Self::Panic),
            3 => Some(Self::UnknownError),
            4 => Some(Self::DuplicateEvent),
            5 => Some(Self::LateEvent),
            _ => None,
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(i32)]
pub(crate) enum ResourceKind {
    Client = 1,
    Request = 2,
    Stream = 3,
    Buffer = 4,
}

impl ResourceKind {
    pub(crate) const fn from_code(code: i32) -> Option<Self> {
        match code {
            1 => Some(Self::Client),
            2 => Some(Self::Request),
            3 => Some(Self::Stream),
            4 => Some(Self::Buffer),
            _ => None,
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
#[repr(i32)]
pub(crate) enum RequestPhase {
    Pending = 11,
    Committed = 12,
    Succeeded = 13,
    Canceled = Status::Canceled as i32,
    Panicked = Status::Panic as i32,
    UnknownError = Status::Unknown as i32,
    Closed = Status::Closed as i32,
}

impl RequestPhase {
    const fn code(self) -> i32 {
        self as i32
    }

    const fn is_terminal(self) -> bool {
        matches!(
            self,
            Self::Succeeded | Self::Canceled | Self::Panicked | Self::UnknownError | Self::Closed
        )
    }
}

pub(crate) struct RequestOutcome {
    pub(crate) status: Status,
    pub(crate) committed: bool,
}

#[derive(Debug, Eq, PartialEq)]
pub(crate) struct StreamEvent {
    pub(crate) sequence: u64,
}

pub(crate) struct StreamMetrics {
    pub(crate) capacity: u32,
    pub(crate) max_depth: u32,
    pub(crate) backpressure_count: u64,
    pub(crate) suppressed_late_events: u64,
}

trait Closable: Send + Sync {
    fn close(&self);
}

struct Client {
    closed: AtomicBool,
    children: Mutex<Vec<Weak<dyn Closable>>>,
}

impl Client {
    fn new() -> Self {
        Self {
            closed: AtomicBool::new(false),
            children: Mutex::new(Vec::new()),
        }
    }

    fn is_closed(&self) -> bool {
        self.closed.load(Ordering::Acquire)
    }

    fn add_child<T>(&self, child: &Arc<T>) -> Result<(), Status>
    where
        T: Closable + 'static,
    {
        if self.is_closed() {
            return Err(Status::Closed);
        }
        let child: Arc<dyn Closable> = child.clone();
        let mut children = lock(&self.children);
        children.retain(|registered| registered.strong_count() > 0);
        children.push(Arc::downgrade(&child));
        drop(children);
        if self.is_closed() {
            child.close();
            return Err(Status::Closed);
        }
        Ok(())
    }
}

impl Closable for Client {
    fn close(&self) {
        if self.closed.swap(true, Ordering::AcqRel) {
            return;
        }
        let children = lock(&self.children).clone();
        for child in children {
            if let Some(child) = child.upgrade() {
                child.close();
            }
        }
    }
}

struct RequestData {
    phase: RequestPhase,
    result: Option<Vec<u8>>,
}

struct Request {
    data: Mutex<RequestData>,
    changed: Condvar,
}

impl Request {
    fn new() -> Self {
        Self {
            data: Mutex::new(RequestData {
                phase: RequestPhase::Pending,
                result: None,
            }),
            changed: Condvar::new(),
        }
    }

    fn spawn(self: &Arc<Self>, fault: Fault, delay_ms: u64) {
        let request = Arc::clone(self);
        thread::spawn(move || {
            let outcome = catch_unwind(AssertUnwindSafe(|| request.run(fault, delay_ms)));
            if outcome.is_err() {
                let mut data = lock(&request.data);
                if !data.phase.is_terminal() {
                    data.phase = RequestPhase::Panicked;
                    data.result = None;
                    request.changed.notify_all();
                }
            }
        });
    }

    fn run(&self, fault: Fault, delay_ms: u64) {
        let delay = Duration::from_millis(delay_ms);
        if fault == Fault::Delayed && !self.wait_in_phase(RequestPhase::Pending, delay) {
            return;
        }

        {
            let mut data = lock(&self.data);
            if data.phase.is_terminal() {
                return;
            }
            match fault {
                Fault::Panic => panic!("injected native bridge fault"),
                Fault::UnknownError => {
                    data.phase = RequestPhase::UnknownError;
                    self.changed.notify_all();
                    return;
                }
                _ => {
                    data.phase = RequestPhase::Committed;
                    self.changed.notify_all();
                }
            }
        }

        if delay_ms > 0 && !self.wait_in_phase(RequestPhase::Committed, delay) {
            return;
        }

        let mut data = lock(&self.data);
        if data.phase == RequestPhase::Committed {
            data.phase = RequestPhase::Succeeded;
            data.result = Some(b"threadline-ok".to_vec());
            self.changed.notify_all();
        }
    }

    fn cancel(&self) -> Status {
        let mut data = lock(&self.data);
        match data.phase {
            RequestPhase::Pending => {
                data.phase = RequestPhase::Canceled;
                data.result = None;
                self.changed.notify_all();
                Status::Ok
            }
            RequestPhase::Committed => Status::AlreadyCommitted,
            _ => Status::Ok,
        }
    }

    fn wait_in_phase(&self, phase: RequestPhase, duration: Duration) -> bool {
        let deadline = Instant::now() + duration;
        let mut data = lock(&self.data);
        loop {
            if data.phase != phase {
                return false;
            }
            let now = Instant::now();
            if now >= deadline {
                return true;
            }
            let remaining = deadline.saturating_duration_since(now);
            let (next, _) = wait_timeout(&self.changed, data, remaining);
            data = next;
        }
    }

    fn phase(&self) -> RequestPhase {
        lock(&self.data).phase
    }

    fn committed(&self) -> bool {
        matches!(
            self.phase(),
            RequestPhase::Committed | RequestPhase::Succeeded
        )
    }

    fn wait(&self, timeout_ms: u64) -> RequestOutcome {
        let deadline = Instant::now() + Duration::from_millis(timeout_ms);
        let mut data = lock(&self.data);
        loop {
            let outcome = match data.phase {
                RequestPhase::Succeeded => Some(RequestOutcome {
                    status: Status::Ok,
                    committed: true,
                }),
                RequestPhase::Canceled => Some(RequestOutcome {
                    status: Status::Canceled,
                    committed: false,
                }),
                RequestPhase::Panicked => Some(RequestOutcome {
                    status: Status::Panic,
                    committed: false,
                }),
                RequestPhase::UnknownError => Some(RequestOutcome {
                    status: Status::Unknown,
                    committed: false,
                }),
                RequestPhase::Closed => Some(RequestOutcome {
                    status: Status::Closed,
                    committed: false,
                }),
                RequestPhase::Pending | RequestPhase::Committed => None,
            };
            if let Some(outcome) = outcome {
                return outcome;
            }

            let now = Instant::now();
            if now >= deadline {
                return RequestOutcome {
                    status: Status::TimedOut,
                    committed: data.phase == RequestPhase::Committed,
                };
            }
            let remaining = deadline.saturating_duration_since(now);
            let (next, timed_out) = wait_timeout(&self.changed, data, remaining);
            data = next;
            if timed_out && !data.phase.is_terminal() {
                return RequestOutcome {
                    status: Status::TimedOut,
                    committed: data.phase == RequestPhase::Committed,
                };
            }
        }
    }

    fn result(&self) -> Result<Vec<u8>, Status> {
        let data = lock(&self.data);
        match data.phase {
            RequestPhase::Succeeded => data.result.clone().ok_or(Status::Unknown),
            RequestPhase::Closed => Err(Status::Closed),
            RequestPhase::Canceled => Err(Status::Canceled),
            RequestPhase::Panicked => Err(Status::Panic),
            RequestPhase::UnknownError => Err(Status::Unknown),
            RequestPhase::Pending | RequestPhase::Committed => Err(Status::TimedOut),
        }
    }
}

impl Closable for Request {
    fn close(&self) {
        let mut data = lock(&self.data);
        if data.phase != RequestPhase::Closed {
            data.phase = RequestPhase::Closed;
            data.result = None;
            self.changed.notify_all();
        }
    }
}

struct StreamData {
    queue: VecDeque<StreamEvent>,
    terminal: Option<Status>,
    last_generated: u64,
    max_depth: u32,
    backpressure_count: u64,
    suppressed_late_events: u64,
}

struct Stream {
    capacity: u32,
    data: Mutex<StreamData>,
    available: Condvar,
    space: Condvar,
}

impl Stream {
    fn new(cursor: u64, capacity: u32) -> Self {
        Self {
            capacity,
            data: Mutex::new(StreamData {
                queue: VecDeque::with_capacity(capacity as usize),
                terminal: None,
                last_generated: cursor,
                max_depth: 0,
                backpressure_count: 0,
                suppressed_late_events: 0,
            }),
            available: Condvar::new(),
            space: Condvar::new(),
        }
    }

    fn spawn(self: &Arc<Self>, cursor: u64, event_count: u32, delay_ms: u64, fault: Fault) {
        let stream = Arc::clone(self);
        thread::spawn(move || {
            let outcome = catch_unwind(AssertUnwindSafe(|| {
                stream.run(cursor, event_count, delay_ms, fault)
            }));
            if outcome.is_err() {
                let mut data = lock(&stream.data);
                if data.terminal.is_none() {
                    data.terminal = Some(Status::Panic);
                    stream.available.notify_all();
                    stream.space.notify_all();
                }
            }
        });
    }

    fn run(&self, cursor: u64, event_count: u32, delay_ms: u64, fault: Fault) {
        if fault == Fault::Panic {
            panic!("injected native stream fault");
        }
        if fault == Fault::UnknownError {
            let mut data = lock(&self.data);
            data.terminal = Some(Status::Unknown);
            self.available.notify_all();
            return;
        }

        for offset in 1..=event_count {
            let expected = match cursor.checked_add(u64::from(offset)) {
                Some(value) => value,
                None => {
                    self.finish(Status::ProtocolViolation);
                    return;
                }
            };
            let candidate = if fault == Fault::DuplicateEvent && offset == 3 {
                expected.saturating_sub(1)
            } else {
                expected
            };

            let mut data = lock(&self.data);
            while data.queue.len() >= self.capacity as usize && data.terminal.is_none() {
                data.backpressure_count = data.backpressure_count.saturating_add(1);
                data = wait(&self.space, data);
            }
            if data.terminal.is_some() {
                return;
            }
            if candidate <= data.last_generated {
                data.terminal = Some(Status::ProtocolViolation);
                self.available.notify_all();
                self.space.notify_all();
                return;
            }
            data.last_generated = candidate;
            data.queue.push_back(StreamEvent {
                sequence: candidate,
            });
            data.max_depth = data.max_depth.max(data.queue.len() as u32);
            self.available.notify_all();
            drop(data);

            if delay_ms > 0 && !self.wait_while_open(Duration::from_millis(delay_ms)) {
                if fault == Fault::LateEvent {
                    self.record_suppressed_late_event();
                }
                return;
            }
        }

        if fault == Fault::LateEvent {
            let deadline = Instant::now() + Duration::from_millis(delay_ms.max(100));
            let mut data = lock(&self.data);
            while data.terminal.is_none() && Instant::now() < deadline {
                let remaining = deadline.saturating_duration_since(Instant::now());
                let (next, _) = wait_timeout(&self.available, data, remaining);
                data = next;
            }
            if data.terminal.is_some() {
                data.suppressed_late_events = data.suppressed_late_events.saturating_add(1);
            } else {
                data.terminal = Some(Status::ProtocolViolation);
            }
            self.available.notify_all();
            self.space.notify_all();
            return;
        }

        self.finish(Status::EndOfStream);
    }

    fn finish(&self, status: Status) {
        let mut data = lock(&self.data);
        if data.terminal.is_none() {
            data.terminal = Some(status);
            self.available.notify_all();
            self.space.notify_all();
        }
    }

    fn wait_while_open(&self, duration: Duration) -> bool {
        let deadline = Instant::now() + duration;
        let mut data = lock(&self.data);
        loop {
            if data.terminal.is_some() {
                return false;
            }
            let now = Instant::now();
            if now >= deadline {
                return true;
            }
            let remaining = deadline.saturating_duration_since(now);
            let (next, _) = wait_timeout(&self.space, data, remaining);
            data = next;
        }
    }

    fn record_suppressed_late_event(&self) {
        let mut data = lock(&self.data);
        if data.terminal.is_some() {
            data.suppressed_late_events = data.suppressed_late_events.saturating_add(1);
        }
        self.available.notify_all();
        self.space.notify_all();
    }

    fn next(&self, timeout_ms: u64) -> Result<StreamEvent, Status> {
        let deadline = Instant::now() + Duration::from_millis(timeout_ms);
        let mut data = lock(&self.data);
        loop {
            if let Some(event) = data.queue.pop_front() {
                self.space.notify_all();
                return Ok(event);
            }
            if let Some(status) = data.terminal {
                return Err(status);
            }

            let now = Instant::now();
            if now >= deadline {
                return Err(Status::TimedOut);
            }
            let remaining = deadline.saturating_duration_since(now);
            let (next, timed_out) = wait_timeout(&self.available, data, remaining);
            data = next;
            if timed_out && data.queue.is_empty() && data.terminal.is_none() {
                return Err(Status::TimedOut);
            }
        }
    }

    fn metrics(&self) -> StreamMetrics {
        let data = lock(&self.data);
        StreamMetrics {
            capacity: self.capacity,
            max_depth: data.max_depth,
            backpressure_count: data.backpressure_count,
            suppressed_late_events: data.suppressed_late_events,
        }
    }

    fn stop(&self, status: Status) {
        let mut data = lock(&self.data);
        if data.terminal != Some(Status::Closed) {
            data.queue.clear();
            data.terminal = Some(status);
            self.available.notify_all();
            self.space.notify_all();
        }
    }
}

impl Closable for Stream {
    fn close(&self) {
        self.stop(Status::Closed);
    }
}

struct Buffer {
    bytes: Box<[u8]>,
}

#[derive(Clone)]
enum Resource {
    Client(Arc<Client>),
    Request(Arc<Request>),
    Stream(Arc<Stream>),
    Buffer(Arc<Buffer>),
}

impl Resource {
    fn kind(&self) -> ResourceKind {
        match self {
            Self::Client(_) => ResourceKind::Client,
            Self::Request(_) => ResourceKind::Request,
            Self::Stream(_) => ResourceKind::Stream,
            Self::Buffer(_) => ResourceKind::Buffer,
        }
    }
}

struct Slot {
    generation: u32,
    resource: Option<Resource>,
}

struct Registry {
    slots: Vec<Slot>,
    free: Vec<usize>,
}

impl Registry {
    fn new() -> Self {
        Self {
            slots: Vec::new(),
            free: Vec::new(),
        }
    }

    fn insert(&mut self, resource: Resource) -> Handle {
        if let Some(index) = self.free.pop() {
            let slot = &mut self.slots[index];
            debug_assert!(slot.resource.is_none());
            slot.resource = Some(resource);
            encode(index, slot.generation)
        } else {
            let index = self.slots.len();
            self.slots.push(Slot {
                generation: 1,
                resource: Some(resource),
            });
            encode(index, 1)
        }
    }

    fn get(&self, handle: Handle) -> Result<Resource, Status> {
        let (index, generation) = decode(handle).ok_or(Status::InvalidHandle)?;
        let slot = self.slots.get(index).ok_or(Status::InvalidHandle)?;
        if slot.generation != generation {
            return Err(Status::InvalidHandle);
        }
        slot.resource.clone().ok_or(Status::InvalidHandle)
    }

    fn release(&mut self, handle: Handle, kind: ResourceKind) -> Status {
        let (index, generation) = match decode(handle) {
            Some(decoded) => decoded,
            None => return Status::InvalidHandle,
        };
        let Some(slot) = self.slots.get_mut(index) else {
            return Status::InvalidHandle;
        };
        if slot.generation != generation {
            return if generation < slot.generation {
                Status::Ok
            } else {
                Status::InvalidHandle
            };
        }
        match slot.resource.as_ref() {
            Some(resource) if resource.kind() == kind => {}
            _ => return Status::InvalidHandle,
        }
        slot.resource = None;
        slot.generation = slot.generation.wrapping_add(1).max(1);
        self.free.push(index);
        Status::Ok
    }

    fn count(&self, kind: ResourceKind) -> u64 {
        self.slots
            .iter()
            .filter_map(|slot| slot.resource.as_ref())
            .filter(|resource| resource.kind() == kind)
            .count() as u64
    }
}

static REGISTRY: OnceLock<Mutex<Registry>> = OnceLock::new();

fn registry() -> &'static Mutex<Registry> {
    REGISTRY.get_or_init(|| Mutex::new(Registry::new()))
}

fn encode(index: usize, generation: u32) -> Handle {
    ((u64::from(generation)) << 32) | ((index as u64) + 1)
}

fn decode(handle: Handle) -> Option<(usize, u32)> {
    let encoded_index = (handle & 0xffff_ffff) as u32;
    let generation = (handle >> 32) as u32;
    if encoded_index == 0 || generation == 0 {
        return None;
    }
    Some(((encoded_index - 1) as usize, generation))
}

fn get_client(handle: Handle) -> Result<Arc<Client>, Status> {
    match lock(registry()).get(handle)? {
        Resource::Client(client) => Ok(client),
        _ => Err(Status::InvalidHandle),
    }
}

fn get_request(handle: Handle) -> Result<Arc<Request>, Status> {
    match lock(registry()).get(handle)? {
        Resource::Request(request) => Ok(request),
        _ => Err(Status::InvalidHandle),
    }
}

fn get_stream(handle: Handle) -> Result<Arc<Stream>, Status> {
    match lock(registry()).get(handle)? {
        Resource::Stream(stream) => Ok(stream),
        _ => Err(Status::InvalidHandle),
    }
}

fn get_buffer(handle: Handle) -> Result<Arc<Buffer>, Status> {
    match lock(registry()).get(handle)? {
        Resource::Buffer(buffer) => Ok(buffer),
        _ => Err(Status::InvalidHandle),
    }
}

pub(crate) fn client_create() -> Handle {
    lock(registry()).insert(Resource::Client(Arc::new(Client::new())))
}

pub(crate) fn client_close(handle: Handle) -> Status {
    match get_client(handle) {
        Ok(client) => {
            client.close();
            Status::Ok
        }
        Err(status) => status,
    }
}

pub(crate) fn client_release(handle: Handle) -> Status {
    if let Ok(client) = get_client(handle) {
        client.close();
    }
    lock(registry()).release(handle, ResourceKind::Client)
}

pub(crate) fn request_start(
    client_handle: Handle,
    fault: Fault,
    delay_ms: u64,
) -> Result<Handle, Status> {
    let client = get_client(client_handle)?;
    if client.is_closed() {
        return Err(Status::Closed);
    }
    let request = Arc::new(Request::new());
    client.add_child(&request)?;
    let handle = lock(registry()).insert(Resource::Request(Arc::clone(&request)));
    request.spawn(fault, delay_ms);
    Ok(handle)
}

pub(crate) fn request_cancel(handle: Handle) -> Status {
    get_request(handle)
        .map(|request| request.cancel())
        .unwrap_or_else(|status| status)
}

pub(crate) fn request_state(handle: Handle) -> Result<i32, Status> {
    Ok(get_request(handle)?.phase().code())
}

pub(crate) fn request_committed(handle: Handle) -> Result<bool, Status> {
    Ok(get_request(handle)?.committed())
}

pub(crate) fn request_wait(handle: Handle, timeout_ms: u64) -> Result<RequestOutcome, Status> {
    Ok(get_request(handle)?.wait(timeout_ms))
}

pub(crate) fn request_result_buffer(handle: Handle) -> Result<Handle, Status> {
    let bytes = request_result_bytes(handle)?;
    Ok(lock(registry()).insert(Resource::Buffer(Arc::new(Buffer {
        bytes: bytes.into_boxed_slice(),
    }))))
}

pub(crate) fn request_result_bytes(handle: Handle) -> Result<Vec<u8>, Status> {
    get_request(handle)?.result()
}

pub(crate) fn request_close(handle: Handle) -> Status {
    match get_request(handle) {
        Ok(request) => {
            request.close();
            Status::Ok
        }
        Err(status) => status,
    }
}

pub(crate) fn request_release(handle: Handle) -> Status {
    if let Ok(request) = get_request(handle) {
        request.close();
    }
    lock(registry()).release(handle, ResourceKind::Request)
}

pub(crate) fn buffer_view(handle: Handle) -> Result<(*const u8, usize), Status> {
    let buffer = get_buffer(handle)?;
    Ok((buffer.bytes.as_ptr(), buffer.bytes.len()))
}

pub(crate) fn buffer_release(handle: Handle) -> Status {
    lock(registry()).release(handle, ResourceKind::Buffer)
}

pub(crate) fn stream_start(
    client_handle: Handle,
    cursor: u64,
    event_count: u32,
    capacity: u32,
    delay_ms: u64,
    fault: Fault,
) -> Result<Handle, Status> {
    if capacity == 0 {
        return Err(Status::InvalidArgument);
    }
    let client = get_client(client_handle)?;
    if client.is_closed() {
        return Err(Status::Closed);
    }
    let stream = Arc::new(Stream::new(cursor, capacity));
    client.add_child(&stream)?;
    let handle = lock(registry()).insert(Resource::Stream(Arc::clone(&stream)));
    stream.spawn(cursor, event_count, delay_ms, fault);
    Ok(handle)
}

pub(crate) fn stream_next(handle: Handle, timeout_ms: u64) -> Result<StreamEvent, Status> {
    get_stream(handle)?.next(timeout_ms)
}

pub(crate) fn stream_metrics(handle: Handle) -> Result<StreamMetrics, Status> {
    Ok(get_stream(handle)?.metrics())
}

pub(crate) fn stream_cancel(handle: Handle) -> Status {
    match get_stream(handle) {
        Ok(stream) => {
            stream.stop(Status::Canceled);
            Status::Ok
        }
        Err(status) => status,
    }
}

pub(crate) fn stream_close(handle: Handle) -> Status {
    match get_stream(handle) {
        Ok(stream) => {
            stream.close();
            Status::Ok
        }
        Err(status) => status,
    }
}

pub(crate) fn stream_release(handle: Handle) -> Status {
    if let Ok(stream) = get_stream(handle) {
        stream.close();
    }
    lock(registry()).release(handle, ResourceKind::Stream)
}

pub(crate) fn resource_count(kind: ResourceKind) -> u64 {
    lock(registry()).count(kind)
}

fn lock<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn wait<'a, T>(condition: &Condvar, guard: MutexGuard<'a, T>) -> MutexGuard<'a, T> {
    condition
        .wait(guard)
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn wait_timeout<'a, T>(
    condition: &Condvar,
    guard: MutexGuard<'a, T>,
    duration: Duration,
) -> (MutexGuard<'a, T>, bool) {
    match condition.wait_timeout(guard, duration) {
        Ok((guard, result)) => (guard, result.timed_out()),
        Err(poisoned) => {
            let (guard, result) = poisoned.into_inner();
            (guard, result.timed_out())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    static TEST_LOCK: Mutex<()> = Mutex::new(());

    const FAULT_FIXTURE: &str = include_str!("../fixtures/fault-cases.tsv");
    const C_HEADER: &str = include_str!("../include/threadline_client_ffi.h");
    const SWIFT_HOST: &str =
        include_str!("../../../apps/ios/Sources/ThreadlineIOSHost/ThreadlineIOSHost.swift");
    const KOTLIN_HOST: &str = include_str!(
        "../../../apps/android/src/main/kotlin/com/threadline/android/ThreadlineAndroidSkeleton.kt"
    );

    fn wait_until_committed(handle: Handle) {
        let deadline = Instant::now() + Duration::from_secs(2);
        while Instant::now() < deadline {
            if request_committed(handle).unwrap_or(false) {
                return;
            }
            thread::sleep(Duration::from_millis(1));
        }
        panic!("request did not reach its commit point");
    }

    fn serial_test() -> MutexGuard<'static, ()> {
        lock(&TEST_LOCK)
    }

    #[test]
    fn hosts_and_header_cover_the_shared_fault_fixture() {
        let _serial = serial_test();
        let expected = [
            (
                "none",
                0,
                "THREADLINE_FAULT_NONE",
                "case none = 0",
                "NONE(0)",
            ),
            (
                "delayed",
                1,
                "THREADLINE_FAULT_DELAYED",
                "case delayed = 1",
                "DELAYED(1)",
            ),
            (
                "panic",
                2,
                "THREADLINE_FAULT_PANIC",
                "case panic = 2",
                "PANIC(2)",
            ),
            (
                "unknown_error",
                3,
                "THREADLINE_FAULT_UNKNOWN_ERROR",
                "case unknownError = 3",
                "UNKNOWN_ERROR(3)",
            ),
            (
                "duplicate_event",
                4,
                "THREADLINE_FAULT_DUPLICATE_EVENT",
                "case duplicateEvent = 4",
                "DUPLICATE_EVENT(4)",
            ),
            (
                "late_event",
                5,
                "THREADLINE_FAULT_LATE_EVENT",
                "case lateEvent = 5",
                "LATE_EVENT(5)",
            ),
        ];

        for (name, code, c_name, swift_case, kotlin_case) in expected {
            let fixture_prefix = format!("{name}\t{code}\t");
            assert!(FAULT_FIXTURE
                .lines()
                .any(|line| line.starts_with(&fixture_prefix)));
            assert!(C_HEADER.contains(&format!("{c_name} = {code}")));
            assert!(SWIFT_HOST.contains(swift_case));
            assert!(KOTLIN_HOST.contains(kotlin_case));
            assert_eq!(
                Fault::from_code(code),
                Some(match code {
                    0 => Fault::None,
                    1 => Fault::Delayed,
                    2 => Fault::Panic,
                    3 => Fault::UnknownError,
                    4 => Fault::DuplicateEvent,
                    5 => Fault::LateEvent,
                    _ => unreachable!(),
                })
            );
        }
    }

    #[test]
    fn generation_checked_handles_do_not_reopen_released_resources() {
        let _serial = serial_test();
        let first = client_create();
        assert_eq!(client_release(first), Status::Ok);
        assert_eq!(client_release(first), Status::Ok);

        let second = client_create();
        assert_ne!(first, second);
        assert_eq!(client_close(first), Status::InvalidHandle);
        assert_eq!(client_release(second), Status::Ok);
    }

    #[test]
    fn null_client_output_does_not_allocate_a_resource() {
        let _serial = serial_test();
        let baseline = resource_count(ResourceKind::Client);
        assert_eq!(
            crate::exports::threadline_client_create(core::ptr::null_mut()),
            Status::InvalidArgument.code()
        );
        assert_eq!(resource_count(ResourceKind::Client), baseline);
    }

    #[test]
    fn null_child_outputs_do_not_leak_native_resources() {
        let _serial = serial_test();
        let client = client_create();
        let baseline_requests = resource_count(ResourceKind::Request);
        let baseline_streams = resource_count(ResourceKind::Stream);
        let baseline_buffers = resource_count(ResourceKind::Buffer);

        assert_eq!(
            crate::exports::threadline_request_start(
                client,
                Fault::Delayed as i32,
                60_000,
                core::ptr::null_mut(),
            ),
            Status::InvalidArgument.code()
        );
        assert_eq!(resource_count(ResourceKind::Request), baseline_requests);

        assert_eq!(
            crate::exports::threadline_stream_start(
                client,
                0,
                1,
                1,
                60_000,
                Fault::None as i32,
                core::ptr::null_mut(),
            ),
            Status::InvalidArgument.code()
        );
        assert_eq!(resource_count(ResourceKind::Stream), baseline_streams);

        let request = request_start(client, Fault::None, 0).expect("request starts");
        assert_eq!(
            request_wait(request, 1_000).expect("request ends").status,
            Status::Ok
        );
        assert_eq!(
            crate::exports::threadline_request_result(request, core::ptr::null_mut()),
            Status::InvalidArgument.code()
        );
        assert_eq!(resource_count(ResourceKind::Buffer), baseline_buffers);
        assert_eq!(request_release(request), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn cancellation_has_a_deterministic_commit_point() {
        let _serial = serial_test();
        let client = client_create();

        let before = request_start(client, Fault::Delayed, 20).expect("request starts");
        assert_eq!(request_cancel(before), Status::Ok);
        let outcome = request_wait(before, 1_000).expect("request remains addressable");
        assert_eq!(outcome.status, Status::Canceled);
        assert!(!outcome.committed);
        assert_eq!(request_release(before), Status::Ok);

        let after = request_start(client, Fault::None, 20).expect("request starts");
        wait_until_committed(after);
        assert_eq!(request_cancel(after), Status::AlreadyCommitted);
        let outcome = request_wait(after, 1_000).expect("request remains addressable");
        assert_eq!(outcome.status, Status::Ok);
        assert!(outcome.committed);
        assert_eq!(request_release(after), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn panic_and_unknown_faults_are_stable_errors() {
        let _serial = serial_test();
        let client = client_create();
        for (fault, expected) in [
            (Fault::Panic, Status::Panic),
            (Fault::UnknownError, Status::Unknown),
        ] {
            let request = request_start(client, fault, 0).expect("request starts");
            assert_eq!(
                request_wait(request, 1_000)
                    .expect("request remains addressable")
                    .status,
                expected
            );
            assert_eq!(request_release(request), Status::Ok);
        }
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn request_buffers_have_explicit_exactly_once_release() {
        let _serial = serial_test();
        let client = client_create();
        let request = request_start(client, Fault::None, 0).expect("request starts");
        assert_eq!(
            request_wait(request, 1_000)
                .expect("request remains addressable")
                .status,
            Status::Ok
        );
        let buffer = request_result_buffer(request).expect("result buffer exists");
        let (pointer, length) = buffer_view(buffer).expect("buffer remains addressable");
        assert!(!pointer.is_null());
        assert_eq!(length, b"threadline-ok".len());
        assert_eq!(resource_count(ResourceKind::Buffer), 1);
        assert_eq!(buffer_release(buffer), Status::Ok);
        assert_eq!(buffer_release(buffer), Status::Ok);
        assert_eq!(resource_count(ResourceKind::Buffer), 0);
        assert_eq!(request_release(request), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn streams_are_bounded_monotonic_and_resume_after_a_cursor() {
        let _serial = serial_test();
        let client = client_create();
        let stream = stream_start(client, 40, 8, 2, 0, Fault::None).expect("stream starts");
        thread::sleep(Duration::from_millis(10));
        let early = stream_metrics(stream).expect("metrics available");
        assert_eq!(early.max_depth, 2);
        assert!(early.backpressure_count > 0);

        let mut sequences = Vec::new();
        loop {
            match stream_next(stream, 1_000) {
                Ok(event) => sequences.push(event.sequence),
                Err(Status::EndOfStream) => break,
                Err(status) => panic!("unexpected stream status: {status:?}"),
            }
        }
        assert_eq!(sequences, (41..=48).collect::<Vec<_>>());
        assert_eq!(stream_release(stream), Status::Ok);

        let resumed = stream_start(client, 48, 2, 1, 0, Fault::None).expect("stream resumes");
        assert_eq!(
            stream_next(resumed, 1_000).expect("first event").sequence,
            49
        );
        assert_eq!(
            stream_next(resumed, 1_000).expect("second event").sequence,
            50
        );
        assert_eq!(stream_next(resumed, 1_000), Err(Status::EndOfStream));
        assert_eq!(stream_release(resumed), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn duplicate_events_fail_before_delivery() {
        let _serial = serial_test();
        let client = client_create();
        let stream =
            stream_start(client, 0, 5, 5, 0, Fault::DuplicateEvent).expect("stream starts");
        assert_eq!(stream_next(stream, 1_000).expect("first").sequence, 1);
        assert_eq!(stream_next(stream, 1_000).expect("second").sequence, 2);
        assert_eq!(stream_next(stream, 1_000), Err(Status::ProtocolViolation));
        assert_eq!(stream_release(stream), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn a_late_event_attempt_after_close_is_suppressed() {
        let _serial = serial_test();
        let client = client_create();
        let stream = stream_start(client, 0, 1, 1, 20, Fault::LateEvent).expect("stream starts");
        assert_eq!(stream_next(stream, 1_000).expect("first").sequence, 1);
        assert_eq!(stream_close(stream), Status::Ok);
        thread::sleep(Duration::from_millis(50));
        let metrics = stream_metrics(stream).expect("metrics remain available until release");
        assert_eq!(metrics.suppressed_late_events, 1);
        assert_eq!(stream_next(stream, 1_000), Err(Status::Closed));
        assert_eq!(stream_release(stream), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn closing_a_client_closes_children_without_late_delivery() {
        let _serial = serial_test();
        let client = client_create();
        let request = request_start(client, Fault::Delayed, 25).expect("request starts");
        let stream = stream_start(client, 0, 10, 1, 25, Fault::LateEvent).expect("stream starts");
        assert_eq!(client_close(client), Status::Ok);
        assert_eq!(
            request_wait(request, 1_000)
                .expect("request remains addressable")
                .status,
            Status::Closed
        );
        assert_eq!(stream_next(stream, 1_000), Err(Status::Closed));
        assert_eq!(request_release(request), Status::Ok);
        assert_eq!(stream_release(stream), Status::Ok);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn close_interrupts_long_running_native_workers() {
        let _serial = serial_test();
        let client = client_create();

        let request = request_start(client, Fault::Delayed, 60_000).expect("request starts");
        let request_resource = get_request(request).expect("request remains addressable");
        assert_eq!(request_close(request), Status::Ok);
        assert_eq!(request_release(request), Status::Ok);

        let stream = stream_start(client, 0, 1, 1, 60_000, Fault::None).expect("stream starts");
        let stream_resource = get_stream(stream).expect("stream remains addressable");
        assert_eq!(stream_next(stream, 1_000).expect("first event").sequence, 1);
        assert_eq!(stream_close(stream), Status::Ok);
        assert_eq!(stream_release(stream), Status::Ok);

        let deadline = Instant::now() + Duration::from_secs(1);
        while (Arc::strong_count(&request_resource) > 1 || Arc::strong_count(&stream_resource) > 1)
            && Instant::now() < deadline
        {
            thread::yield_now();
        }
        assert_eq!(Arc::strong_count(&request_resource), 1);
        assert_eq!(Arc::strong_count(&stream_resource), 1);
        assert_eq!(client_release(client), Status::Ok);
    }

    #[test]
    fn one_thousand_lifecycle_loops_leave_no_registered_resources() {
        let _serial = serial_test();
        let baseline_clients = resource_count(ResourceKind::Client);
        let baseline_requests = resource_count(ResourceKind::Request);
        let baseline_streams = resource_count(ResourceKind::Stream);

        for _ in 0..1_000 {
            let client = client_create();
            let request = request_start(client, Fault::None, 0).expect("request starts");
            let _ = request_wait(request, 1_000);
            assert_eq!(request_close(request), Status::Ok);
            assert_eq!(request_release(request), Status::Ok);
            assert_eq!(request_release(request), Status::Ok);

            let stream = stream_start(client, 0, 0, 1, 0, Fault::None).expect("stream starts");
            assert_eq!(stream_next(stream, 1_000), Err(Status::EndOfStream));
            assert_eq!(stream_close(stream), Status::Ok);
            assert_eq!(stream_release(stream), Status::Ok);
            assert_eq!(stream_release(stream), Status::Ok);

            assert_eq!(client_close(client), Status::Ok);
            assert_eq!(client_release(client), Status::Ok);
            assert_eq!(client_release(client), Status::Ok);
        }

        assert_eq!(resource_count(ResourceKind::Client), baseline_clients);
        assert_eq!(resource_count(ResourceKind::Request), baseline_requests);
        assert_eq!(resource_count(ResourceKind::Stream), baseline_streams);
    }
}
