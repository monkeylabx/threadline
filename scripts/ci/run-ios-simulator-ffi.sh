#!/usr/bin/env bash
set -euo pipefail

: "${THREADLINE_FFI_LIBRARY_DIR:?THREADLINE_FFI_LIBRARY_DIR is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"

runtime_identifier="$({ xcrun simctl list --json runtimes; } | node -e '
  const fs = require("node:fs");
  const data = JSON.parse(fs.readFileSync(0, "utf8"));
  const runtimes = data.runtimes
    .filter((runtime) => runtime.isAvailable && runtime.identifier.includes("SimRuntime.iOS-"))
    .sort((left, right) => left.version.localeCompare(right.version, undefined, { numeric: true }));
  if (runtimes.length === 0) process.exit(2);
  process.stdout.write(runtimes.at(-1).identifier);
')"

device_type_identifier="$({ xcrun simctl list --json devicetypes; } | node -e '
  const fs = require("node:fs");
  const data = JSON.parse(fs.readFileSync(0, "utf8"));
  const preferred = ["iPhone 17 Pro", "iPhone 16 Pro", "iPhone 15 Pro"];
  const devices = data.devicetypes.filter((device) => device.name.startsWith("iPhone"));
  const selected = preferred.map((name) => devices.find((device) => device.name === name)).find(Boolean) ?? devices[0];
  if (!selected) process.exit(2);
  process.stdout.write(selected.identifier);
')"

simulator_name="Threadline-T010A-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}"
simulator_id="$(xcrun simctl create "${simulator_name}" "${device_type_identifier}" "${runtime_identifier}")"
result_bundle="${RUNNER_TEMP}/threadline-ios-simulator.xcresult"
derived_data="${RUNNER_TEMP}/threadline-ios-simulator-derived-data"

cleanup() {
  xcrun simctl shutdown "${simulator_id}" >/dev/null 2>&1 || true
  xcrun simctl delete "${simulator_id}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

xcrun simctl boot "${simulator_id}"
xcrun simctl bootstatus "${simulator_id}" -b

pushd apps/ios >/dev/null
xcodebuild test \
  -scheme ThreadlineIOSHost \
  -destination "platform=iOS Simulator,id=${simulator_id}" \
  -derivedDataPath "${derived_data}" \
  -resultBundlePath "${result_bundle}" \
  CODE_SIGNING_ALLOWED=NO
popd >/dev/null

{
  echo "### T010-A iOS Simulator"
  echo
  echo "- Runtime: \`${runtime_identifier}\`"
  echo "- Device type: \`${device_type_identifier}\`"
  echo "- Rust facade: universal simulator static library"
  echo "- XCTest result: passed"
} >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
