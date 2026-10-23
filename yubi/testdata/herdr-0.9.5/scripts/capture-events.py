#!/usr/bin/env python3
"""Capture what yubi's events.subscribe connection sees for each session
change that should (or should not) end the overlay.

Usage: capture-events.py

Run it against a throwaway server (see ../../README.md) with the probe
plugin linked:

    herdr plugin link yubi/testdata/herdr-0.9.5/scripts/probe-plugin

For each scenario the script builds a fresh workspace: Target Pane T with a
sibling S to its right, then opens the probe overlay O over T (zoomed and
focused, like yubi). It then subscribes exactly as yubi does, makes one
change and records every line the subscription receives. Each output file
ndjson/events-<scenario>.ndjson holds the subscribe request line, the
response line and the pushed event lines, exactly as sent and received;
events-<scenario>.ids.json names the scenario's panes, tab and workspace.
"""
import json
import os
import select
import socket
import time

here = os.path.dirname(os.path.abspath(__file__))
out = os.path.join(os.path.dirname(here), "ndjson")
sock = os.environ.get("HERDR_SOCKET_PATH") or os.path.join(
    os.environ.get("XDG_CONFIG_HOME", os.path.expanduser("~/.config")), "herdr", "herdr.sock")

# yubi's subscription, in yubi's order.
SUBSCRIBE = {"id": "1", "method": "events.subscribe", "params": {"subscriptions": [
    {"type": t} for t in ["pane.closed", "pane.exited", "pane.moved", "pane.focused",
                          "tab.focused", "workspace.focused", "layout.updated"]]}}


def call(method, params):
    s = socket.socket(socket.AF_UNIX)
    s.connect(sock)
    s.sendall((json.dumps({"id": "c", "method": method, "params": params}) + "\n").encode())
    buf = b""
    while not buf.endswith(b"\n"):
        c = s.recv(65536)
        if not c:
            break
        buf += c
    s.close()
    resp = json.loads(buf)
    if "error" in resp:
        raise RuntimeError(f"{method}: {resp['error']}")
    return resp["result"]


def setup():
    ws = call("workspace.create", {"focus": True})
    t = ws["root_pane"]["pane_id"]
    s = call("pane.split", {"target_pane_id": t, "direction": "right"})["pane"]["pane_id"]
    o = call("plugin.pane.open", {"plugin_id": "wchoco.yubi-probe", "entrypoint": "ui",
                                  "focus": True})["plugin_pane"]["pane"]["pane_id"]
    time.sleep(0.5)
    return {"workspace": ws["workspace"]["workspace_id"], "tab": ws["tab"]["tab_id"],
            "target": t, "sibling": s, "overlay": o}


def record(name, ids, change, req=SUBSCRIBE):
    s = socket.socket(socket.AF_UNIX)
    s.connect(sock)
    line = json.dumps(req, separators=(",", ":")) + "\n"
    s.sendall(line.encode())
    buf = b""
    while b"\n" not in buf:
        c = s.recv(65536)
        if not c:
            break
        buf += c
    if change:
        change(ids)
    # Collect until the stream has been quiet for a second.
    while True:
        r, _, _ = select.select([s], [], [], 1.0)
        if not r:
            break
        c = s.recv(65536)
        if not c:
            break
        buf += c
    s.close()
    with open(os.path.join(out, "events-" + name + ".ndjson"), "w") as f:
        f.write(line + buf.decode())
    if ids:
        with open(os.path.join(out, "events-" + name + ".ids.json"), "w") as f:
            f.write(json.dumps(ids, indent=2) + "\n")


scenarios = {
    # Should end the overlay.
    "target-closed": lambda i: call("pane.close", {"pane_id": i["target"]}),
    "target-exited": lambda i: call("pane.send_input", {"pane_id": i["target"], "text": "exit", "keys": ["Enter"]}),
    # herdr refuses to move a pane out of a zoomed tab (reason "zoomed_tab"),
    # so the tab has to lose its zoom first.
    "target-moved": lambda i: (call("pane.zoom", {"pane_id": i["overlay"], "mode": "off"}),
                               call("pane.move", {"pane_id": i["target"],
                                                  "destination": {"type": "new_workspace"}})),
    "focus-target": lambda i: call("pane.focus", {"pane_id": i["target"]}),
    "tab-focused": lambda i: call("tab.create", {"workspace_id": i["workspace"], "focus": True}),
    "workspace-focused": lambda i: call("workspace.create", {"focus": True}),
    "sibling-split": lambda i: call("pane.split", {"target_pane_id": i["sibling"], "direction": "down"}),
    # Should not end the overlay.
    "split-resized": lambda i: call("pane.resize", {"pane_id": i["target"], "direction": "left", "amount": 0.1}),
}

os.makedirs(out, exist_ok=True)
for name, change in scenarios.items():
    ids = setup()
    record(name, ids, change)
    for w in call("workspace.list", {})["workspaces"]:
        call("workspace.close", {"workspace_id": w["workspace_id"]})

# A subscription naming a pane that does not exist is rejected.
rejected = {"id": "1", "method": "events.subscribe", "params": {"subscriptions": [
    {"type": "pane.scroll_changed", "pane_id": "w9:p99"}]}}
record("subscribe-rejected", {}, None, rejected)
