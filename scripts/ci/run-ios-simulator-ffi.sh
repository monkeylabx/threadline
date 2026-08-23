#!/usr/bin/env bash
set -euo pipefail

: "${THREADLINE_FFI_LIBRARY_DIR:?THREADLINE_FFI_LIBRARY_DIR is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"

runtime_identifier="com.apple.CoreSimulator.SimRuntime.iOS-26-5"
device_type_identifier="com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro"

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
export THREADLINE_STREAM_TRACE=1
xcodebuild test \
  -scheme ThreadlineIOSHost \
  -destination "platform=iOS Simulator,id=${simulator_id}" \
  -derivedDataPath "${derived_data}" \
  -resultBundlePath "${result_bundle}" \
  CODE_SIGNING_ALLOWED=NO

xcodebuild test \
  -scheme ThreadlineIOSHost \
  -destination "platform=iOS Simulator,id=${simulator_id}" \
  -derivedDataPath "${derived_data}" \
  -only-testing:ThreadlineIOSHostTests/ThreadlineIOSHostTests/testSlowStreamConsumersDoNotStarveIndependentStreamCompletion \
  -test-iterations 5 \
  CODE_SIGNING_ALLOWED=NO
popd >/dev/null

{
  echo "### T010-A iOS Simulator"
  echo
  echo "- Runtime: \`${runtime_identifier}\`"
  echo "- Device type: \`${device_type_identifier}\`"
  echo "- Rust facade: universal simulator static library"
  echo "- Stream ordering trace: \`[threadline-stream-order]\` enabled"
  echo "- Stream starvation XCTest repetitions: \`5\`"
  echo "- XCTest result: passed"
} >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
