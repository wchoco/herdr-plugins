#!/usr/bin/env python3
"""Capture the socket exchanges behind yubi's Paste Copy.

Usage: capture-paste.py <pane-id>

Run it against a throwaway server (see ../../README.md) with
HERDR_SOCKET_PATH unset or pointing at that server; <pane-id> is a pane
showing a shell prompt, which receives the pasted text (no Enter is sent).
Each output file holds the request line, then the response line, exactly
as sent and received.
"""
import json
import os
import socket
import sys

here = os.path.dirname(os.path.abspath(__file__))
out = os.path.join(os.path.dirname(here), "ndjson")
sock = os.environ.get("HERDR_SOCKET_PATH") or os.path.join(
    os.environ.get("XDG_CONFIG_HOME", os.path.expanduser("~/.config")), "herdr", "herdr.sock")
pane = sys.argv[1]


def call(req):
    s = socket.socket(socket.AF_UNIX)
    s.connect(sock)
    line = json.dumps(req, separators=(",", ":")) + "\n"
    s.sendall(line.encode())
    buf = b""
    while not buf.endswith(b"\n"):
        c = s.recv(65536)
        if not c:
            break
        buf += c
    s.close()
    return line, buf.decode()


captures = [
    ("pane-send-input", {"id": "1", "method": "pane.send_input",
                         "params": {"pane_id": pane, "text": "echo pasted\nsecond line"}}),
    ("pane-send-input-not-found", {"id": "1", "method": "pane.send_input",
                                   "params": {"pane_id": "w9:p99", "text": "x"}}),
]
os.makedirs(out, exist_ok=True)
for name, req in captures:
    line, resp = call(req)
    with open(os.path.join(out, name + ".ndjson"), "w") as f:
        f.write(line + resp)
