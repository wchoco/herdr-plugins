#!/usr/bin/env python3
"""Capture the socket exchanges behind yubi's re-launch decision.

Usage: capture-relaunch.py <pane-id> <overlay-pane-id>

Run it against a throwaway server (see ../../README.md) with
HERDR_SOCKET_PATH unset or pointing at that server; <pane-id> is any
pane and <overlay-pane-id> an open yubi overlay, which this script closes.
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
pane, overlay = sys.argv[1], sys.argv[2]


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


# Ordered: the overlay is closed before the second close is captured.
captures = [
    ("pane-get", {"id": "1", "method": "pane.get", "params": {"pane_id": pane}}),
    ("pane-get-not-found", {"id": "1", "method": "pane.get", "params": {"pane_id": "w9:p99"}}),
    ("plugin-pane-close", {"id": "1", "method": "plugin.pane.close", "params": {"pane_id": overlay}}),
    ("plugin-pane-close-not-found", {"id": "1", "method": "plugin.pane.close", "params": {"pane_id": overlay}}),
    ("pane-zoom", {"id": "1", "method": "pane.zoom", "params": {"pane_id": pane, "mode": "on"}}),
    ("pane-zoom-not-found", {"id": "1", "method": "pane.zoom", "params": {"pane_id": "w9:p99", "mode": "on"}}),
]
os.makedirs(out, exist_ok=True)
for name, req in captures:
    line, resp = call(req)
    with open(os.path.join(out, name + ".ndjson"), "w") as f:
        f.write(line + resp)
