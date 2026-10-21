# Development Realtime StreamEvents

NOT PRODUCTION. Run `go run ./realtime` from `services/` with
`THREADLINE_REALTIME_CORE_URL`, `THREADLINE_REALTIME_NATS_URL`,
`THREADLINE_NATS_REALTIME_USER` and `THREADLINE_NATS_REALTIME_PASSWORD`.
`THREADLINE_REALTIME_LISTEN_ADDRESS` defaults to `127.0.0.1:8082`.
The #329 reader subscribes only to the fixed Event subject and cannot publish.

Route only `SyncService/StreamEvents` to this process; all unary Sync and message
commands remain on Core. Each stream requires one Channel cursor and a bearer
credential forwarded only to the configured Core origin (redirects disabled).
Every wake or 2-second reconciliation fetches currently authorized contiguous
Core events. JetStream bytes never become client content. Gaps and reconnects
use the caller's applied cursor. No broker replay or Realtime database exists.

Bounds: 256 streams, one coalesced wake each, 32 pending notifications/1 MiB,
one Event per Core page, 128 pages per catch-up, 1-minute stream lifetime and
30-second write deadline. A slow/canceled client reconnects with its own cursor.
Core permission rejection terminates the stream without delivery.

Run `go test -race ./realtime/...`. Set `THREADLINE_TEST_REALTIME_NATS_URL` to the
pinned #329 fixture to run two real Connect subscribers, broker fan-out,
outsider denial and subscription cleanup. That test uses a typed fake Core;
real Core integration evidence is recorded separately in the task handoff.
