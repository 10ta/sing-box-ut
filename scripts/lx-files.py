#!/usr/bin/env python3
"""Select the sing-box-lx files that lx-files.txt asks for.

Reads repository-relative paths on stdin (for example `git ls-files` or
`git ls-tree -r --name-only <commit>` of an lx checkout) and prints the
selected ones, sorted. prepare.sh uses it to copy files; lx-diff.sh uses it to
compare two lx commits without checking them out, so both apply one rule set.

Rules (lx-files.txt): one glob per line, matched segment by segment ("*" never
crosses "/"). A line ending in "/" selects that directory's *.go files, not its
subdirectories. A line starting with "!" drops files whose base name matches.
"#" starts a comment. A rule that selects nothing is an error.

usage: scripts/lx-files.py [RULES_FILE] < paths
"""
import fnmatch
import os
import sys


def load_rules(path):
    includes, excludes = [], []
    with open(path) as f:
        for raw in f:
            line = raw.split("#", 1)[0].strip()
            if not line:
                continue
            if line.startswith("!"):
                excludes.append(line[1:])
            else:
                includes.append(line)
    return includes, excludes


def matches(pattern, path):
    if pattern.endswith("/"):
        return os.path.dirname(path) == pattern.rstrip("/") and path.endswith(".go")
    pattern_parts, path_parts = pattern.split("/"), path.split("/")
    return len(pattern_parts) == len(path_parts) and all(
        fnmatch.fnmatchcase(part, glob) for part, glob in zip(path_parts, pattern_parts)
    )


def main():
    rules = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), "..", "lx-files.txt")
    includes, excludes = load_rules(rules)
    paths = [line.strip() for line in sys.stdin if line.strip()]
    selected, failed = set(), []
    for pattern in includes:
        hits = [
            p for p in paths
            if matches(pattern, p) and not any(fnmatch.fnmatchcase(os.path.basename(p), ex) for ex in excludes)
        ]
        if not hits:
            failed.append(pattern)
        selected.update(hits)
    for pattern in failed:
        print(f"lx-files: nothing matches {pattern}", file=sys.stderr)
    if failed:
        sys.exit(1)
    print("\n".join(sorted(selected)))


if __name__ == "__main__":
    main()
