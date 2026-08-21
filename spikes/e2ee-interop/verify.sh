#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
SPIKE="$ROOT/spikes/e2ee-interop"
VECTOR="$ROOT/test/crypto/e2ee-interop-v1.vector"

: "${CARGO:=cargo}"
: "${GRADLE_USER_HOME:=${TMPDIR:-/tmp}/threadline-t011-gradle}"
export GRADLE_USER_HOME

"$CARGO" test --manifest-path "$SPIKE/rust/Cargo.toml" --locked
"$CARGO" test --manifest-path "$SPIKE/interop-mls-rs/Cargo.toml" --locked
swift run --package-path "$SPIKE/swift" T011SwiftHarness "$VECTOR"
"$ROOT/gradlew" -p "$SPIKE/kotlin" run --args="$VECTOR"

echo "t011: PASS Rust/OpenMLS + OpenMLS<->mls-rs interop + Swift + Kotlin harnesses"
