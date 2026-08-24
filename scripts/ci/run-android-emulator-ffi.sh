#!/usr/bin/env bash
set -euo pipefail

: "${ANDROID_SDK_ROOT:?ANDROID_SDK_ROOT is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"

avd_name="threadline-t010a"
system_image="system-images;android-35;google_apis;x86_64"
emulator_log="${RUNNER_TEMP}/threadline-android-emulator.log"
avd_home="${RUNNER_TEMP}/threadline-android-avd"
avdmanager="${ANDROID_SDK_ROOT}/cmdline-tools/latest/bin/avdmanager"
emulator="${ANDROID_SDK_ROOT}/emulator/emulator"
adb="${ANDROID_SDK_ROOT}/platform-tools/adb"
export ANDROID_AVD_HOME="${avd_home}"
repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
android_root="${repository_root}/apps/android"

mkdir -p "${ANDROID_AVD_HOME}"

cleanup() {
  timeout 5s "${adb}" emu kill >/dev/null 2>&1 || true
  if [[ -n "${emulator_pid:-}" ]]; then
    kill "${emulator_pid}" >/dev/null 2>&1 || true
  fi
}
diagnostics() {
  local exit_code=$?
  if [[ ${exit_code} -ne 0 ]]; then
    echo "Android Emulator diagnostics (last 200 lines):"
    tail -n 200 "${emulator_log}" 2>/dev/null || true
    timeout 10s "${adb}" logcat -d -t 300 2>/dev/null || true
  fi
  cleanup
  trap - EXIT
  exit "${exit_code}"
}
trap diagnostics EXIT

printf 'no\n' | "${avdmanager}" create avd \
  --force \
  --name "${avd_name}" \
  --package "${system_image}" \
  --device pixel_6

"${emulator}" \
  -avd "${avd_name}" \
  -no-window \
  -no-audio \
  -no-boot-anim \
  -no-snapshot \
  -no-metrics \
  -gpu swiftshader_indirect \
  >"${emulator_log}" 2>&1 &
emulator_pid=$!

"${adb}" start-server
device_state=""
for _ in $(seq 1 60); do
  if ! kill -0 "${emulator_pid}" 2>/dev/null; then
    echo "Android Emulator exited before registering with adb" >&2
    exit 1
  fi
  device_state="$("${adb}" get-state 2>/dev/null || true)"
  if [[ "${device_state}" == "device" ]]; then
    break
  fi
  sleep 2
done
if [[ "${device_state}" != "device" ]]; then
  echo "Android Emulator did not register with adb within two minutes" >&2
  exit 1
fi

boot_completed=""
for _ in $(seq 1 180); do
  if ! kill -0 "${emulator_pid}" 2>/dev/null; then
    echo "Android Emulator exited before completing boot" >&2
    exit 1
  fi
  boot_completed="$("${adb}" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')"
  if [[ "${boot_completed}" == "1" ]]; then
    break
  fi
  sleep 2
done
if [[ "${boot_completed}" != "1" ]]; then
  echo "Android Emulator did not boot within six minutes" >&2
  exit 1
fi

"${adb}" shell settings put global window_animation_scale 0
"${adb}" shell settings put global transition_animation_scale 0
"${adb}" shell settings put global animator_duration_scale 0

"${android_root}/gradlew" -p "${android_root}" connectedDebugAndroidTest --no-daemon

{
  echo "### T010-A Android Emulator"
  echo
  echo "- System image: \`${system_image}\`"
  echo "- ABI: \`x86_64\`"
  echo "- Instrumentation result: passed"
} >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
