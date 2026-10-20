"""Bounded, loopback-only OpenAI fixture; credentials come from the route file."""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from threading import Thread
from urllib.parse import urlsplit


def completion(body, command):
    tools = body.get("tools", [])
    advertised = [tool["function"]["name"] for tool in tools]
    if not command or any(m["role"] == "tool" for m in body["messages"]):
        return {"content": "synthetic probe complete"}, "stop", advertised
    name = next((name for name in advertised if name.endswith("shell")), "shell")
    call = {"index": 0, "id": "probe-call", "type": "function",
            "function": {"name": name, "arguments": json.dumps({"command": command})}}
    return {"tool_calls": [call]}, "tool_calls", advertised


class Endpoint:
    def __init__(self, route):
        self.route = route
        self.command = None
        self.requests = []
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                return

            def do_GET(self):
                self.connection.settimeout(10)
                body = (b"synthetic healthy" if self.path == "/health" else
                        json.dumps({"data": [{"id": route["model"], "object": "model"}]}).encode())
                self.send_response(200)
                self.end_headers()
                self.wfile.write(body)

            def do_POST(self):
                self.connection.settimeout(10)
                if self.headers.get("Authorization") != "Bearer " + route["credential"]:
                    self.send_error(401)
                    return
                length = int(self.headers.get("Content-Length", "0"))
                if self.path != "/v1/chat/completions" or not 0 < length <= 1048576:
                    self.send_error(400)
                    return
                body = json.loads(self.rfile.read(length))
                if (body.get("model") != route["model"] or body.get("stream") is not True
                        or len(owner.requests) >= 8):
                    self.send_error(400)
                    return
                delta, reason, tools = completion(body, owner.command)
                owner.requests.append({"tools": tools, "messages": body["messages"]})
                chunks = [{"role": "assistant"}, delta, {}]
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()
                for index, chunk in enumerate(chunks):
                    payload = {"id": "probe", "object": "chat.completion.chunk",
                               "created": 0, "model": route["model"], "choices": [{
                                   "index": 0, "delta": chunk,
                                   "finish_reason": reason if index == 2 else None}]}
                    self.wfile.write(("data: " + json.dumps(payload) + "\n\n").encode())
                self.wfile.write(b"data: [DONE]\n\n")

        self.server = HTTPServer(("127.0.0.1", urlsplit(route["endpoint"]).port), Handler)
        self.server.timeout = 10
        self.thread = Thread(target=self.server.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_args):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=10)
        if self.thread.is_alive():
            raise RuntimeError("synthetic endpoint did not stop")
