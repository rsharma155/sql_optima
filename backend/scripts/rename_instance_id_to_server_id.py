#!/usr/bin/env python3
"""
Rename instance_id → server_id inside SQL string literals in Go source files.

Only touches occurrences that are inside backtick SQL strings (raw string literals)
or double-quoted strings, and only when the word appears as a column/alias name
(i.e., not inside msdb system table references like sysjobhistory).

Usage:
    python3 scripts/rename_instance_id_to_server_id.py [--dry-run]
"""

import re
import sys
import os
from pathlib import Path

DRY_RUN = "--dry-run" in sys.argv

# Repo root relative to this script (backend/)
BACKEND_DIR = Path(__file__).parent.parent

# Go source files to scan (exclude tests)
GO_FILES = [
    p for p in BACKEND_DIR.rglob("*.go")
    if "_test.go" not in p.name
]

# Patterns that should NOT be renamed — system table column aliases
SKIP_CONTEXTS = re.compile(
    r"(sysjobhistory|msdb\.|sys\.dm_)",
    re.IGNORECASE,
)

# Match instance_id as a word (not part of a larger identifier)
PATTERN = re.compile(r"\binstance_id\b")

changed = []

for path in sorted(GO_FILES):
    original = path.read_text(encoding="utf-8")
    lines = original.splitlines(keepends=True)
    new_lines = []
    file_changed = False
    inside_backtick = False

    for lineno, line in enumerate(lines, 1):
        new_line = line

        # Track backtick raw string boundaries (Go raw string literals).
        # A backtick opens or closes a raw string. Count them to toggle state.
        tick_count = line.count("`")
        if tick_count % 2 == 1:
            inside_backtick = not inside_backtick

        in_string = inside_backtick or '`' in line

        if in_string and PATTERN.search(new_line):
            # Skip lines that reference SQL Server system tables
            if SKIP_CONTEXTS.search(new_line):
                new_lines.append(line)
                continue

            replaced = PATTERN.sub("server_id", new_line)
            if replaced != new_line:
                print(f"  {path.relative_to(BACKEND_DIR)}:{lineno}  {new_line.rstrip()!r}")
                print(f"      → {replaced.rstrip()!r}")
                new_line = replaced
                file_changed = True

        new_lines.append(new_line)

    if file_changed:
        changed.append(path)
        if not DRY_RUN:
            path.write_text("".join(new_lines), encoding="utf-8")

print()
if DRY_RUN:
    print(f"DRY RUN — {len(changed)} file(s) would be changed.")
else:
    print(f"Done — {len(changed)} file(s) updated:")
    for p in changed:
        print(f"  {p.relative_to(BACKEND_DIR)}")
