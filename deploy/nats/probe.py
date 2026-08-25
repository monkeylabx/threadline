#!/usr/bin/env python3
"""Hermetic, non-publishing probe for the restricted NATS Worker principal."""

from __future__ import annotations

import argparse
import json
import os
import secrets
import socket
import sys
import urllib.request
from typing import Any, BinaryIO, Optional

from contract import (
    BUSINESS_SUBJECT,
    USER_INFO_SUBJECT,
    WORKER_ACCOUNT,
    WORKER_INBOX,
    WORKER_USER,
)


MAX_LINE = 64 * 1024
MAX_PAYLOAD = 1024 * 1024


class ProbeError(RuntimeError):
    """A deliberately redacted probe failure."""


class AuthenticationRejected(ProbeError):
    """The broker explicitly rejected authentication or authorization."""


def _subjects(value: Any) -> list[str]:
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise ProbeError("invalid effective permissions")
    return sorted(value)


def validate_user_info(response: Any) -> None:
    if not isinstance(response, dict) or response.get("error") is not None:
        raise ProbeError("USER.INFO failed")
    data = response.get("data")
    if not isinstance(data, dict):
        raise ProbeError("USER.INFO data is absent")
    if data.get("user") != WORKER_USER or data.get("account") != WORKER_ACCOUNT:
        raise ProbeError("USER.INFO identity mismatch")
    permissions = data.get("permissions")
    if not isinstance(permissions, dict) or set(permissions) != {"publish", "subscribe"}:
        raise ProbeError("USER.INFO permissions mismatch")
    publish = permissions["publish"]
    subscribe = permissions["subscribe"]
    if not isinstance(publish, dict) or not isinstance(subscribe, dict):
        raise ProbeError("USER.INFO permissions mismatch")
    if set(publish) != {"allow", "deny"} or set(subscribe) != {"allow", "deny"}:
        raise ProbeError("USER.INFO permissions mismatch")
    if _subjects(publish["allow"]) != sorted([BUSINESS_SUBJECT, USER_INFO_SUBJECT]):
        raise ProbeError("effective publish allow mismatch")
    if _subjects(publish["deny"]) != [WORKER_INBOX]:
        raise ProbeError("effective publish deny mismatch")
    if _subjects(subscribe["allow"]) != [WORKER_INBOX]:
        raise ProbeError("effective subscribe allow mismatch")
    if _subjects(subscribe["deny"]) != sorted(
        [USER_INFO_SUBJECT, "$SYS.REQ.USER.*.INFO"]
    ):
        raise ProbeError("effective subscribe deny mismatch")


def _counter(value: Any, label: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ProbeError(f"invalid JetStream {label}")
    return value


def jetstream_signature(jsz: Any) -> tuple[Any, ...]:
    if not isinstance(jsz, dict):
        raise ProbeError("invalid JetStream monitoring response")
    total_fields = ("memory", "storage", "streams", "consumers", "messages", "bytes")
    totals = tuple(_counter(jsz.get(field), field) for field in total_fields)
    details = jsz.get("account_details")
    if not isinstance(details, list):
        raise ProbeError("JetStream account details are absent")
    accounts = []
    for detail in details:
        if not isinstance(detail, dict) or not isinstance(detail.get("name"), str):
            raise ProbeError("invalid JetStream account details")
        stream_detail = detail.get("stream_detail", [])
        if not isinstance(stream_detail, list):
            raise ProbeError("invalid JetStream stream details")
        accounts.append(
            (
                detail["name"],
                _counter(detail.get("memory"), "account memory"),
                _counter(detail.get("storage"), "account storage"),
                _counter(detail.get("reserved_memory"), "reserved memory"),
                _counter(detail.get("reserved_storage"), "reserved storage"),
                json.dumps(stream_detail, sort_keys=True, separators=(",", ":")),
            )
        )
    if {account[0] for account in accounts} != {WORKER_ACCOUNT}:
        raise ProbeError("JetStream account set does not match the reviewed fixture")
    return (totals, tuple(sorted(accounts)))


def read_jsz(url: str, timeout: float) -> Any:
    try:
        with urllib.request.urlopen(url.rstrip("/") + "/jsz?accounts=true&streams=true", timeout=timeout) as response:
            body = response.read(MAX_PAYLOAD + 1)
    except Exception as error:
        raise ProbeError("JetStream monitoring is unavailable") from error
    if len(body) > MAX_PAYLOAD:
        raise ProbeError("JetStream monitoring response is too large")
    try:
        return json.loads(body)
    except (UnicodeError, json.JSONDecodeError) as error:
        raise ProbeError("JetStream monitoring response is invalid") from error


class NATSClient:
    def __init__(
        self,
        host: str,
        port: int,
        timeout: float,
        user: Optional[str],
        password: Optional[str],
    ) -> None:
        opened_socket: Optional[socket.socket] = None
        try:
            opened_socket = socket.create_connection((host, port), timeout=timeout)
            self.socket = opened_socket
            self.socket.settimeout(timeout)
            self.reader: BinaryIO = self.socket.makefile("rb")
            if not self._read_line().startswith(b"INFO "):
                raise ProbeError("NATS greeting is invalid")
            options: dict[str, Any] = {
                "verbose": False,
                "pedantic": True,
                "name": "threadline-worker-permission-probe",
                "lang": "python",
                "version": "1",
                "protocol": 1,
            }
            if user is not None:
                options["user"] = user
            if password is not None:
                options["pass"] = password
            self._send(b"CONNECT " + json.dumps(options, separators=(",", ":")).encode("utf-8") + b"\r\n")
            self.flush()
        except Exception as error:
            reader = getattr(self, "reader", None)
            if reader is not None:
                reader.close()
            if opened_socket is not None:
                opened_socket.close()
            if isinstance(error, ProbeError):
                raise
            raise ProbeError("NATS connection failed") from error

    def __enter__(self) -> "NATSClient":
        return self

    def __exit__(self, *_: Any) -> None:
        self.close()

    def close(self) -> None:
        try:
            self.reader.close()
        finally:
            self.socket.close()

    def _read_line(self) -> bytes:
        line = self.reader.readline(MAX_LINE + 1)
        if len(line) > MAX_LINE or not line.endswith(b"\r\n"):
            raise ProbeError("NATS protocol line is invalid")
        return line[:-2]

    def _send(self, value: bytes) -> None:
        try:
            self.socket.sendall(value)
        except OSError as error:
            raise ProbeError("NATS write failed") from error

    def flush(self) -> None:
        self._send(b"PING\r\n")
        while True:
            line = self._read_line()
            if line == b"PONG":
                return
            if line.startswith(b"-ERR"):
                lowered = line.lower()
                if b"authorization violation" in lowered or b"authentication" in lowered:
                    raise AuthenticationRejected("NATS rejected the credentials")
                raise ProbeError("NATS rejected the operation")
            if line.startswith((b"+OK", b"INFO ")):
                continue
            raise ProbeError("unexpected NATS protocol response")

    def expect_subscription_denied(self, subject: str) -> None:
        self._send(f"SUB {subject} 1\r\nPING\r\n".encode("ascii"))
        self._expect_permission_error(b"subscription")

    def expect_publish_denied(self, subject: str) -> None:
        self._send(f"PUB {subject} 0\r\n\r\nPING\r\n".encode("ascii"))
        self._expect_permission_error(b"publish")

    def _expect_permission_error(self, operation: bytes) -> None:
        while True:
            line = self._read_line()
            lowered = line.lower()
            if line.startswith(b"-ERR"):
                if b"permissions violation" not in lowered or operation not in lowered:
                    raise ProbeError("NATS returned an unexpected error")
                return
            if line == b"PONG":
                raise ProbeError("forbidden NATS operation was accepted")
            if line.startswith((b"+OK", b"INFO ")):
                continue
            raise ProbeError("unexpected NATS protocol response")

    def user_info(self) -> Any:
        reply = "_INBOX.threadline.worker." + secrets.token_hex(16)
        self._send(
            f"SUB {reply} 1\r\nPUB {USER_INFO_SUBJECT} {reply} 0\r\n\r\nPING\r\n".encode("ascii")
        )
        payload: Optional[bytes] = None
        while True:
            line = self._read_line()
            if line.startswith(b"-ERR"):
                raise ProbeError("USER.INFO was rejected")
            if line.startswith(b"MSG "):
                parts = line.split()
                if (
                    len(parts) not in (4, 5)
                    or parts[1] != reply.encode("ascii")
                    or parts[2] != b"1"
                ):
                    raise ProbeError("unexpected NATS message")
                try:
                    size = int(parts[-1])
                except ValueError as error:
                    raise ProbeError("invalid NATS message size") from error
                if size < 0 or size > MAX_PAYLOAD:
                    raise ProbeError("NATS message is too large")
                payload = self.reader.read(size)
                if len(payload) != size or self.reader.read(2) != b"\r\n":
                    raise ProbeError("truncated NATS message")
                break
            if line == b"PONG":
                continue
            if line.startswith((b"+OK", b"INFO ")):
                continue
            raise ProbeError("unexpected NATS protocol response")
        if payload is None:
            raise ProbeError("USER.INFO response is absent")
        try:
            return json.loads(payload)
        except (UnicodeError, json.JSONDecodeError) as error:
            raise ProbeError("USER.INFO response is invalid") from error


def authentication_must_fail(host: str, port: int, timeout: float, user: Optional[str], password: Optional[str]) -> None:
    try:
        with NATSClient(host, port, timeout, user, password):
            pass
    except AuthenticationRejected:
        return
    raise ProbeError("invalid credentials were accepted")


def run(host: str, port: int, monitor_url: str, timeout: float, user: str, password: str) -> None:
    before = jetstream_signature(read_jsz(monitor_url, timeout))

    authentication_must_fail(host, port, timeout, None, None)
    authentication_must_fail(host, port, timeout, user + "-invalid", password)
    authentication_must_fail(host, port, timeout, user, password + "-invalid")

    with NATSClient(host, port, timeout, user, password) as client:
        validate_user_info(client.user_info())

    for subject in (USER_INFO_SUBJECT, "$SYS.REQ.USER.*.INFO", "$SYS.REQ.USER.>"):
        with NATSClient(host, port, timeout, user, password) as client:
            client.expect_subscription_denied(subject)

    with NATSClient(host, port, timeout, user, password) as client:
        client.expect_publish_denied("_INBOX.threadline.worker.probe")

    after = jetstream_signature(read_jsz(monitor_url, timeout))
    if after != before:
        raise ProbeError("JetStream state changed during the permission probe")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=4222)
    parser.add_argument("--monitor-url", default="http://127.0.0.1:8222")
    parser.add_argument("--timeout", type=float, default=5.0)
    args = parser.parse_args()
    user = os.environ.get("THREADLINE_NATS_WORKER_USER")
    password = os.environ.get("THREADLINE_NATS_WORKER_PASSWORD")
    if not user or not password or args.port < 1 or args.port > 65535 or args.timeout <= 0:
        print("NATS worker permission probe failed", file=sys.stderr)
        return 1
    try:
        run(args.host, args.port, args.monitor_url, args.timeout, user, password)
    except Exception:
        print("NATS worker permission probe failed", file=sys.stderr)
        return 1
    print("NATS worker permission probe passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
