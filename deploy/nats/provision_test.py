"""Hermetic create-or-check and bounded reply regression tests."""

import copy
import io
import unittest
from unittest.mock import Mock

from provision import CONFIG, provision, validate
from probe import ProbeError
from request import response


class StreamProvisionTest(unittest.TestCase):
    def test_broker_omission_of_false_no_ack_is_valid(self):
        config = dict(CONFIG)
        del config["no_ack"]
        validate({"config": config})

    def test_existing_exact_config_is_only_inspected(self):
        client = Mock()
        client.request.return_value = {"config": CONFIG}
        provision(client)
        self.assertEqual(client.request.call_count, 2)
        self.assertTrue(all(".INFO." in call.args[0] for call in client.request.call_args_list))

    def test_missing_stream_is_created_then_inspected(self):
        client = Mock()
        client.request.side_effect = [{"error": {"err_code": 10059}}, {"config": CONFIG}, {"config": CONFIG}]
        provision(client)
        self.assertIn(".CREATE.", client.request.call_args_list[1].args[0])
        self.assertEqual(client.request.call_args_list[1].args[1], CONFIG)

    def test_existing_drift_never_updates_or_creates(self):
        client = Mock()
        client.request.return_value = {"config": {**CONFIG, "duplicate_window": 1}}
        with self.assertRaises(ProbeError):
            provision(client)
        self.assertEqual(client.request.call_count, 1)

    def test_non_missing_errors_do_not_create(self):
        for error in ({"code": 403}, {}, "bad", {"err_code": 10058}):
            client = Mock()
            client.request.return_value = {"error": error}
            with self.assertRaises(ProbeError):
                provision(client)
            self.assertEqual(client.request.call_count, 1)

    def test_rejects_every_config_drift(self):
        for key in CONFIG:
            changed = copy.deepcopy(CONFIG)
            changed[key] = None
            with self.subTest(key=key), self.assertRaises(ProbeError):
                validate({"config": changed})
        for key in ("sealed", "deny_delete"):
            with self.subTest(key=key), self.assertRaises(ProbeError):
                validate({"config": {**CONFIG, key: True}})

    def test_rejects_bad_create_and_final_read(self):
        for replies in ([{"error": {"err_code": 10059}}, {"error": {"code": 400}}],
                        [{"config": CONFIG}, {"config": {}}]):
            client = Mock()
            client.request.side_effect = replies
            with self.assertRaises(ProbeError):
                provision(client)


class ReplyTest(unittest.TestCase):
    def test_reads_bounded_exact_reply(self):
        client = Mock(reader=io.BytesIO(b'{"ok":true}\r\n'))
        client._read_line.side_effect = [b"PONG", b"MSG inbox 1 11"]
        self.assertEqual(response(client, "inbox"), {"ok": True})

    def test_rejects_malformed_foreign_and_flooded_replies(self):
        for line in (b"MSG foreign 1 2", b"MSG inbox 1 -1", b"MSG inbox 1 1048577",
                     b"MSG inbox 1 nan", b"-ERR denied", b"PONG"):
            client = Mock(reader=io.BytesIO(b'{}\r\n'))
            client._read_line.return_value = line
            with self.subTest(line=line), self.assertRaises(ProbeError):
                response(client, "inbox")

    def test_rejects_truncated_json_and_nonobjects(self):
        for payload in (b"{", b"[]", b"null", b"\xff"):
            client = Mock(reader=io.BytesIO(payload + b"\r\n"))
            client._read_line.return_value = f"MSG inbox 1 {len(payload)}".encode()
            with self.subTest(payload=payload), self.assertRaises(ProbeError):
                response(client, "inbox")


if __name__ == "__main__":
    unittest.main()
