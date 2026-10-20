"""Hermetic decision tests; the real network/process probe is explicitly opt-in."""
import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, patch

import goose_probe as probe
import goose_probe_policy as policy
import goose_probe_rpc as rpc
from goose_probe_endpoint import completion
from goose_probe_rpc import Worker


def route():
    return {"synthetic": True, "endpoint": "http://127.0.0.1:48991",
            "credential": "synthetic-probe-only", "model": "fixture-from-policy"}


class PolicyTests(unittest.TestCase):
    def test_route_accepts_only_explicit_synthetic_loopback_origin(self):
        self.assertEqual(policy.validate_route(route()), route())
        invalid = [("endpoint", value) for value in (
            "https://127.0.0.1:9", "http://localhost:9", "http://example.com:9",
            "http://user@127.0.0.1:9", "http://127.0.0.1:9/x", "http://127.0.0.1",
            "http://127.0.0.1:9?x", "http://127.0.0.1:9#x")]
        invalid += [("synthetic", False), ("synthetic", "true"), ("credential", "real-key"),
                    ("model", " "), ("model", "x" * 129), ("model", "x\0y")]
        for key, value in invalid:
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                policy.validate_route(dict(route(), **{key: value}))

    def test_hash_mismatch_fails_before_launch(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary, config = root / "binary", root / "route.json"
            binary.write_bytes(b"synthetic binary")
            config.write_text(json.dumps(route()))
            with self.assertRaisesRegex(ValueError, "digest mismatch"):
                policy.load_policy(config, binary)
            digest = hashlib.sha256(binary.read_bytes()).hexdigest()
            with patch.object(policy, "BINARY_SHA256", digest):
                self.assertEqual(policy.load_policy(config, binary), route())

    def test_launch_has_no_inherited_config_tools_or_secrets(self):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory) / "worker"
            command = policy.launch(work, Path("/pinned/goose"), route())
            config = json.loads((work / "config/config.yaml").read_text())
            self.assertEqual(set(config["extensions"]), set(policy.EXTENSIONS))
            self.assertTrue(all(not x["enabled"] for x in config["extensions"].values()))
            self.assertEqual(command[0], "/usr/bin/sandbox-exec")
            self.assertEqual(command[-1], "acp")
            self.assertIn('(deny default)', (work / "worker.sb").read_text())
            self.assertIn('localhost:48991', (work / "worker.sb").read_text())
            with patch.dict("os.environ", {"REAL_KEY": "sensitive", "HTTP_PROXY": "remote"}):
                env = policy.environment(work, route())
            self.assertNotIn("REAL_KEY", env)
            self.assertNotIn("HTTP_PROXY", env)
            self.assertEqual(env["GOOSE_MODEL"], route()["model"])
            self.assertEqual(env["OPENAI_API_KEY"], route()["credential"])
            self.assertIn("127.0.0.1", env["NO_PROXY"].split(","))
            self.assertEqual(env["NO_PROXY"], env["no_proxy"])
            other = Path(directory) / "control"
            policy.launch(other, Path("/pinned/goose"), route(), developer=True)
            config = json.loads((other / "config/config.yaml").read_text())
            self.assertEqual([k for k, v in config["extensions"].items() if v["enabled"]],
                             ["developer"])


class ObserverTests(unittest.TestCase):
    @patch.object(rpc.signal, "SIGKILL", 9, create=True)
    def test_failed_setup_reaps_started_worker_and_closes_allocated_resources(self):
        with tempfile.TemporaryDirectory() as directory:
            child, selector = Mock(), Mock()
            selector.__enter__ = Mock(return_value=selector)
            selector.__exit__ = Mock(return_value=False)
            selector.register.side_effect = OSError("synthetic registration failure")
            with patch.object(rpc.subprocess, "Popen", return_value=child), \
                    patch.object(rpc.selectors, "DefaultSelector", return_value=selector), \
                    patch.object(rpc.os, "killpg", create=True) as kill:
                with self.assertRaisesRegex(OSError, "registration"):
                    Worker(["synthetic"], {}, Path(directory))
                kill.assert_called_once_with(child.pid, rpc.signal.SIGKILL)
                child.wait.assert_called_once_with(timeout=10)
                child.stdin.close.assert_called_once()
                child.stdout.close.assert_called_once()
                selector.__exit__.assert_called_once()

    @patch.object(rpc.signal, "SIGKILL", 9, create=True)
    def test_failed_group_stop_falls_back_without_claiming_success(self):
        child = Mock()
        with patch.object(rpc.os, "killpg", side_effect=PermissionError("denied"), create=True):
            with self.assertRaises(PermissionError):
                rpc.terminate(child)
        child.kill.assert_called_once()

    def test_frame_read_enforces_size_eof_deadline_and_json(self):
        worker = Worker.__new__(Worker)
        worker.buffer = b""
        worker.selector, worker.child = Mock(), Mock()
        worker.selector.select.return_value = ["ready"]
        with patch.object(rpc.time, "monotonic", return_value=0):
            with patch.object(rpc.os, "read", return_value=b'{"id":1}\n'):
                self.assertEqual(worker.receive(1), {"id": 1})
            for block, error in ((b"", RuntimeError), (b"x" * 262145, ValueError),
                                 (b"invalid\n", ValueError)):
                worker.buffer = b""
                with patch.object(rpc.os, "read", return_value=block), self.assertRaises(error):
                    worker.receive(1)
            worker.buffer = b""
            worker.selector.select.return_value = []
            with self.assertRaises(TimeoutError):
                worker.receive(1)

    def test_known_child_observation_reports_absence_and_fails_on_observer_error(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "pid"
            marker.write_text("12345")
            with patch.object(probe.subprocess, "run") as run:
                run.return_value = SimpleNamespace(returncode=0, stdout="S\n")
                self.assertEqual(probe.child_status(marker), "S")
                run.return_value = SimpleNamespace(returncode=1, stdout="")
                self.assertEqual(probe.child_status(marker), "absent")
                run.return_value = SimpleNamespace(returncode=2, stdout="")
                with self.assertRaisesRegex(RuntimeError, "observation failed"):
                    probe.child_status(marker)

    def test_synthetic_tool_call_continuation_and_text(self):
        body = {"messages": [{"role": "user"}], "tools": []}
        delta, reason, tools = completion(body, "synthetic command")
        self.assertEqual((reason, tools), ("tool_calls", []))
        self.assertEqual(delta["tool_calls"][0]["function"]["name"], "shell")
        body["tools"] = [{"function": {"name": "shell"}}]
        self.assertEqual(completion(body, "command")[2], ["shell"])
        body["messages"].append({"role": "tool"})
        self.assertEqual(completion(body, "command")[1], "stop")
        self.assertEqual(completion(body, None)[0]["content"], "synthetic probe complete")

    def test_bounded_observer_rejects_callbacks_remote_errors_and_notice_flood(self):
        worker = Worker.__new__(Worker)
        worker.receive = Mock(side_effect=[{"method": "session/update"},
                                          {"id": 3, "result": {"stopReason": "end_turn"}}])
        self.assertEqual(worker.response(3), {"stopReason": "end_turn"})
        for frame in ({"id": 9, "method": "session/request_permission"},
                      {"method": "unknown"}, {"id": 3, "error": {"code": -1}}):
            worker.receive = Mock(return_value=frame)
            with self.assertRaises((ValueError, RuntimeError)):
                worker.response(3)
        worker.receive = Mock(return_value={"method": "session/update"})
        with self.assertRaisesRegex(ValueError, "budget"):
            worker.response(3)
        self.assertEqual(worker.receive.call_count, 128)

    def test_cleanup_receipts_do_not_turn_acp_cancel_into_containment(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            worker = Mock()
            worker.cancel.return_value = {"stopReason": "cancelled"}
            worker.close.return_value = -9
            with patch.object(probe, "launch", return_value=["synthetic"]), \
                    patch.object(probe, "Worker", return_value=worker), \
                    patch.object(probe, "wait_marker"), patch.object(probe.time, "sleep"), \
                    patch.object(probe, "child_status", side_effect=lambda _p: "absent") as status:
                status.side_effect = ["S", "S", "absent"]
                result = probe.cancellation(root, Path("/binary"), route(), SimpleNamespace(), "owner-stop")
                self.assertEqual(result["root_wait_status"], -9)
                self.assertEqual(result["sealed_and_empty"], "NOT PROVEN")
                worker.close.assert_called_once()
                (root / "foreground").mkdir()
                (root / "foreground/done").touch()
                status.side_effect = ["S", "absent", "absent"]
                with self.assertRaisesRegex(RuntimeError, "completed after"):
                    probe.cancellation(root, Path("/binary"), route(), SimpleNamespace(), "foreground")
                (root / "background").mkdir()
                (root / "background/done").touch()
                status.side_effect = ["S", "S", "absent"]
                result = probe.cancellation(root, Path("/binary"), route(), SimpleNamespace(), "background")
                self.assertTrue(result["completion_after_cancel"])


if __name__ == "__main__":
    unittest.main()
