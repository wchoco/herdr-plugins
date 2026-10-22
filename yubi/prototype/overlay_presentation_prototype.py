#!/usr/bin/env python3
"""PROTOTYPE -- throwaway. Answers "Overlay presentation" (wchoco/herdr-plugins#9).

Three structurally different presentations of yubi's Hint Mode and Line Mode,
rendered in the terminal so they can be judged at real cell density.

  python3 yubi/prototype/overlay_presentation_prototype.py            # canned screen
  python3 yubi/prototype/overlay_presentation_prototype.py --pane ID  # real herdr pane

Keys:  <- / ->  switch variant      Tab  switch Hint Mode / Line Mode
       Line Mode: digits , - Backspace Enter      Hint Mode: letters (Uppercase = copy+paste)
       Esc  clear input (or quit when input is empty)      Ctrl-C  quit

Nothing is copied; the "would copy" text is shown in the state line instead.
"""
import os
import re
import shutil
import subprocess
import sys
import termios
import tty
import unicodedata

# ---------------------------------------------------------------- screen source

CANNED = """\
~/dev/herdr-plugins main > git log --oneline -3
92753cb docs(yubi): add glossary terms for yubi (#12)
17c2f34 docs(yubi): add glossary terms for yubi
72b0700 feat: add merge skill for gated PR merges (#2)
~/dev/herdr-plugins main > go test ./yubi/...
--- FAIL: TestNumbering (0.00s)
    numbering_test.go:42: got 3, want 4
FAIL	github.com/wchoco/herdr-plugins/yubi	0.012s
~/dev/herdr-plugins main > curl -sI https://github.com/wchoco/herdr-plugins/issues/9
HTTP/2 200
server: github.com

~/dev/herdr-plugins main > cat /tmp/yubi/日本語メモ.txt
日本語の行です。全角文字は2セル幅なので、Hint や Line Number を重ねると崩れやすい。
{LONG}
~/dev/herdr-plugins main >"""

LONG = ("2026-10-22T09:14:03Z level=warn msg=\"retrying upload\" url=https://example.com/api/v1/"
        "uploads/8f3a2c1 attempt=3 path=/var/log/yubi/upload.log elapsed=1.204s peer=10.0.3.17")


def cell_width(ch):
    return 2 if unicodedata.east_asian_width(ch) in "WF" else 1


def soft_wrap(line, width):
    """What a terminal of `width` columns shows: one string per screen row."""
    rows, cur, w = [], "", 0
    for ch in line:
        cw = cell_width(ch)
        if w + cw > width:
            rows.append(cur)
            cur, w = "", 0
        cur += ch
        w += cw
    rows.append(cur)
    return rows


def load_rows(pane, width):
    if pane:
        out = subprocess.run(["herdr", "pane", "read", pane, "--source", "visible"],
                             capture_output=True, text=True, check=True).stdout
        return out.rstrip("\n").split("\n")
    rows = []
    for line in CANNED.replace("{LONG}", LONG).split("\n"):
        rows.extend(soft_wrap(line.replace("\t", "    "), width))
    return rows


# ---------------------------------------------------------------- cell grid

DIM, RESET = "\x1b[2m", "\x1b[0m"


class Row:
    """A screen row as cells; a wide char is a head cell followed by a None cell."""

    def __init__(self, text, style=""):
        self.cells = []
        for ch in text:
            self.cells.append([ch, style])
            if cell_width(ch) == 2:
                self.cells.append([None, style])

    def pad(self, n):
        while len(self.cells) < n:
            self.cells.append([" ", ""])

    def style(self, start, end, style):
        for c in self.cells[start:end]:
            c[1] = style

    def put(self, col, text, style):
        """Overwrite cells from col, repairing any wide char cut in half."""
        for ch in text:
            self.pad(col + 1)
            if self.cells[col][0] is None and col > 0:
                self.cells[col - 1][0] = " "
            if col + 1 < len(self.cells) and self.cells[col + 1][0] is None:
                self.cells[col + 1][0] = " "
            self.cells[col] = [ch, style]
            col += 1

    def insert(self, col, text, style):
        self.cells[col:col] = [[ch, style] for ch in text]

    def clip(self, width, marker="›"):
        if len(self.cells) > width:
            self.cells = self.cells[:width - 1]
            if self.cells and self.cells[-1][0] is None:
                self.cells[-1] = [" ", ""]
            self.cells.append([marker, "\x1b[7m"])

    def render(self):
        out, cur = [], None
        for ch, st in self.cells:
            if ch is None:
                continue
            if st != cur:
                out.append(RESET + st)
                cur = st
            out.append(ch)
        return "".join(out) + RESET


def text_col(text, char_idx):
    """Cell column of the char at char_idx (char count != column count)."""
    return sum(cell_width(c) for c in text[:char_idx])


# ---------------------------------------------------------------- matching / hints

PATTERNS = [
    ("url", r"https?://[^\s\"']+"),
    ("path", r"(?:~|\.\.?)?/[^\s:\"']+"),
    ("file:line", r"\b[\w.-]+\.\w+:\d+\b"),
    ("sha", r"\b[0-9a-f]{7,40}\b"),
    ("ip", r"\b\d{1,3}(?:\.\d{1,3}){3}\b"),
]
ALPHABET = "asdfghjklqwertyuiopzxcvbnm"


def find_matches(rows):
    """(row, start, end, text), leftmost wins, no overlaps."""
    found = []
    for r, text in enumerate(rows):
        taken = []
        cands = []
        for _, pat in PATTERNS:
            for m in re.finditer(pat, text):
                cands.append((m.start(), -(m.end() - m.start()), m.end()))
        for s, _, e in sorted(cands):
            if all(e <= ts or s >= te for ts, te in taken):
                taken.append((s, e))
                found.append((r, s, e, text[s:e]))
    return found


def assign_hints(matches):
    """Prefix-free 1-2 letter hints, nearest the bottom first, duplicates share one."""
    n = len({m[3] for m in matches})
    singles = len(ALPHABET)
    while singles > 0 and singles + (len(ALPHABET) - singles) * len(ALPHABET) < n:
        singles -= 1
    singles = min(singles, n) if n <= len(ALPHABET) else singles
    pool = list(ALPHABET[:singles]) + [a + b for a in ALPHABET[singles:] for b in ALPHABET]
    by_text, out = {}, {}
    for m in sorted(matches, key=lambda m: (-m[0], -m[1])):
        if m[3] not in by_text:
            by_text[m[3]] = pool[len(by_text)]
        out[m] = by_text[m[3]]
    return out


# ---------------------------------------------------------------- line numbers

def line_numbers(rows):
    """Row index -> Line Number, 1 at the last non-blank row, counting upward."""
    last = max((i for i, t in enumerate(rows) if t.strip()), default=-1)
    return {i: last - i + 1 for i in range(last + 1)}


def parse_selection(inp, maxn):
    """Loose grammar just for echoing: '3', '3-7', '2,5'. Real grammar is another ticket."""
    sel, bad = set(), []
    for tok in filter(None, inp.split(",")):
        m = re.fullmatch(r"(\d+)(?:-(\d*))?", tok)
        if not m:
            bad.append(tok)
            continue
        a = int(m.group(1))
        b = int(m.group(2)) if m.group(2) else a
        if m.group(2) == "":
            b = a  # range still being typed
        lo, hi = min(a, b), max(a, b)
        if lo < 1 or hi > maxn:
            bad.append(tok)
        sel.update(range(max(lo, 1), min(hi, maxn) + 1))
    return sel, bad


# ---------------------------------------------------------------- variants
# Each variant returns (body rows, footer string) for the given state.

HINT_ST = "\x1b[1;30;43m"     # black on yellow
HINT_DONE = "\x1b[2;30;43m"   # typed part of a hint
MATCH_ST = "\x1b[1;33m"
LNUM_ST = "\x1b[1;30;46m"     # black on cyan
SEL_ST = "\x1b[7m"


def variant_a(st):
    """In place (tmux-thumbs): overlay placement, labels overwrite cells, nothing moves."""
    rows = [Row(t) for t in st.rows]
    if st.mode == "hint":
        for r in rows:
            r.style(0, len(r.cells), DIM)
        for (ri, s, e, text), hint in st.hints.items():
            row, t = rows[ri], st.rows[ri]
            c0, c1 = text_col(t, s), text_col(t, e)
            row.style(c0, c1, MATCH_ST)
            if hint.startswith(st.input):
                row.put(c0, st.input, HINT_DONE)
                row.put(c0 + len(st.input), hint[len(st.input):], HINT_ST)
        footer = f" HINT  {st.input or '…'}"
    else:
        width = len(str(max(st.nums.values(), default=1)))
        for ri, n in st.nums.items():
            if n in st.sel:
                rows[ri].pad(st.width)
                rows[ri].style(0, st.width, SEL_ST)
            rows[ri].put(0, str(n).rjust(width), LNUM_ST)
        footer = f" LINE  {st.input or '…'}" + (f"   invalid: {','.join(st.bad)}" if st.bad else "")
    for r in rows:
        r.clip(st.width)
    return rows, footer


def variant_b(st):
    """Gutter: lossless text, labels get their own cells, text shifts right."""
    rows = [Row(t) for t in st.rows]
    if st.mode == "hint":
        # Hints inserted in front of each Match; right-to-left so columns stay valid.
        per_row = {}
        for m, hint in st.hints.items():
            per_row.setdefault(m[0], []).append((m, hint))
        for ri, items in per_row.items():
            t = st.rows[ri]
            for (ri_, s, e, text), hint in sorted(items, key=lambda x: -x[0][1]):
                c0, c1 = text_col(t, s), text_col(t, e)
                rows[ri].style(c0, c1, "\x1b[4m")
                if hint.startswith(st.input):
                    rows[ri].insert(c0, hint[len(st.input):], HINT_ST)
                    rows[ri].insert(c0, st.input, HINT_DONE)
        top = Row(f" yubi hint> {st.input}▏", "\x1b[1m")
    else:
        width = len(str(max(st.nums.values(), default=1)))
        for ri, r in enumerate(rows):
            n = st.nums.get(ri)
            mark = "▶" if n in st.sel else " "
            label = (str(n).rjust(width) if n else " " * width)
            r.insert(0, f"{mark}{label} │ ", "")
            r.style(1, 1 + width, LNUM_ST if n else "")
            if n in st.sel:
                r.pad(st.width)
                r.style(width + 4, st.width, SEL_ST)
        top = Row(f" yubi line> {st.input}▏" + (f"   invalid: {','.join(st.bad)}" if st.bad else ""), "\x1b[1m")
    for r in rows:
        r.clip(st.width)
    preview = ""
    if st.mode == "line" and st.sel:
        preview = f" Enter copies {len(st.sel)} line(s)"
    return [top] + rows, preview


def variant_c(st):
    """Popup: framed, centred, smaller; labels live at the right edge / after Matches."""
    inner = max(20, st.width - 6)
    rows = [Row(t) for t in st.rows]
    if st.mode == "hint":
        for r in rows:
            r.style(0, len(r.cells), DIM)
        for (ri, s, e, text), hint in st.hints.items():
            row, t = rows[ri], st.rows[ri]
            c0, c1 = text_col(t, s), text_col(t, e)
            row.style(c0, c1, MATCH_ST)
            if hint.startswith(st.input):
                row.put(c1, hint[len(st.input):], HINT_ST)
        title = f" yubi · hint {st.input} "
    else:
        width = len(str(max(st.nums.values(), default=1)))
        for ri, n in st.nums.items():
            r = rows[ri]
            r.clip(inner - width - 1, "…")
            r.pad(inner - width)
            if n in st.sel:
                r.style(0, inner - width, SEL_ST)
            r.put(inner - width, str(n).rjust(width), LNUM_ST)
        title = f" yubi · line {st.input or '_'} " + (f"invalid {','.join(st.bad)} " if st.bad else "")
    for r in rows:
        r.clip(inner)
    framed = [Row("  ╭" + title.center(inner, "─")[:inner] + "╮")]
    for r in rows[-(st.height - 4):]:
        r.pad(inner)
        r.cells = [[ch, ""] for ch in "  │"] + r.cells + [["│", ""]]
        framed.append(r)
    framed.append(Row("  ╰" + "─" * inner + "╯"))
    return framed, ""


VARIANTS = [
    ("A", "In place (overlay, overwrite)", variant_a),
    ("B", "Gutter (overlay, shift text)", variant_b),
    ("C", "Popup (framed, right edge / after Match)", variant_c),
]


# ---------------------------------------------------------------- state / loop

class State:
    def __init__(self, rows, width, height):
        self.rows, self.width, self.height = rows, width, height
        self.mode, self.input, self.variant, self.last = "line", "", 0, ""
        self.matches = find_matches(rows)
        self.hints = assign_hints(self.matches)
        self.nums = line_numbers(rows)
        self.refresh()

    def refresh(self):
        self.sel, self.bad = parse_selection(self.input, max(self.nums.values(), default=0))

    def key(self, k):
        if k in ("\x1b[C", "\x1b[D"):
            self.variant = (self.variant + (1 if k == "\x1b[C" else -1)) % len(VARIANTS)
        elif k == "\t":
            self.mode = "hint" if self.mode == "line" else "line"
            self.input = ""
        elif k == "\x7f":
            self.input = self.input[:-1]
        elif self.mode == "line":
            if k == "\r" and self.sel:
                by_num = {n: self.rows[i] for i, n in self.nums.items()}
                text = "\n".join(by_num[n] for n in sorted(self.sel, reverse=True))
                self.last = f"would copy {len(self.sel)} line(s): {text[:60]!r}"
                self.input = ""
            elif k in "0123456789,-" and len(k) == 1:
                self.input += k
        elif k.isalpha() and len(k) == 1:
            typed = self.input + k.lower()
            hit = [m for m, h in self.hints.items() if h == typed]
            if hit:
                verb = "copy+paste" if k.isupper() else "copy"
                self.last = f"would {verb}: {hit[0][3]!r}"
                self.input = ""
            elif any(h.startswith(typed) for h in self.hints.values()):
                self.input = typed
        self.refresh()

    def draw(self):
        key, name, fn = VARIANTS[self.variant]
        body, footer = fn(self)
        out = ["\x1b[H\x1b[2J"]
        avail = self.height - 2
        for r in body[-avail:] if len(body) > avail else body:
            out.append(r.render() + "\r\n")
        out.append(f"\x1b[{self.height - 1};1H{footer}")
        state = (f" ◀ {key}: {name} ▶ │ {self.mode} │ in={self.input!r} sel={sorted(self.sel)} "
                 f"│ {self.last}")
        out.append(f"\x1b[{self.height};1H\x1b[7m{state[:self.width].ljust(self.width)}\x1b[0m")
        sys.stdout.write("".join(out))
        sys.stdout.flush()


def main():
    pane = sys.argv[2] if len(sys.argv) > 2 and sys.argv[1] == "--pane" else None
    width, height = shutil.get_terminal_size()
    st = State(load_rows(pane, width), width, height)
    fd = sys.stdin.fileno()
    saved = termios.tcgetattr(fd)
    sys.stdout.write("\x1b[?1049h\x1b[?25l")
    try:
        tty.setraw(fd)
        while True:
            st.draw()
            k = os.read(fd, 16).decode(errors="replace")
            if k == "\x03" or (k == "\x1b" and not st.input):
                break
            if k == "\x1b":
                st.input = ""
                st.refresh()
                continue
            st.key(k)
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, saved)
        sys.stdout.write("\x1b[?25h\x1b[?1049l")


if __name__ == "__main__":
    main()
