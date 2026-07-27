#!/usr/bin/env bash
# Dashboard identity gate: every provisioned dashboard carries a stable uid.
#
# Grafana's file provider identifies a dashboard by uid. A file without one is
# assigned a random uid at provision time, so the dashboard has no durable
# identity: a dashboard saved from the UI detaches from its file, and the next
# restart provisions a second dashboard with the same title beside it. Pinning
# the uid in the file is what keeps one file to one dashboard, and what keeps
# /d/<uid>/ links stable across a rebuild.
# Runs as a prek hook locally and in CI (same config).

set -euo pipefail

python3 <<'PY'
import collections
import glob
import json
import os
import sys

DASHBOARD_DIR = "observability/grafana/dashboards"
UID_MAX = 40  # Grafana rejects anything longer

by_uid = collections.defaultdict(list)
failures = []

for path in sorted(glob.glob(os.path.join(DASHBOARD_DIR, "*.json"))):
    try:
        with open(path, encoding="utf-8") as handle:
            dashboard = json.load(handle)
    except (OSError, json.JSONDecodeError) as exc:
        failures.append(f"{path}: cannot read as JSON: {exc}")
        continue

    if not isinstance(dashboard, dict):
        failures.append(
            f"{path}: top level is {type(dashboard).__name__}, expected a JSON object"
        )
        continue

    uid = dashboard.get("uid")
    if uid is None or uid == "":
        failures.append(
            f'{path}: no "uid" -- add a stable one so this file owns exactly '
            "one dashboard (without it a UI save detaches and the next restart "
            "provisions a duplicate)"
        )
    elif not isinstance(uid, str):
        failures.append(f'{path}: "uid" must be a string, got {type(uid).__name__}')
    elif len(uid) > UID_MAX:
        failures.append(f'{path}: uid "{uid}" is longer than {UID_MAX} characters')
    else:
        by_uid[uid].append(path)

for uid, paths in sorted(by_uid.items()):
    if len(paths) > 1:
        failures.append(f'uid "{uid}" is used by more than one file: {", ".join(paths)}')

if failures:
    for failure in failures:
        print(failure, file=sys.stderr)
    sys.exit(1)
PY
