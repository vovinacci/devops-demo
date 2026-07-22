#!/bin/sh
# Supplies LOADGEN_REF_UNIX (the RFC-0001 D5 evaluation reference,
# loadprofile/README.md) once per container start. Compose `environment:`
# entries are static strings -- they cannot shell-interpolate `date +%s`
# themselves -- so this wrapper does it before k6 ever loads a script.
# Overridable: set LOADGEN_REF_UNIX before starting the container to
# reproduce a specific run (e.g. to line up with a fixed golden window).
set -eu

: "${LOADGEN_REF_UNIX:=$(date +%s)}"
export LOADGEN_REF_UNIX

exec k6 "$@"
