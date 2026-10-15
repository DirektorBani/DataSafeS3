"""Webhook sink for the cluster KMS gate. One JSON body per line."""
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PATH = os.environ.get("HOOK_PATH", "/data/events.jsonl")
os.makedirs(os.path.dirname(PATH), exist_ok=True)


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0") or 0)
        body = self.rfile.read(n)
        with open(PATH, "ab") as fh:
            fh.write(body + b"\n")
        self.send_response(204)
        self.end_headers()

    def log_message(self, fmt, *args):
        return


ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
