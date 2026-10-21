"""Hermetic strict-result/cleanup cases imported by the probe-support test suite."""
import unittest
from contextlib import nullcontext
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, patch
import goose_probe as probe


def route():
    return {"synthetic": True, "endpoint": "http://127.0.0.1:48991",
            "credential": "synthetic-probe-only", "model": "fixture-from-policy"}


class StrictProbeTests(unittest.TestCase):
    def test_strict_result_requires_exact_bounded_text_evidence(self):
        result = {"initialized": True, "session_created": True, "text_matches": True,
                  "text_bytes": 24, "stop_reason": "end_turn"}
        probe.validate_strict_result(result)
        invalid = [{"initialized": True, "session_created": True,
                    "turn": "Err(UnexpectedMessage)"}, {}, None]
        invalid += [dict(result, **{key: value}) for key, value in (
            ("initialized", False), ("session_created", False), ("text_matches", False),
            ("text_bytes", 0), ("text_bytes", 25), ("stop_reason", "cancelled"))]
        for value in invalid:
            with self.subTest(value=value), self.assertRaisesRegex(RuntimeError, "bounded text"):
                probe.validate_strict_result(value)

    def test_strict_probe_validates_status_endpoint_and_tools(self):
        result = {"initialized": True, "session_created": True, "text_matches": True,
                  "text_bytes": 24, "stop_reason": "end_turn"}
        cases = [(0, [{"tools": []}], result, None),
                 (1, [{"tools": []}], result, "process failed"),
                 (0, [], result, "never reached"),
                 (0, [{"tools": ["shell"]}], result, "advertised tools"),
                 (0, [{"tools": []}], {}, "bounded text")]
        for status, requests, receipt, error in cases:
            worker = Mock()
            worker.receive.return_value = receipt
            worker.finish.return_value = status
            endpoint = SimpleNamespace(command="stale", requests=[{"tools": ["stale"]}])
            def emit(_deadline):
                endpoint.requests.extend(requests)
                return receipt
            worker.receive.side_effect = emit
            with self.subTest(error=error), patch.object(probe, "Worker", return_value=worker), \
                    patch.object(probe, "launch", return_value=["synthetic"]):
                args = (Path("/synthetic"), Path("/binary"), route(), endpoint, Path("/example"))
                with self.assertRaisesRegex(RuntimeError, error) if error else nullcontext():
                    actual = probe.strict_agentd(*args)
                if error is None:
                    self.assertEqual(actual, dict(result, advertised_tools=[]))
                self.assertIsNone(endpoint.command)
                self.assertEqual(endpoint.requests, requests)
                worker.finish.assert_called_once()
                worker.close.assert_not_called()

    def test_failed_strict_exchange_cleans_up_retained_worker(self):
        for operation in ("receive", "finish"):
            worker = Mock()
            getattr(worker, operation).side_effect = TimeoutError("synthetic timeout")
            endpoint = SimpleNamespace(command=None, requests=[])
            with self.subTest(operation=operation), \
                    patch.object(probe, "Worker", return_value=worker), \
                    patch.object(probe, "launch", return_value=["synthetic"]):
                with self.assertRaisesRegex(TimeoutError, "synthetic timeout"):
                    probe.strict_agentd(Path("/synthetic"), Path("/binary"), route(),
                                        endpoint, Path("/example"))
                worker.close.assert_called_once()
