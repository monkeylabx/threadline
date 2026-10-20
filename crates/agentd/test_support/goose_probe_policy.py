"""Opt-in synthetic probe policy. Never used by the production daemon."""
import hashlib
import json
from pathlib import Path
from urllib.parse import urlsplit

BINARY_SHA256 = "b7a38f3ff1e2d48b34b6550f84cf4104f3f7d797429e1231d1255d0501d47702"
ARCHIVE_SHA256 = "7674b0124aab685c71f8782fb7e65bac100c736ce3de0c9d3bf46ba07910e412"
EXTENSIONS = ("analyze", "apps", "extensionmanager", "scheduler", "summon",
              "tom", "skills", "developer")


def validate_origin(endpoint):
    url = urlsplit(endpoint)
    if (url.scheme != "http" or url.hostname != "127.0.0.1" or not url.port
            or url.path or url.query or url.fragment or url.username is not None):
        raise ValueError("synthetic endpoint must be a bare IPv4 loopback origin")


def validate_route(route):
    validate_origin(route["endpoint"])
    if route["synthetic"] is not True or route["credential"] != "synthetic-probe-only":
        raise ValueError("only the explicit synthetic credential is accepted")
    if (not isinstance(route["model"], str) or not route["model"].strip()
            or len(route["model"]) > 128 or "\0" in route["model"]):
        raise ValueError("routing policy must supply a model")
    return route


def load_policy(policy_path, binary):
    route = validate_route(json.loads(Path(policy_path).read_text()))
    if hashlib.sha256(Path(binary).read_bytes()).hexdigest() != BINARY_SHA256:
        raise ValueError("pinned Goose binary digest mismatch")
    return route


def environment(work, route):
    return {"PATH": "/usr/bin:/bin", "HOME": str(work), "TMPDIR": str(work),
            "GOOSE_PATH_ROOT": str(work), "GOOSE_PROVIDER": "openai",
            "GOOSE_MODEL": route["model"], "OPENAI_HOST": route["endpoint"],
            "OPENAI_API_KEY": route["credential"], "GOOSE_MODE": "approve",
            "OPENAI_BASE_PATH": "v1/chat/completions",
            "GOOSE_DISABLE_KEYRING": "1", "NO_PROXY": "127.0.0.1,::1,localhost",
            "no_proxy": "127.0.0.1,::1,localhost", "TERM": "dumb"}


def write_config(work, developer=False):
    config = work / "config"
    config.mkdir(parents=True)
    extensions = {name: {"enabled": developer and name == "developer",
                         "type": "platform", "name": name}
                  for name in EXTENSIONS}
    (config / "config.yaml").write_text(json.dumps({
        "GOOSE_MODE": "auto" if developer else "approve",
        "GOOSE_DISABLE_SESSION_NAMING": True, "extensions": extensions}))


def profile(work, binary, route):
    # JSON string escaping is also valid for these Seatbelt string literals.
    paths = " ".join('(subpath ' + json.dumps(p) + ')' for p in
                     ("/System", "/usr", "/bin", "/sbin", "/dev", str(work)))
    port = urlsplit(route["endpoint"]).port
    return ('(version 1) (deny default)\n'
            '(allow process* sysctl-read mach-lookup file-read-metadata)\n'
            '(allow signal (target children))\n'
            '(allow file-read* (literal "/") (literal "/private/etc/ssl/openssl.cnf") ' + paths +
            ' (literal ' + json.dumps(str(binary)) + '))\n'
            '(allow file-write* (subpath ' + json.dumps(str(work)) + '))\n'
            '(allow file-write* (literal "/dev/null"))\n'
            '(allow network-outbound (remote ip "localhost:' + str(port) + '"))\n')


def launch(work, binary, route, developer=False):
    work.mkdir(mode=0o700)
    (work / "data" / "sessions").mkdir(parents=True)
    write_config(work, developer)
    sandbox = work / "worker.sb"
    sandbox.write_text(profile(work, binary, route))
    return ["/usr/bin/sandbox-exec", "-f", str(sandbox), str(binary), "acp"]
