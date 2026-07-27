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
# every flowchart link shape, longest-first so `-.->` is not consumed by the
# plain-arrow alternative. The bare open links need three characters (`---`,
# `===`), which is what keeps the two dashes of an inline `a -- text --> b`
# label from reading as a link of their own.
ARROW = re.compile(
    r"<-{2,}>|<-\.-+>|<={2,}>"  # bidirectional
    r"|o-{2,}o|x-{2,}x"  # circle / cross at both ends
    r"|-\.-+[>ox]|-\.-+"  # dotted
    r"|-{2,}[>ox]|-{3,}"  # normal
    r"|={2,}[>ox]|={3,}"  # thick
    r"|~{3,}"  # invisible
)


def strip_preamble(lines):
    """Drop a diagram's front matter, init directives and comments.

    Mermaid allows `---` front matter and `%%{init: ...}%%` before the type
    declaration. Stripping rather than rejecting keeps a sequenceDiagram with
    front matter skipping correctly; what must not happen is a flowchart
    reading as some other diagram type and skipping its edge check.
    Returns None when the front matter never closes.
    """
    if lines and lines[0] == "---":
        for position in range(1, len(lines)):
            if lines[position] == "---":
                lines = lines[position + 1 :]
                break
        else:
            return None
    return [line for line in lines if not line.startswith("%%")]


failures = []
checked = skipped = 0

for path in sorted(set(glob.glob("**/*.md", recursive=True))):
    text = open(path, encoding="utf-8").read()
    for index, block in enumerate(BLOCK.findall(text)):
        lines = [line.strip() for line in block.split("\n") if line.strip()]
        lines = strip_preamble(lines)
        if lines is None:
            failures.append(f"{path}: mermaid block {index} has unclosed front matter")
            continue
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
