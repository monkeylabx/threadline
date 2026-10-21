#!/usr/bin/env python3
"""Create-or-check the development domain Event Stream. Never update drift."""

import argparse
import os
import sys

from permissions import STREAM, SUBJECT, PROVISION_USER, PROVISION_PASSWORD
from probe import ProbeError
from request import Connection

CONFIG = {
    "name": STREAM, "subjects": [SUBJECT], "retention": "limits",
    "storage": "file", "num_replicas": 1, "max_msg_size": 327680,
    "duplicate_window": 120000000000, "no_ack": False,
}


def validate(value: dict) -> None:
    config = value.get("config")
    if not isinstance(config, dict) or value.get("error") is not None:
        raise ProbeError("Stream inspection failed")
    if any(config.get(key, False if key == "no_ack" else None) != expected for key, expected in CONFIG.items()):
        raise ProbeError("Stream configuration drift")
    if config.get("sealed", False) or config.get("deny_delete", False):
        raise ProbeError("Stream configuration drift")


def provision(connection: Connection) -> None:
    current = connection.request(f"$JS.API.STREAM.INFO.{STREAM}", {})
    error = current.get("error")
    if error is not None:
        if not isinstance(error, dict) or error.get("err_code") != 10059:
            raise ProbeError("Stream inspection failed")
        # CREATE with an exact existing config is idempotent if another actor won.
        validate(connection.request(f"$JS.API.STREAM.CREATE.{STREAM}", CONFIG))
    else:
        validate(current)
    validate(connection.request(f"$JS.API.STREAM.INFO.{STREAM}", {}))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=4222)
    args = parser.parse_args()
    connection = Connection(args.host, args.port,
                            os.environ.get("THREADLINE_NATS_PLATFORM_USER", PROVISION_USER),
                            os.environ.get("THREADLINE_NATS_PLATFORM_PASSWORD", PROVISION_PASSWORD))
    try:
        provision(connection)
    except (ProbeError, OSError):
        print("NATS Stream provisioning failed", file=sys.stderr)
        return 1
    print("NATS domain Event Stream ready (development only)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
