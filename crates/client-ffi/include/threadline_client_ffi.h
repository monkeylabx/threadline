#ifndef THREADLINE_CLIENT_FFI_H
#define THREADLINE_CLIENT_FFI_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

enum threadline_status {
  THREADLINE_STATUS_OK = 0,
  THREADLINE_STATUS_INVALID_ARGUMENT = 1,
  THREADLINE_STATUS_INVALID_HANDLE = 2,
  THREADLINE_STATUS_CLOSED = 3,
  THREADLINE_STATUS_CANCELED = 4,
  THREADLINE_STATUS_PANIC = 5,
  THREADLINE_STATUS_ALREADY_COMMITTED = 6,
  THREADLINE_STATUS_TIMED_OUT = 7,
  THREADLINE_STATUS_BACKPRESSURE = 8,
  THREADLINE_STATUS_PROTOCOL_VIOLATION = 9,
  THREADLINE_STATUS_END_OF_STREAM = 10,
  THREADLINE_STATUS_UNKNOWN = 255,
};

enum threadline_fault {
  THREADLINE_FAULT_NONE = 0,
  THREADLINE_FAULT_DELAYED = 1,
  THREADLINE_FAULT_PANIC = 2,
  THREADLINE_FAULT_UNKNOWN_ERROR = 3,
  THREADLINE_FAULT_DUPLICATE_EVENT = 4,
  THREADLINE_FAULT_LATE_EVENT = 5,
};

enum threadline_resource_kind {
  THREADLINE_RESOURCE_CLIENT = 1,
  THREADLINE_RESOURCE_REQUEST = 2,
  THREADLINE_RESOURCE_STREAM = 3,
  THREADLINE_RESOURCE_BUFFER = 4,
};

typedef struct threadline_buffer {
  const uint8_t *data;
  size_t len;
  uint64_t handle;
} threadline_buffer;

typedef struct threadline_stream_metrics {
  uint32_t capacity;
  uint32_t max_depth;
  uint64_t backpressure_count;
  uint64_t suppressed_late_events;
} threadline_stream_metrics;

uint32_t threadline_client_ffi_contract_version(void);

int32_t threadline_client_create(uint64_t *output);
int32_t threadline_client_close(uint64_t handle);
int32_t threadline_client_release(uint64_t handle);

int32_t threadline_request_start(uint64_t client_handle, int32_t fault,
                                 uint64_t delay_ms, uint64_t *output);
int32_t threadline_request_cancel(uint64_t handle);
int32_t threadline_request_state(uint64_t handle);
int32_t threadline_request_is_committed(uint64_t handle, uint8_t *committed);
int32_t threadline_request_wait(uint64_t handle, uint64_t timeout_ms,
                                uint8_t *committed);
int32_t threadline_request_result(uint64_t handle, threadline_buffer *output);
int32_t threadline_request_close(uint64_t handle);
int32_t threadline_request_release(uint64_t handle);

int32_t threadline_buffer_release(uint64_t handle);

int32_t threadline_stream_start(uint64_t client_handle, uint64_t cursor,
                                uint32_t event_count, uint32_t capacity,
                                uint64_t delay_ms, int32_t fault,
                                uint64_t *output);
int32_t threadline_stream_next(uint64_t handle, uint64_t timeout_ms,
                               uint64_t *sequence);
int32_t threadline_stream_get_metrics(uint64_t handle,
                                      threadline_stream_metrics *output);
int32_t threadline_stream_cancel(uint64_t handle);
int32_t threadline_stream_close(uint64_t handle);
int32_t threadline_stream_release(uint64_t handle);

uint64_t threadline_debug_resource_count(int32_t kind);

#ifdef __cplusplus
}
#endif

#endif
