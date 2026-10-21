"""Real pinned-process boundary tests; enable with THREADLINE_TEST_NATS_BINARY."""

import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import unittest

from permissions import PROVISION_USER, PROVISION_PASSWORD, STREAM
from probe import ProbeError
from provision import CONFIG, provision
from request import Connection


def available_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


@unittest.skipUnless(os.environ.get("THREADLINE_TEST_NATS_BINARY"), "real NATS binary not selected")
class StreamProcessTest(unittest.TestCase):
    def setUp(self):
        binary = os.environ["THREADLINE_TEST_NATS_BINARY"]
        version = subprocess.check_output([binary, "--version"], text=True)
        self.assertIn("v2.10.20", version)
        directory = tempfile.TemporaryDirectory(prefix="threadline-domain-stream-")
        self.addCleanup(directory.cleanup)
        config = json.loads(Path(__file__).with_name("nats-server.conf").read_text())
        port = available_port()
        config.update({"host": "127.0.0.1", "port": port, "http_port": available_port()})
        config["jetstream"]["store_dir"] = directory.name + "/data"
        path = Path(directory.name) / "nats.conf"
        path.write_text(json.dumps(config))
        process = subprocess.Popen([binary, "-c", str(path)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(self.stop, process)
        self.client = Connection("127.0.0.1", port, PROVISION_USER, PROVISION_PASSWORD)
        self.wait_ready(port)

    @staticmethod
    def stop(process):
        process.terminate()
        process.wait(timeout=5)

    @staticmethod
    def wait_ready(port):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.1):
                    return
            except OSError:
                time.sleep(0.02)
        raise AssertionError("NATS did not become ready")

    def test_real_create_and_idempotent_recheck(self):
        provision(self.client)
        provision(self.client)
        result = self.client.request(f"$JS.API.STREAM.INFO.{STREAM}", {})
        self.assertEqual(result["state"]["messages"], 0)
        self.assertEqual(result["config"]["duplicate_window"], CONFIG["duplicate_window"])

    def test_real_drift_is_rejected_without_mutation(self):
        changed = {**CONFIG, "duplicate_window": 1000000000}
        self.client.request(f"$JS.API.STREAM.CREATE.{STREAM}", changed)
        with self.assertRaises(ProbeError):
            provision(self.client)
        result = self.client.request(f"$JS.API.STREAM.INFO.{STREAM}", {})
        self.assertEqual(result["config"]["duplicate_window"], changed["duplicate_window"])


if __name__ == "__main__":
    unittest.main()
