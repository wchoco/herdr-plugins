#!/usr/bin/env python3
"""Capture pane.layout request/response pairs from a running herdr server.

Usage: capture-layout.py <pane-id>

Run it through scripts/throwaway-herdr (see ../../README.md); <pane-id> is a pane in
a tab split three ways (`pane split <p1> --direction right`, then
`pane split <p2> --direction down`), with no overlay open.
"""
import json
import os
import socket
import sys

here = os.path.dirname(os.path.abspath(__file__))
out = os.path.join(os.path.dirname(here), "ndjson")
if "THROWAWAY_HERDR_DIR" not in os.environ:
    sys.exit("run this through scripts/throwaway-herdr")
sock = os.path.join(os.environ["XDG_CONFIG_HOME"], "herdr", "herdr.sock")
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


captures = {
    "pane-layout-split3": {"id": "1", "method": "pane.layout", "params": {"pane_id": pane}},
    "pane-layout-not-found": {"id": "1", "method": "pane.layout", "params": {"pane_id": "w9:p99"}},
}
os.makedirs(out, exist_ok=True)
for name, req in captures.items():
    line, resp = call(req)
    with open(os.path.join(out, name + ".ndjson"), "w") as f:
        f.write(line + resp)
