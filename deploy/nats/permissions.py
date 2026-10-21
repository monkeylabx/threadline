"""Exact development-only capabilities for the domain Event transport."""

STREAM = "THREADLINE_DOMAIN_EVENTS_V1"
SUBJECT = "threadline.domain.events.v1"
INSPECT = ["$JS.API.INFO", "$JS.API.STREAM.NAMES", f"$JS.API.STREAM.INFO.{STREAM}"]
WORKER_PUBLISH = [SUBJECT, "$SYS.REQ.USER.INFO", *INSPECT]
PROVISION_USER = "threadline_platform_dev"
PROVISION_PASSWORD = "threadline-worker-dev-only"  # Public dev fixture only.
PROVISION_INBOX = "_INBOX.threadline.platform.>"
PROVISION_PUBLISH = [f"$JS.API.STREAM.CREATE.{STREAM}", f"$JS.API.STREAM.INFO.{STREAM}"]

REALTIME_USER = "threadline_realtime_dev"
REALTIME_PERMISSIONS = {
    "publish": {"deny": [">"]},
    "subscribe": {"allow": [SUBJECT], "deny": ["$SYS.>"]},
}
