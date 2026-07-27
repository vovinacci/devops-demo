#!/usr/bin/env bash
# Diagram gate: every node an edge points at must actually be declared.
#
# Mermaid renders on GitHub, not in CI, so a diagram is just text to every
# other check in this repo -- a typo in a node id produces a stray empty box
# or silently drops the edge, and nothing fails. That is how a broken render
# reached main once already.
#
# Flowcharts only. sequenceDiagram and stateDiagram-v2 have unrelated grammars
# and are counted as skipped rather than parsed with the wrong rules.
# Runs as a prek hook locally and in CI (same config).

set -euo pipefail

python3 <<'PY'
import glob
import re
import sys

# the closing fence may follow immediately, so an empty block is still a block
BLOCK = re.compile(r"```mermaid\n(.*?)```", re.S)
SUBGRAPH = re.compile(r"^subgraph\s+(\w+)")
QUOTED = re.compile(r'"[^"]*"')
EDGE_LABEL = re.compile(r"\|[^|]*\|")
# a declaration is an id immediately followed by its shape bracket, which may
# sit at the start of a line or inline on either side of an edge
DECL = re.compile(r"(\w+)\s*[\[\({]")
# longest-first so `-.->` is not consumed by the plain-arrow alternative
ARROW = re.compile(r"-\.->|-{2,3}>|={2}>|-{3}")

failures = []
checked = skipped = 0

for path in sorted(set(glob.glob("**/*.md", recursive=True))):
    text = open(path, encoding="utf-8").read()
    for index, block in enumerate(BLOCK.findall(text)):
        lines = [line.strip() for line in block.split("\n") if line.strip()]
        if not lines:
            failures.append(f"{path}: mermaid block {index} is empty")
            continue

        if lines[0].split()[0] not in ("flowchart", "graph"):
            skipped += 1
            continue
        checked += 1

        declared, subgraphs, referenced = set(), set(), set()
        for line in lines[1:]:
            if line == "end":
                continue
            match = SUBGRAPH.match(line)
            if match:
                subgraphs.add(match.group(1))
                continue

            # labels can contain anything, including brackets and arrows
            bare = QUOTED.sub('""', EDGE_LABEL.sub(" ", line))
            declared.update(DECL.findall(bare))
            if ARROW.search(bare):
                for part in ARROW.split(bare):
                    match = re.match(r"^(\w+)", part.strip())
                    if match:
                        referenced.add(match.group(1))

        for node in sorted(referenced - declared - subgraphs):
            failures.append(
                f"{path} block {index}: edge points at undeclared node {node!r}"
            )

if failures:
    for failure in failures:
        print(failure, file=sys.stderr)
    sys.exit(1)

print(f"mermaid: {checked} flowchart(s) checked, {skipped} other diagram(s) skipped")
PY
