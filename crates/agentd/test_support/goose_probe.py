"""Run explicitly: synthetic Goose R2 evidence, no production worker admission."""
import argparse
import json
import platform
import re
import subprocess
import time
from pathlib import Path

from goose_probe_endpoint import Endpoint
from goose_probe_policy import BINARY_SHA256, environment, launch, load_policy
from goose_probe_rpc import Worker


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def wait_marker(path):
    deadline = time.monotonic() + 15
    while not path.exists() or path.stat().st_size == 0:
        if time.monotonic() >= deadline:
            raise TimeoutError("fixture did not start: " + path.name)
        time.sleep(0.05)


def child_status(marker):
    # Observation of a controlled fixture only; never a PID-based kill/recovery target.
    pid = int(marker.read_text())
    require(pid > 1, "invalid synthetic child observation")
    result = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "stat="],
                            capture_output=True, text=True, timeout=5)
    require(result.returncode in (0, 1), "child observation failed")
    return result.stdout.strip() if result.returncode == 0 else "absent"


def restrictions(command, env, work):
    canary = work.parent / "denied-canary"
    canary.write_text("synthetic-only")
    sandbox = command[:3]
    positive = subprocess.run(sandbox + ["/usr/bin/curl", "-fsS", "--noproxy", "*",
                              env["OPENAI_HOST"] + "/health"], env=env, cwd=work,
                              capture_output=True, timeout=5)
    require(positive.returncode == 0 and positive.stdout == b"synthetic healthy",
            "sandboxed loopback positive control failed")
    commands = ["/bin/cat " + str(canary), "echo forbidden > " + str(canary),
                "/usr/bin/curl -v --noproxy '*' --connect-timeout 2 http://192.0.2.1:9"]
    results = []
    for check in commands:
        result = subprocess.run(sandbox + ["/bin/sh", "-c", check], env=env,
                                cwd=work, capture_output=True, timeout=5)
        denied = result.returncode != 0 and b"Operation not permitted" in result.stderr
        require(denied, "Seatbelt negative control failed: " + check)
        results.append(denied)
    require(canary.read_text() == "synthetic-only", "canary was modified")
    return results


def restricted(root, binary, route, endpoint):
    work = root / "restricted"
    command = launch(work, binary, route)
    env = environment(work, route)
    controls = restrictions(command, env, work)
    endpoint.command = "echo forbidden > " + str(work / "shell-ran")
    endpoint.requests.clear()
    worker = Worker(command, env, work)
    try:
        session = worker.session(work)
        require(session["modes"]["currentModeId"] == "approve", "wrong session mode")
        require(session["_meta"]["extensionResults"] == [], "unexpected loaded extension")
        worker.prompt()
        result = worker.response(3)
        require(endpoint.requests, "restricted turn never reached the synthetic endpoint")
        require(all(not r["tools"] for r in endpoint.requests), "restricted turn advertised tools")
        rejected = "Tool 'shell' was not advertised for this model turn"
        require(rejected in json.dumps(endpoint.requests), "forced shell was not rejected")
        require(not (work / "shell-ran").exists(), "restricted shell executed")
        return {"mode": "approve", "advertised_tools": [], "forced_shell_rejected": True,
                "stop_reason": result["stopReason"], "negative_controls": controls}
    finally:
        worker.close()


def strict_agentd(root, binary, route, endpoint, example):
    work = root / "strict"
    command = launch(work, binary, route)
    endpoint.command = None
    worker = Worker([str(example), str(work)] + command, environment(work, route), work)
    try:
        result = worker.receive(time.monotonic() + 90)
        status = worker.finish()
        worker = None
        require(status == 0, "agentd strict probe process failed")
        return result
    finally:
        if worker is not None:
            worker.close()


def cancellation(root, binary, route, endpoint, case):
    work = root / case
    command = launch(work, binary, route, developer=True)
    env = environment(work, route)
    env["GOOSE_MODE"] = "auto"  # Explicit synthetic tool control only.
    start, done = work / "start", work / "done"
    finish = "/bin/sleep 4; echo completed > " + str(done)
    if case == "foreground":
        endpoint.command = "echo $$ > " + str(start) + "; " + finish
    else:
        endpoint.command = "(" + finish + ") & echo $! > " + str(start) + "; /bin/sleep 12"
    worker = Worker(command, env, work)
    try:
        worker.session(work)
        worker.prompt()
        wait_marker(start)
        before = child_status(start)
        require(before != "absent" and not before.startswith("Z"), "fixture child not live")
        result = worker.cancel()
        after_cancel = child_status(start)
        require(result["stopReason"] == "cancelled", "missing ACP cancellation receipt")
        if case == "owner-stop":
            status = worker.close()
            worker = None
        else:
            status = None
        time.sleep(5)
        survived = done.exists()
        after_stop = child_status(start)
        if case != "background":
            require(not survived, case + " fixture completed after cancellation")
            require(after_stop == "absent" or after_stop.startswith("Z"), "fixture child still live")
        return {"started": True, "acp_stop_reason": "cancelled",
                "fixture_before": before, "fixture_after_acp_cancel": after_cancel,
                "fixture_after_observation": after_stop,
                "completion_after_cancel": survived, "root_wait_status": status,
                "sealed_and_empty": "NOT PROVEN"}
    finally:
        if worker is not None:
            worker.close()


def run(args):
    require(platform.system() == "Darwin" and platform.machine() == "arm64",
            "this pinned probe requires macOS arm64; no unrestricted fallback")
    binary, example = Path(args.binary).resolve(), Path(args.agentd_example).resolve()
    route = load_policy(args.policy, binary)
    root = Path(args.output).resolve()
    require(re.fullmatch(r"/[A-Za-z0-9/_.-]+", str(root)), "unsafe synthetic fixture path")
    root.mkdir(mode=0o700)
    receipt = {"os": platform.platform(), "binary_sha256": BINARY_SHA256,
               "routing_policy": "explicit synthetic loopback file", "production_admitted": False}
    with Endpoint(route) as endpoint:
        receipt["strict_agentd"] = strict_agentd(root, binary, route, endpoint, example)
        receipt["restricted"] = restricted(root, binary, route, endpoint)
        for case in ("foreground", "background", "owner-stop"):
            receipt[case] = cancellation(root, binary, route, endpoint, case)
    (root / "receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
    print(json.dumps(receipt, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("binary", "policy", "output", "agentd-example"):
        parser.add_argument("--" + name, required=True)
    run(parser.parse_args())
