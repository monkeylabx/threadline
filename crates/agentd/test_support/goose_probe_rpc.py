"""Deadline/frame bounded test-only ACP observer; never grants tool callbacks."""
import json
import os
import selectors
import signal
import subprocess
import time


class Worker:
    def __init__(self, command, env, work):
        self.log = (work / "stderr.log").open("wb")
        self.child = subprocess.Popen(command, env=env, cwd=work, start_new_session=True,
                                      stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                      stderr=self.log)
        self.buffer = b""
        self.frames = []
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.child.stdout, selectors.EVENT_READ)

    def send(self, method, params, request_id=None):
        frame = {"jsonrpc": "2.0", "method": method, "params": params}
        if request_id is not None:
            frame["id"] = request_id
        self.child.stdin.write(json.dumps(frame).encode() + b"\n")
        self.child.stdin.flush()

    def receive(self, deadline):
        while b"\n" not in self.buffer:
            if not self.selector.select(max(0, deadline - time.monotonic())):
                raise TimeoutError("ACP receive deadline")
            block = os.read(self.child.stdout.fileno(), 4096)
            if not block:
                raise RuntimeError("unexpected worker EOF")
            self.buffer += block
            if len(self.buffer) > 262144:
                raise ValueError("ACP frame budget exceeded")
        line, self.buffer = self.buffer.split(b"\n", 1)
        frame = json.loads(line)
        self.frames.append(frame)
        return frame

    def response(self, request_id):
        deadline = time.monotonic() + 20
        for _ in range(128):
            frame = self.receive(deadline)
            if frame.get("id") == request_id:
                if "error" in frame:
                    raise RuntimeError("ACP remote error: " + str(frame["error"]))
                return frame["result"]
            if frame.get("method") != "session/update" or "id" in frame:
                raise ValueError("unexpected ACP message or forbidden callback")
        raise ValueError("ACP notification budget exceeded")

    def session(self, work):
        self.send("initialize", {"protocolVersion": 1, "clientCapabilities": {}}, 1)
        if self.response(1)["protocolVersion"] != 1:
            raise ValueError("protocol mismatch")
        self.send("session/new", {"cwd": str(work), "mcpServers": []}, 2)
        result = self.response(2)
        self.session_id = result["sessionId"]
        return result

    def prompt(self):
        self.send("session/prompt", {"sessionId": self.session_id, "prompt": [
            {"type": "text", "text": "synthetic fixture"}]}, 3)

    def cancel(self):
        self.send("session/cancel", {"sessionId": self.session_id})
        return self.response(3)

    def close(self):
        # Experimental same-group cleanup, explicitly not SealedAndEmpty.
        try:
            os.killpg(self.child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass  # An already absent group is followed by the retained child wait.
        return self.finish()

    def finish(self):
        try:
            return self.child.wait(timeout=10)
        finally:
            self.child.stdin.close()
            self.child.stdout.close()
            self.selector.close()
            self.log.close()
