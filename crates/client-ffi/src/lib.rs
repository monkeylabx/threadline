//! Versioned native bridge facade used by the M0 host harness.
//!
//! The facade deliberately contains no message, storage, or cryptographic
//! behavior. Hosts own their callback executors and pull terminal results or
//! stream events from this bounded interface, so Rust never retains a Swift or
//! Kotlin object.

#![deny(unsafe_code)]

mod runtime;

use std::panic::{catch_unwind, AssertUnwindSafe};

use runtime::{Fault, ResourceKind, Status};

const CONTRACT_VERSION: u32 = 1;

#[repr(C)]
#[derive(Clone, Copy, Debug)]
pub struct ThreadlineBuffer {
    pub data: *const u8,
    pub len: usize,
    pub handle: u64,
}

impl Default for ThreadlineBuffer {
    fn default() -> Self {
        Self {
            data: core::ptr::null(),
            len: 0,
            handle: 0,
        }
    }
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct ThreadlineStreamMetrics {
    pub capacity: u32,
    pub max_depth: u32,
    pub backpressure_count: u64,
    pub suppressed_late_events: u64,
}

fn status_guard(operation: impl FnOnce() -> Status) -> i32 {
    catch_unwind(AssertUnwindSafe(operation))
        .unwrap_or(Status::Panic)
        .code()
}

fn jni_handle_guard(operation: impl FnOnce() -> Result<u64, Status>) -> i64 {
    match catch_unwind(AssertUnwindSafe(operation)) {
        Ok(Ok(handle)) => {
            i64::try_from(handle).unwrap_or_else(|_| -(Status::ProtocolViolation.code() as i64))
        }
        Ok(Err(status)) => -(status.code() as i64),
        Err(_) => -(Status::Panic.code() as i64),
    }
}

fn value_guard(operation: impl FnOnce() -> i64) -> i64 {
    catch_unwind(AssertUnwindSafe(operation)).unwrap_or(-(Status::Panic.code() as i64))
}

#[allow(
    unsafe_code,
    reason = "the reviewed facade validates every pointer before writing FFI output"
)]
mod exports {
    use super::*;

    fn write_output<T>(output: *mut T, value: T) -> Result<(), Status> {
        if output.is_null() {
            return Err(Status::InvalidArgument);
        }

        unsafe { output.write(value) };
        Ok(())
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_client_ffi_contract_version() -> u32 {
        CONTRACT_VERSION
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_client_create(output: *mut u64) -> i32 {
        status_guard(|| {
            if output.is_null() {
                return Status::InvalidArgument;
            }
            match write_output(output, runtime::client_create()) {
                Ok(()) => Status::Ok,
                Err(status) => status,
            }
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_client_close(handle: u64) -> i32 {
        status_guard(|| runtime::client_close(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_client_release(handle: u64) -> i32 {
        status_guard(|| runtime::client_release(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_start(
        client_handle: u64,
        fault: i32,
        delay_ms: u64,
        output: *mut u64,
    ) -> i32 {
        status_guard(|| {
            let fault = match Fault::from_code(fault) {
                Some(fault) => fault,
                None => return Status::InvalidArgument,
            };
            let handle = match runtime::request_start(client_handle, fault, delay_ms) {
                Ok(handle) => handle,
                Err(status) => return status,
            };
            match write_output(output, handle) {
                Ok(()) => Status::Ok,
                Err(status) => {
                    let _ = runtime::request_release(handle);
                    status
                }
            }
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_cancel(handle: u64) -> i32 {
        status_guard(|| runtime::request_cancel(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_state(handle: u64) -> i32 {
        catch_unwind(AssertUnwindSafe(|| {
            runtime::request_state(handle).unwrap_or_else(Status::code)
        }))
        .unwrap_or(Status::Panic.code())
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_is_committed(handle: u64, committed: *mut u8) -> i32 {
        status_guard(|| match runtime::request_committed(handle) {
            Ok(value) => match write_output(committed, u8::from(value)) {
                Ok(()) => Status::Ok,
                Err(status) => status,
            },
            Err(status) => status,
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_wait(
        handle: u64,
        timeout_ms: u64,
        committed: *mut u8,
    ) -> i32 {
        status_guard(|| match runtime::request_wait(handle, timeout_ms) {
            Ok(outcome) => match write_output(committed, u8::from(outcome.committed)) {
                Ok(()) => outcome.status,
                Err(status) => status,
            },
            Err(status) => status,
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_result(handle: u64, output: *mut ThreadlineBuffer) -> i32 {
        status_guard(|| {
            let buffer_handle = match runtime::request_result_buffer(handle) {
                Ok(handle) => handle,
                Err(status) => return status,
            };
            let (data, len) = match runtime::buffer_view(buffer_handle) {
                Ok(view) => view,
                Err(status) => {
                    let _ = runtime::buffer_release(buffer_handle);
                    return status;
                }
            };
            let value = ThreadlineBuffer {
                data,
                len,
                handle: buffer_handle,
            };
            match write_output(output, value) {
                Ok(()) => Status::Ok,
                Err(status) => {
                    let _ = runtime::buffer_release(buffer_handle);
                    status
                }
            }
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_close(handle: u64) -> i32 {
        status_guard(|| runtime::request_close(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_request_release(handle: u64) -> i32 {
        status_guard(|| runtime::request_release(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_buffer_release(handle: u64) -> i32 {
        status_guard(|| runtime::buffer_release(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_stream_start(
        client_handle: u64,
        cursor: u64,
        event_count: u32,
        capacity: u32,
        delay_ms: u64,
        fault: i32,
        output: *mut u64,
    ) -> i32 {
        status_guard(|| {
            let fault = match Fault::from_code(fault) {
                Some(fault) => fault,
                None => return Status::InvalidArgument,
            };
            let handle = match runtime::stream_start(
                client_handle,
                cursor,
                event_count,
                capacity,
                delay_ms,
                fault,
            ) {
                Ok(handle) => handle,
                Err(status) => return status,
            };
            match write_output(output, handle) {
                Ok(()) => Status::Ok,
                Err(status) => {
                    let _ = runtime::stream_release(handle);
                    status
                }
            }
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_stream_next(
        handle: u64,
        timeout_ms: u64,
        sequence: *mut u64,
    ) -> i32 {
        status_guard(|| match runtime::stream_next(handle, timeout_ms) {
            Ok(event) => match write_output(sequence, event.sequence) {
                Ok(()) => Status::Ok,
                Err(status) => status,
            },
            Err(status) => status,
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_stream_get_metrics(
        handle: u64,
        output: *mut ThreadlineStreamMetrics,
    ) -> i32 {
        status_guard(|| match runtime::stream_metrics(handle) {
            Ok(metrics) => match write_output(
                output,
                ThreadlineStreamMetrics {
                    capacity: metrics.capacity,
                    max_depth: metrics.max_depth,
                    backpressure_count: metrics.backpressure_count,
                    suppressed_late_events: metrics.suppressed_late_events,
                },
            ) {
                Ok(()) => Status::Ok,
                Err(status) => status,
            },
            Err(status) => status,
        })
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_stream_cancel(handle: u64) -> i32 {
        status_guard(|| runtime::stream_cancel(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_stream_close(handle: u64) -> i32 {
        status_guard(|| runtime::stream_close(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_stream_release(handle: u64) -> i32 {
        status_guard(|| runtime::stream_release(handle))
    }

    #[unsafe(no_mangle)]
    pub extern "C" fn threadline_debug_resource_count(kind: i32) -> u64 {
        catch_unwind(AssertUnwindSafe(|| {
            ResourceKind::from_code(kind)
                .map(runtime::resource_count)
                .unwrap_or(0)
        }))
        .unwrap_or(0)
    }

    // JNI entry points intentionally exchange only primitive values. Kotlin
    // owns its Executor and callback fence; Rust never retains a JVM object.

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeBridgeContractVersion(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
    ) -> i32 {
        CONTRACT_VERSION as i32
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeClientCreate(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
    ) -> i64 {
        jni_handle_guard(|| Ok(runtime::client_create()))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeClientClose(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::client_close(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeClientRelease(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::client_release(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestStart(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        client_handle: i64,
        fault: i32,
        delay_ms: i64,
    ) -> i64 {
        jni_handle_guard(|| {
            let fault = Fault::from_code(fault).ok_or(Status::InvalidArgument)?;
            let delay_ms = u64::try_from(delay_ms).map_err(|_| Status::InvalidArgument)?;
            runtime::request_start(client_handle as u64, fault, delay_ms)
        })
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestCancel(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::request_cancel(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestState(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        catch_unwind(AssertUnwindSafe(|| {
            runtime::request_state(handle as u64).unwrap_or_else(Status::code)
        }))
        .unwrap_or(Status::Panic.code())
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestWait(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
        timeout_ms: i64,
    ) -> i32 {
        status_guard(|| {
            let timeout_ms = match u64::try_from(timeout_ms) {
                Ok(value) => value,
                Err(_) => return Status::InvalidArgument,
            };
            runtime::request_wait(handle as u64, timeout_ms)
                .map(|outcome| outcome.status)
                .unwrap_or_else(|status| status)
        })
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestCommitted(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> u8 {
        catch_unwind(AssertUnwindSafe(|| {
            runtime::request_committed(handle as u64).unwrap_or(false)
        }))
        .map(u8::from)
        .unwrap_or(0)
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestResultLength(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        catch_unwind(AssertUnwindSafe(|| {
            runtime::request_result_bytes(handle as u64)
                .and_then(|bytes| i32::try_from(bytes.len()).map_err(|_| Status::ProtocolViolation))
                .unwrap_or_else(|status| -status.code())
        }))
        .unwrap_or(-Status::Panic.code())
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestResultByte(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
        index: i32,
    ) -> i32 {
        catch_unwind(AssertUnwindSafe(|| {
            let index = match usize::try_from(index) {
                Ok(value) => value,
                Err(_) => return -Status::InvalidArgument.code(),
            };
            runtime::request_result_bytes(handle as u64)
                .and_then(|bytes| {
                    bytes
                        .get(index)
                        .copied()
                        .map(i32::from)
                        .ok_or(Status::InvalidArgument)
                })
                .unwrap_or_else(|status| -status.code())
        }))
        .unwrap_or(-Status::Panic.code())
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestClose(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::request_close(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeRequestRelease(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::request_release(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamStart(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        client_handle: i64,
        cursor: i64,
        event_count: i32,
        capacity: i32,
        delay_ms: i64,
        fault: i32,
    ) -> i64 {
        jni_handle_guard(|| {
            let fault = Fault::from_code(fault).ok_or(Status::InvalidArgument)?;
            let cursor = u64::try_from(cursor).map_err(|_| Status::InvalidArgument)?;
            let event_count = u32::try_from(event_count).map_err(|_| Status::InvalidArgument)?;
            let capacity = u32::try_from(capacity).map_err(|_| Status::InvalidArgument)?;
            let delay_ms = u64::try_from(delay_ms).map_err(|_| Status::InvalidArgument)?;
            runtime::stream_start(
                client_handle as u64,
                cursor,
                event_count,
                capacity,
                delay_ms,
                fault,
            )
        })
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamNext(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
        timeout_ms: i64,
    ) -> i64 {
        value_guard(|| {
            let timeout_ms = match u64::try_from(timeout_ms) {
                Ok(value) => value,
                Err(_) => return -(Status::InvalidArgument.code() as i64),
            };
            match runtime::stream_next(handle as u64, timeout_ms) {
                Ok(event) => i64::try_from(event.sequence)
                    .unwrap_or(-(Status::ProtocolViolation.code() as i64)),
                Err(Status::EndOfStream) => 0,
                Err(status) => -(status.code() as i64),
            }
        })
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamCapacity(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        catch_unwind(AssertUnwindSafe(|| {
            runtime::stream_metrics(handle as u64)
                .map(|metrics| metrics.capacity as i32)
                .unwrap_or_else(|status| -status.code())
        }))
        .unwrap_or(-Status::Panic.code())
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamMaxDepth(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        catch_unwind(AssertUnwindSafe(|| {
            runtime::stream_metrics(handle as u64)
                .map(|metrics| metrics.max_depth as i32)
                .unwrap_or_else(|status| -status.code())
        }))
        .unwrap_or(-Status::Panic.code())
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamBackpressureCount(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i64 {
        value_guard(|| {
            runtime::stream_metrics(handle as u64)
                .and_then(|metrics| {
                    i64::try_from(metrics.backpressure_count).map_err(|_| Status::ProtocolViolation)
                })
                .unwrap_or_else(|status| -(status.code() as i64))
        })
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamSuppressedLateEvents(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i64 {
        value_guard(|| {
            runtime::stream_metrics(handle as u64)
                .and_then(|metrics| {
                    i64::try_from(metrics.suppressed_late_events)
                        .map_err(|_| Status::ProtocolViolation)
                })
                .unwrap_or_else(|status| -(status.code() as i64))
        })
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamCancel(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::stream_cancel(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamClose(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::stream_close(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeStreamRelease(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        handle: i64,
    ) -> i32 {
        status_guard(|| runtime::stream_release(handle as u64))
    }

    #[unsafe(no_mangle)]
    pub extern "system" fn Java_com_threadline_android_ThreadlineNative_nativeDebugResourceCount(
        _environment: *mut core::ffi::c_void,
        _receiver: *mut core::ffi::c_void,
        kind: i32,
    ) -> i64 {
        ResourceKind::from_code(kind)
            .map(runtime::resource_count)
            .unwrap_or(0) as i64
    }
}

pub use exports::{
    threadline_client_create, threadline_client_release, threadline_stream_next,
    threadline_stream_release, threadline_stream_start,
};

#[cfg(test)]
mod tests {
    use super::exports::{
        threadline_client_ffi_contract_version,
        Java_com_threadline_android_ThreadlineNative_nativeClientCreate,
        Java_com_threadline_android_ThreadlineNative_nativeClientRelease,
        Java_com_threadline_android_ThreadlineNative_nativeRequestStart,
        Java_com_threadline_android_ThreadlineNative_nativeStreamStart,
    };
    use super::Status;

    #[test]
    fn contract_starts_at_version_one() {
        assert_eq!(threadline_client_ffi_contract_version(), 1);
    }

    #[test]
    fn jni_handle_results_preserve_stable_error_codes() {
        let environment = core::ptr::null_mut();
        let receiver = core::ptr::null_mut();
        let client =
            Java_com_threadline_android_ThreadlineNative_nativeClientCreate(environment, receiver);
        assert!(client > 0);
        assert_eq!(
            Java_com_threadline_android_ThreadlineNative_nativeStreamStart(
                environment,
                receiver,
                client,
                0,
                1,
                0,
                0,
                0,
            ),
            -(Status::InvalidArgument.code() as i64)
        );
        assert_eq!(
            Java_com_threadline_android_ThreadlineNative_nativeClientRelease(
                environment,
                receiver,
                client,
            ),
            Status::Ok.code()
        );
        assert_eq!(
            Java_com_threadline_android_ThreadlineNative_nativeRequestStart(
                environment,
                receiver,
                client,
                0,
                0,
            ),
            -(Status::InvalidHandle.code() as i64)
        );
    }
}
