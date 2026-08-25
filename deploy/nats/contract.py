#!/usr/bin/env python3
"""Fail-closed validation for the restricted Threadline NATS fixture."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Iterable, List, Tuple


SYSTEM_ACCOUNT = "SYS"
WORKER_ACCOUNT = "THREADLINE_WORKER"
WORKER_USER = "threadline_worker_dev"
BUSINESS_SUBJECT = "threadline.domain.events.v1"
USER_INFO_SUBJECT = "$SYS.REQ.USER.INFO"
WORKER_INBOX = "_INBOX.threadline.worker.>"


class ContractError(ValueError):
    """A safe, non-secret-bearing contract failure."""


def _object(pairs: Iterable[Tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ContractError("duplicate JSON key")
        result[key] = value
    return result


def load_config(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_object)
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise ContractError("NATS fixture config is unreadable") from error
    if not isinstance(value, dict):
        raise ContractError("NATS fixture config must be an object")
    return value


def _exact_keys(value: Any, expected: set[str], label: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != expected:
        raise ContractError(f"{label} keys do not match the reviewed contract")
    return value


def _exact_subjects(value: Any, expected: List[str], label: str) -> None:
    if value != expected:
        raise ContractError(f"{label} subjects do not match the reviewed contract")


def validate_config(config: dict[str, Any]) -> None:
    _exact_keys(
        config,
        {"server_name", "port", "http_port", "jetstream", "system_account", "accounts"},
        "top-level config",
    )
    if config["server_name"] != "threadline-dev" or config["port"] != 4222:
        raise ContractError("server identity or client port changed")
    if config["http_port"] != 8222:
        raise ContractError("monitor port changed")
    if config["system_account"] != SYSTEM_ACCOUNT:
        raise ContractError("system account changed")
    if config["jetstream"] != {"store_dir": "/data"}:
        raise ContractError("JetStream storage contract changed")

    accounts = _exact_keys(config["accounts"], {SYSTEM_ACCOUNT, WORKER_ACCOUNT}, "accounts")
    if accounts[SYSTEM_ACCOUNT] != {}:
        raise ContractError("SYS must not expose an authenticatable principal")

    worker = _exact_keys(accounts[WORKER_ACCOUNT], {"jetstream", "users"}, "worker account")
    if worker["jetstream"] is not True:
        raise ContractError("JetStream must be enabled for the worker account")
    users = worker["users"]
    if not isinstance(users, list) or len(users) != 1:
        raise ContractError("the worker account must enumerate exactly one principal")

    user = _exact_keys(users[0], {"user", "password", "permissions"}, "worker principal")
    if user["user"] != WORKER_USER:
        raise ContractError("worker principal changed")
    password = user["password"]
    if not isinstance(password, str) or not password.startswith(("$2a$", "$2b$", "$2y$")):
        raise ContractError("worker password must be bcrypt-hashed")

    permissions = _exact_keys(user["permissions"], {"publish", "subscribe"}, "permissions")
    publish = _exact_keys(permissions["publish"], {"allow", "deny"}, "publish permissions")
    subscribe = _exact_keys(permissions["subscribe"], {"allow", "deny"}, "subscribe permissions")
    _exact_subjects(publish["allow"], [BUSINESS_SUBJECT, USER_INFO_SUBJECT], "publish allow")
    _exact_subjects(publish["deny"], [WORKER_INBOX], "publish deny")
    _exact_subjects(subscribe["allow"], [WORKER_INBOX], "subscribe allow")
    _exact_subjects(
        subscribe["deny"],
        [USER_INFO_SUBJECT, "$SYS.REQ.USER.*.INFO"],
        "subscribe deny",
    )


def verify_file(path: Path) -> None:
    validate_config(load_config(path))
