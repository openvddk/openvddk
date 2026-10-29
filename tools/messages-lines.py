#!/usr/bin/env python3
# Copyright 2026 Andrija Panic and the OpenVDDK contributors
# SPDX-License-Identifier: Apache-2.0

"""Check and refresh the line numbers in docs/MESSAGES.md.

Every `internal/…go:N` / `cabi/…go:N` reference must point at a log
statement (a line containing Infof, Warnf, logf, warnf or c.warn) whose
text matches the row's quoted text where that text is distinctive. A
reference that drifted is moved to the line of the same file that contains
the row's quoted text (the first backticked text of the row, up to its
first format verb). Usage: messages-lines.py [--fix]
"""
import re
import sys

DOC = "docs/MESSAGES.md"
LOGCALL = re.compile(r"Infof|Warnf|logf|warnf|c\.warn")
REF = re.compile(r"((?:internal|cabi)/[\w/]+\.go):(\d+)")


def needle(text):
    """The longest literal part of a format string (between verbs)."""
    parts = re.split(r"%[-+# 0-9.]*[a-zA-Z]|…", text)
    best = max((x.strip() for x in parts), key=len, default="")
    return best if len(best) >= 6 else None


def main(fix):
    doc = open(DOC, encoding="utf-8").read().split("\n")
    files = {}
    bad = fixed = 0
    for i, row in enumerate(doc):
        for m in REF.finditer(row):
            path, n = m.group(1), int(m.group(2))
            lines = files.setdefault(path, open(path, encoding="utf-8").read().split("\n"))
            q = re.findall(r"`([^`]+)`", row)
            hits = []
            if q:
                exact = '"' + q[0] + '"'
                hits = [k + 1 for k, l in enumerate(lines) if exact in l and LOGCALL.search(l)]
                if len(hits) != 1:
                    nd = needle(q[0])
                    hits = [k + 1 for k, l in enumerate(lines) if nd and nd in l and LOGCALL.search(l)] if nd else []
            on_log = 0 < n <= len(lines) and LOGCALL.search(lines[n - 1])
            # A log line is accepted when the row's text is on it, or when the
            # text (a description rather than the format string) is on no line.
            if on_log and (n in hits or not hits):
                continue
            bad += 1
            if fix and len(hits) == 1:
                doc[i] = doc[i].replace(m.group(0), f"{path}:{hits[0]}", 1)
                fixed += 1
            else:
                print(f"{DOC}:{i + 1}: {path}:{n} is not a log statement; candidates {hits}")
    if fix and fixed:
        open(DOC, "w", encoding="utf-8", newline="\n").write("\n".join(doc))
    print(f"references not on a log statement: {bad}; refreshed: {fixed}")
    return 0 if bad == fixed else 1


if __name__ == "__main__":
    sys.exit(main("--fix" in sys.argv))
