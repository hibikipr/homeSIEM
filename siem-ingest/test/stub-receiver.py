#!/usr/bin/env python3
"""Stub HTTP receiver standing in for siem-api's /ingest/fastpath and
/sources/heartbeat during local pipeline verification. Logs every request's
path, headers, and JSON body to stdout as one line of JSON, so a test script
can grep the container's logs to assert on what Vector actually sent."""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length) if length else b""
        try:
            parsed_body = json.loads(body) if body else None
        except json.JSONDecodeError:
            parsed_body = None
        record = {
            "path": self.path,
            "fastpath_token": self.headers.get("X-Fastpath-Token"),
            "body": parsed_body,
        }
        print(json.dumps(record), flush=True)
        self.send_response(202)
        self.end_headers()

    def log_message(self, format, *args):
        pass  # suppress default request logging; the JSON line above is what matters


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8080
    HTTPServer(("0.0.0.0", port), Handler).serve_forever()
