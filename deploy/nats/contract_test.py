#!/usr/bin/env python3
"""Static contract tests for the restricted Threadline NATS fixture."""

from __future__ import annotations

import copy
import re
import textwrap
import unittest
from pathlib import Path

from contract import ContractError, load_config, validate_config


ROOT = Path(__file__).resolve().parents[2]
CONFIG = ROOT / "deploy" / "nats" / "nats-server.conf"


class NATSAccountContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.config = load_config(CONFIG)

    def test_only_system_and_worker_accounts_are_declared(self) -> None:
        validate_config(self.config)

    def test_rejects_an_external_system_principal(self) -> None:
        changed = copy.deepcopy(self.config)
        changed["accounts"]["SYS"]["users"] = [{"user": "external", "password": "secret"}]
        with self.assertRaisesRegex(ContractError, "SYS"):
            validate_config(changed)

    def test_rejects_an_additional_worker_principal(self) -> None:
        changed = copy.deepcopy(self.config)
        changed["accounts"]["THREADLINE_WORKER"]["users"].append(
            copy.deepcopy(changed["accounts"]["THREADLINE_WORKER"]["users"][0])
        )
        with self.assertRaisesRegex(ContractError, "exactly one"):
            validate_config(changed)

    def test_rejects_broader_subscription_permissions(self) -> None:
        changed = copy.deepcopy(self.config)
        changed["accounts"]["THREADLINE_WORKER"]["users"][0]["permissions"]["subscribe"]["allow"] = [">"]
        with self.assertRaisesRegex(ContractError, "subscribe allow"):
            validate_config(changed)

    def test_rejects_missing_user_info_denial(self) -> None:
        changed = copy.deepcopy(self.config)
        changed["accounts"]["THREADLINE_WORKER"]["users"][0]["permissions"]["subscribe"]["deny"] = []
        with self.assertRaisesRegex(ContractError, "subscribe deny"):
            validate_config(changed)

    def test_rejects_imports_or_exports(self) -> None:
        for key in ("imports", "exports"):
            with self.subTest(key=key):
                changed = copy.deepcopy(self.config)
                changed["accounts"]["THREADLINE_WORKER"][key] = []
                with self.assertRaisesRegex(ContractError, "worker account"):
                    validate_config(changed)

    def test_rejects_plaintext_password(self) -> None:
        changed = copy.deepcopy(self.config)
        changed["accounts"]["THREADLINE_WORKER"]["users"][0]["password"] = "secret"
        with self.assertRaisesRegex(ContractError, "bcrypt"):
            validate_config(changed)


class NATSDeploymentContractTest(unittest.TestCase):
    def test_compose_mounts_the_reviewed_config_read_only(self) -> None:
        compose = (ROOT / "deploy" / "compose" / "compose.yaml").read_text(encoding="utf-8")
        self.assertIn("../nats/nats-server.conf:/etc/nats/nats-server.conf:ro", compose)
        self.assertIn("--config=/etc/nats/nats-server.conf", compose)
        self.assertNotIn("--user=", compose)
        self.assertNotIn("--pass=", compose)

    def test_kind_embeds_the_exact_reviewed_config(self) -> None:
        manifest = (ROOT / "deploy" / "kind" / "stack.yaml").read_text(encoding="utf-8")
        match = re.search(
            r"name: nats-server-config\n(?:[^\n]*\n)*?  nats-server\.conf: \|\n(?P<body>(?: {4}[^\n]*\n)+)",
            manifest,
        )
        self.assertIsNotNone(match, "nats-server-config ConfigMap is missing")
        embedded = textwrap.dedent(match.group("body"))
        self.assertEqual(embedded, CONFIG.read_text(encoding="utf-8"))

    def test_kind_exposes_worker_credentials_to_no_workload_yet(self) -> None:
        manifest = (ROOT / "deploy" / "kind" / "stack.yaml").read_text(encoding="utf-8")
        self.assertEqual(manifest.count("name: nats-worker-dev-only-credentials"), 1)
        self.assertNotIn("name: nats-dev-only-credentials", manifest)

    def test_kind_has_exact_worker_to_nats_network_path(self) -> None:
        manifest = (ROOT / "deploy" / "kind" / "stack.yaml").read_text(encoding="utf-8")
        self.assertIn("name: allow-worker-to-nats", manifest)
        self.assertIn("name: allow-nats-from-worker", manifest)
        self.assertGreaterEqual(manifest.count("threadline.io/nats-principal: worker"), 2)

    def test_up_paths_run_the_permission_probe(self) -> None:
        compose_make = (ROOT / "deploy" / "compose" / "Makefile").read_text(encoding="utf-8")
        kind_make = (ROOT / "deploy" / "kind" / "Makefile").read_text(encoding="utf-8")
        for makefile in (compose_make, kind_make):
            self.assertIn("verify-nats-config", makefile)
            self.assertIn("verify-nats-worker", makefile)
            self.assertIn("probe.py", makefile)
        self.assertIn("get secret nats-worker-dev-only-credentials", kind_make)


if __name__ == "__main__":
    unittest.main()
