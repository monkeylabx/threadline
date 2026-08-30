# Private development stack

T016 provides two equivalent local dependency stacks:

- Docker Compose for ordinary workstation development.
- Kind for Kubernetes and NetworkPolicy development.

Both contain PostgreSQL, NATS JetStream, Redis, MinIO, OpenTelemetry
Collector, Prometheus, and Jaeger. No Threadline business workload is
connected yet. The NATS checks described below are deployment probes, not a
Worker runtime.

## Security boundary

All committed credentials are fixed, conspicuously non-production values and
are distinct per dependency so a compromised local service does not inherit a
reusable credential for its peers:

| Service | User | Password / database |
| --- | --- | --- |
| PostgreSQL | `threadline_postgres_dev` | `threadline-postgres-dev-only`; database `threadline` |
| NATS Worker fixture | `threadline_worker_dev` | `threadline-worker-dev-only` |
| Redis | n/a | `threadline-redis-dev-only` |
| MinIO | `threadline_minio_dev` | `threadline-minio-dev-only` |

Never replace them with production credentials, tokens, signing material,
recovery keys, or encoded equivalents. Do not seed real messages, files,
prompts, or user data into this stack.

The OpenTelemetry Collector accepts metrics and traces only. It deliberately
does not define a log pipeline. A fail-closed Collector-side guard runs before
either exporter; callers cannot opt out of it:

- Resource and instrumentation-scope metadata is reduced to Collector-written
  development constants.
- Span names, event names, status messages, trace state, and all caller span
  and event attributes are replaced with fixed safe values. Spans carrying
  links are dropped because Collector `0.107.0` has no link-level transform
  context for safely inspecting link attributes.
- Only `threadline.dev.operation.duration`,
  `threadline.dev.operation.count`, and
  `threadline.dev.operation.errors` metric names are accepted. Descriptions,
  units, resource/scope metadata, and datapoint attributes are replaced with
  fixed safe values. Datapoints carrying exemplars are dropped because
  exemplar filtered attributes cannot be covered by the datapoint allowlist.
- OTTL parse or runtime errors propagate and drop the affected payload rather
  than bypassing the guard.

This prevents accidental export of attributes named or containing message,
file, prompt, token, key, path, stdout, stderr, and other unreviewed content:
all caller-supplied attribute keys are removed, including unknown future keys.
Applications must still never put message bodies, file contents, prompts,
tokens, keys, or other content into telemetry. The guard is not a defense
against a malicious instrumenter encoding data into numeric measurements,
timestamps, trace/span IDs, OTLP schema URLs (which Collector `0.107.0` does
not expose to these transform contexts), or traffic shape. Instrumentation
must use only fixed, reviewed semantic-convention schema URLs. Extending any
allowlist requires a reviewed Collector configuration change in both Compose
and Kind.

T016 does not provide KMS/HSM integration. Compose declares a separate
internal `recovery-control` network. Kind declares the
`threadline-recovery-dev` Namespace with a default-deny ingress/egress
NetworkPolicy and no workload. Future Recovery Control work must add explicit
identity, peer and KMS/HSM allowlists; it must not relax the default policy.

### Restricted NATS Worker fixture

The pinned NATS `2.10.20` fixture uses `deploy/nats/nats-server.conf`. It
declares exactly two accounts: `SYS`, with no externally authenticatable
principal, and `THREADLINE_WORKER`, with one enumerated development principal.
The Worker principal may publish only to `threadline.domain.events.v1` and
`$SYS.REQ.USER.INFO`; it may subscribe only to
`_INBOX.threadline.worker.>`. Explicit denies prevent it from subscribing to
the exact or mapped wildcard USER.INFO subjects and from publishing to its own
reply-inbox namespace. No anonymous user, authentication callout, leaf node,
gateway, account import, or account export is configured. The password stored
in the server config is bcrypt-hashed; the conspicuously development-only
cleartext value appears only in the local fixture controls.

`make ... up` validates the config with the pinned server binary and runs a
redacting, hermetic permission probe. The probe authenticates every declared
application principal (currently one), requires exact effective identity and
permissions from the server-owned USER.INFO service, proves anonymous and
wrong credentials fail, proves exact and wildcard USER.INFO subscriptions
fail, and proves the Worker cannot publish its reply inbox. It records
JetStream account storage counters before and after, and never publishes the
business subject. A same-account client therefore cannot impersonate the
USER.INFO responder in this fixture. Probe failures intentionally do not print
credentials, subjects, domains, or server responses.

This evidence is limited to the reviewed local broker fixture. It does not
prove production credential distribution, pod admission controls, NATS
transport encryption, protection from a Kubernetes or Docker administrator,
or that the future Worker connection is exclusively owned by trusted Worker
code. Production readiness remains blocked until Worker connection custody is
implemented and audited separately.

## Image preparation

The `up` commands never pull images. Every workload is pinned as
`repository:version@sha256:manifest-list-digest`, and Kind uses
`imagePullPolicy: Never`. The committed pins are checked against the Compose
and Kubernetes manifests before startup; local `RepoDigests` must match.

The validated tool baseline is Docker Engine `29.6.2`, Docker Compose
`5.4.0`, Kind `v0.32.0`, kubectl `v1.36.2`, and the Kind node image
`kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5`.
The Makefiles fail closed on a different version. Connected preparation
verifies each registry manifest-list digest. Because Docker archive import
restores images and tags but not registry `RepoDigests`, the offline bundle
also carries an exact image-ID manifest and checksum. Offline startup uses
tag aliases only after the archive, platform, ID manifest, and exact locked
image set have all been verified. Update tools and image digests together in
one reviewed change; never move a tag without its digest.

These checks validate tools that are already installed. T016 does not provide
tool installers, binary packages, or their checksums, so it is not a complete
reproducible tool-supply chain. Provision and verify those packages through the
organization's approved software distribution process before running the
Makefiles.

On a connected preparation machine:

```text
make -C deploy/compose images
make -C deploy/compose save-images ARCHIVE=/tmp/threadline-compose-images.tar

make -C deploy/kind images
make -C deploy/kind save-images ARCHIVE=/tmp/threadline-kind-images.tar
```

Each `save-images` command writes adjacent `.sha256`, `.platform`, `.ids`, and
`.ids.sha256` files. Copy all five files together. `linux/amd64` and
`linux/arm64` are supported;
the default follows the Docker daemon architecture and can be made explicit
with `PLATFORM=linux/arm64` or `PLATFORM=linux/amd64`. `load-images` rejects a
platform mismatch, verifies both SHA-256 sidecars before load, then verifies
every imported image ID against the exact committed lock set before the
archive can be used. Treat the checksums and ID manifest as release evidence;
distribute them through the organization's authenticated evidence channel.

Copy the archive into the isolated environment, then load it:

```text
make -C deploy/compose load-images ARCHIVE=/path/to/threadline-compose-images.tar
make -C deploy/kind load-images ARCHIVE=/path/to/threadline-kind-images.tar
```

Image acquisition is the only connected phase. The committed manifest-list
digests were resolved from the official registries; still run vulnerability,
license and provenance review before distributing an enterprise offline bundle.

Kind `v0.32.0` imports a Docker image through containerd with
`--all-platforms`, which fails when a native-only pull retains a multi-platform
index whose other platform content is intentionally absent. The Makefile uses
containerd's current-platform `--digests` import instead, while keeping the
committed manifest-list lock. Kustomize renders fixed version-tag aliases for
the node's current-platform runtime because Kubernetes otherwise asks
containerd for the absent multi-platform index verbatim. Those aliases are
usable only after the Makefile verifies their image IDs against the connected
digest lock or the authenticated offline ID manifest. Normal rollouts and
NetworkPolicy probes then verify the imported workload.

## Compose

Start all dependencies with one command:

```text
make -C deploy/compose up
```

All published ports bind to loopback only:

| Service | Address |
| --- | --- |
| PostgreSQL | `127.0.0.1:5432` |
| NATS | `127.0.0.1:4222` |
| NATS monitoring | `http://127.0.0.1:8222` |
| Redis | `127.0.0.1:6379` |
| MinIO API | `http://127.0.0.1:9000` |
| MinIO console | `http://127.0.0.1:9001` |
| OTLP gRPC | `127.0.0.1:4317` |
| OTLP HTTP | `http://127.0.0.1:4318` |
| Prometheus | `http://127.0.0.1:9090` |
| Jaeger | `http://127.0.0.1:16686` |

Override a host port through the corresponding `THREADLINE_DEV_*_PORT`
environment variable.

Compose uses a separate internal network for each standalone dependency and
each required observability edge (`Prometheus -> OTel` and `OTel -> Jaeger`).
MinIO, OTel, Prometheus, and Jaeger each have a single-network BusyBox
readiness probe; no probe bridges dependency networks. Compose networks do not
provide directional policy, so the two observability edges remain symmetric
at transport level and must never be treated as production isolation.

Operational commands:

```text
make -C deploy/compose config
make -C deploy/compose verify-nats-config
make -C deploy/compose verify-nats-worker
make -C deploy/compose status
make -C deploy/compose logs
make -C deploy/compose down
make -C deploy/compose destroy
make -C deploy/compose recreate
```

`down` retains named volumes. `destroy` removes them. `recreate` destroys all
local state and proves that a clean stack can be rebuilt from local images.

## Kind

Start the cluster and all dependencies with one command:

```text
make -C deploy/kind up
```

The command requires the Kind node image and every workload image to already
exist in the local Docker image store. It creates the cluster if necessary,
or fails if an existing cluster's node image or Kubernetes server version does
not match the pins. It then loads images, applies Kustomize resources, restarts
config-consuming Deployments, waits for those rollouts and all seven
Deployments to become Available, and runs allow/deny NetworkPolicy probes.
Waiting on Deployments avoids the first-create race where a Pod query can
briefly match zero resources. NetworkPolicy probes verify both permitted
observability edges (`Prometheus -> OTel` and `OTel -> Jaeger`), the intended
`Worker -> NATS:4222` edge, and reject all three destinations from an unlabeled
source. The temporary Worker-labeled probe receives no credential. The
development Worker credential is held in a dedicated Secret that is not
mounted into any dependency workload; the host-side permission probe reads
that Secret through the local cluster administrator context without printing
it. The probes use the locked BusyBox image, wait
for asynchronous policy reconciliation, and remove their temporary Pods.

Services remain ClusterIP-only. Use temporary port forwarding when needed:

```text
kubectl --context kind-threadline-dev -n threadline-dev port-forward service/postgres 5432:5432
kubectl --context kind-threadline-dev -n threadline-dev port-forward service/minio 9000:9000 9001:9001
kubectl --context kind-threadline-dev -n threadline-dev port-forward service/prometheus 9090:9090
kubectl --context kind-threadline-dev -n threadline-dev port-forward service/jaeger 16686:16686
```

Operational commands:

```text
make -C deploy/kind config
make -C deploy/kind verify-cluster
make -C deploy/kind verify-network-policy
make -C deploy/kind verify-nats-config
make -C deploy/kind verify-nats-worker
make -C deploy/kind status
make -C deploy/kind down
make -C deploy/kind destroy
make -C deploy/kind recreate
```

`down` removes stack resources but leaves the cluster. `destroy` removes the
entire cluster. Kind dependency data uses `emptyDir`, so replacement or
destruction is intentionally data-destructive and reproducible.

## Verification

Run from the repository root:

```text
make -C deploy/compose config
python3 deploy/nats/contract_test.py
python3 deploy/nats/probe_test.py
make -C deploy/compose verify-lock
make -C deploy/kind verify-lock
kubectl kustomize deploy/kind >/dev/null
git diff --check
```

With Docker, Kind and kubectl available, also run:

```text
make -C deploy/compose up
make -C deploy/compose verify-nats-worker
make -C deploy/compose status
make -C deploy/compose recreate
make -C deploy/compose destroy

make -C deploy/kind up
make -C deploy/kind verify-nats-worker
make -C deploy/kind status
make -C deploy/kind recreate
make -C deploy/kind destroy
```
