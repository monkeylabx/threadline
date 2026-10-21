"""Bounded request/reply helper for platform fixture operations."""

import json
import secrets
from dataclasses import dataclass

from probe import MAX_PAYLOAD, NATSClient, ProbeError


@dataclass(frozen=True)
class Connection:
    host: str
    port: int
    user: str
    password: str

    def request(self, subject: str, value: dict) -> dict:
        inbox = "_INBOX.threadline.platform." + secrets.token_hex(16)
        payload = json.dumps(value, separators=(",", ":")).encode()
        with NATSClient(self.host, self.port, 5, self.user, self.password) as client:
            client._send(f"SUB {inbox} 1\r\n".encode())
            client._send(f"PUB {subject} {inbox} {len(payload)}\r\n".encode() + payload + b"\r\n")
            return response(client, inbox)


def response(client: NATSClient, inbox: str) -> dict:
    for _ in range(16):
        line = client._read_line()
        if line.startswith((b"+OK", b"INFO ", b"PONG")):
            continue
        size = reply_size(line, inbox)
        return reply_object(client, size)

    raise ProbeError("platform reply budget exceeded")


def reply_size(line: bytes, inbox: str) -> int:
    parts = line.split()
    if len(parts) != 4 or parts[:3] != [b"MSG", inbox.encode(), b"1"]:
        raise ProbeError("invalid platform reply")
    try:
        size = int(parts[3])
    except ValueError as error:
        raise ProbeError("invalid platform size") from error
    if size < 0 or size > MAX_PAYLOAD:
        raise ProbeError("invalid platform size")
    return size


def reply_object(client: NATSClient, size: int) -> dict:
    payload = client.reader.read(size)
    if len(payload) != size or client.reader.read(2) != b"\r\n":
        raise ProbeError("truncated platform reply")
    try:
        value = json.loads(payload)
    except (ValueError, UnicodeError) as error:
        raise ProbeError("invalid platform JSON") from error
    if not isinstance(value, dict):
        raise ProbeError("invalid platform object")
    return value
