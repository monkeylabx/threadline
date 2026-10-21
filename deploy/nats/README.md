# Development domain Event fixture

These principals and passwords are public **NOT PRODUCTION** fixtures. The SYS
account remains inaccessible. Worker can publish only the domain Event subject,
query USER.INFO and inspect the exact mapped Stream; it cannot create, update or
delete Streams. The platform principal can create/inspect only the mapped Stream
and cannot publish business events. Its public password is intentionally shared
with the pre-existing dev fixture; never reuse either credential in production.

`make -C deploy/compose up` creates or verifies the Stream after startup. CI uses
the same `provision.py` twice before the non-mutating permission probe. No new
container or unpinned download is needed. Kind mirrors the account configuration;
a Kind provisioning Job is outside this Slice A Compose task.

Realtime uses `threadline_realtime_dev` with the same public dev-only password.
It may subscribe only to the exact Event subject and cannot publish any subject.

The Go connection **must** set
`nats.CustomInboxPrefix("_INBOX.threadline.worker")`. The default random inbox
is deliberately unauthorized. Startup inspection must use the same authenticated
connection as publishing; do not introduce a privileged management connection.

Checks:

```sh
python3 -m unittest discover -s deploy/nats -p '*_test.py'
THREADLINE_TEST_NATS_BINARY=/absolute/path/to/nats-server python3 deploy/nats/provision_integration_test.py
```

The opt-in integration suite requires exactly NATS 2.10.20, starts isolated local
processes and removes their data. It proves create/recheck and unchanged drift
rejection. The effective-permission probe proves Worker management mutation is
denied without publishing or changing Stream state. #148's production permission
proof is not closed by development fixture evidence.
