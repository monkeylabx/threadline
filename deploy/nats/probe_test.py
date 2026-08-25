#!/usr/bin/env python3
"""Tests for the hermetic NATS permission probe."""

from __future__ import annotations

import copy
import io
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

from probe import (
    AuthenticationRejected,
    NATSClient,
    ProbeError,
    authentication_must_fail,
    jetstream_signature,
    validate_user_info,
)


PROBE = Path(__file__).with_name("probe.py")


EXPECTED_INFO = {
    "data": {
        "user": "threadline_worker_dev",
        "account": "THREADLINE_WORKER",
        "permissions": {
            "publish": {
                "allow": ["threadline.domain.events.v1", "$SYS.REQ.USER.INFO"],
                "deny": ["_INBOX.threadline.worker.>"],
            },
            "subscribe": {
                "allow": ["_INBOX.threadline.worker.>"],
                "deny": ["$SYS.REQ.USER.INFO", "$SYS.REQ.USER.*.INFO"],
            },
        },
    }
}


class EffectivePermissionTest(unittest.TestCase):
    def test_accepts_the_exact_effective_permissions(self) -> None:
        validate_user_info(EXPECTED_INFO)

    def test_rejects_a_wrong_account(self) -> None:
        changed = copy.deepcopy(EXPECTED_INFO)
        changed["data"]["account"] = "SYS"
        with self.assertRaises(ProbeError):
            validate_user_info(changed)

    def test_rejects_a_broadened_effective_subscription(self) -> None:
        changed = copy.deepcopy(EXPECTED_INFO)
        changed["data"]["permissions"]["subscribe"]["allow"] = [">"]
        with self.assertRaises(ProbeError):
            validate_user_info(changed)

    def test_user_info_waits_for_message_after_pong(self) -> None:
        payload = b'{"data":{"ok":true}}'
        client = object.__new__(NATSClient)
        client.reader = io.BytesIO(
            b"PONG\r\n"
            b"MSG _INBOX.threadline.worker.fixed 1 "
            + str(len(payload)).encode("ascii")
            + b"\r\n"
            + payload
            + b"\r\n"
        )
        client.socket = _RecordingSocket()
        with patch("probe.secrets.token_hex", return_value="fixed"):
            self.assertEqual(client.user_info(), {"data": {"ok": True}})


class _RecordingSocket:
    def __init__(self) -> None:
        self.sent = bytearray()

    def sendall(self, value: bytes) -> None:
        self.sent.extend(value)


class AuthenticationTest(unittest.TestCase):
    def test_accepts_only_explicit_authentication_rejection(self) -> None:
        with patch("probe.NATSClient", side_effect=AuthenticationRejected("rejected")):
            authentication_must_fail("127.0.0.1", 4222, 1, None, None)

    def test_does_not_treat_transport_failure_as_authentication_rejection(self) -> None:
        with patch("probe.NATSClient", side_effect=ProbeError("network failed")):
            with self.assertRaisesRegex(ProbeError, "network failed"):
                authentication_must_fail("127.0.0.1", 4222, 1, None, None)


class JetStreamStateTest(unittest.TestCase):
    def setUp(self) -> None:
        self.empty_fixture = {
            "memory": 0,
            "storage": 0,
            "streams": 0,
            "consumers": 0,
            "messages": 0,
            "bytes": 0,
            "account_details": [
                {
                    "name": "THREADLINE_WORKER",
                    "memory": 0,
                    "storage": 0,
                    "reserved_memory": 18446744073709551615,
                    "reserved_storage": 18446744073709551615,
                    "api": {"total": 0, "errors": 0},
                }
            ],
        }

    def test_signature_ignores_monitoring_counters(self) -> None:
        before = copy.deepcopy(self.empty_fixture)
        after = copy.deepcopy(before)
        after["account_details"][0]["api"]["total"] = 99
        self.assertEqual(jetstream_signature(before), jetstream_signature(after))

    def test_signature_detects_persisted_state_change(self) -> None:
        before = copy.deepcopy(self.empty_fixture)
        after = copy.deepcopy(before)
        after["streams"] = 1
        after["account_details"][0]["stream_detail"] = [{"name": "unexpected"}]
        self.assertNotEqual(jetstream_signature(before), jetstream_signature(after))

    def test_signature_requires_the_worker_account(self) -> None:
        missing = copy.deepcopy(self.empty_fixture)
        missing["account_details"] = []
        with self.assertRaisesRegex(ProbeError, "account set"):
            jetstream_signature(missing)


class RedactionTest(unittest.TestCase):
    def test_missing_credentials_returns_only_a_fixed_error(self) -> None:
        result = subprocess.run(
            [sys.executable, str(PROBE)],
            env={},
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertEqual(result.stderr, "NATS worker permission probe failed\n")


if __name__ == "__main__":
    unittest.main()
