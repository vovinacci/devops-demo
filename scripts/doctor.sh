#!/usr/bin/env bash
# Toolchain doctor (ADR-0012): verifies the local environment and
# prints actionable errors. The first command to run after cloning.
# Exit code 1 if any required check fails; warnings do not fail.

set -uo pipefail

# canonical toolchain versions live in .mise.toml (single source of truth)
PYTHON_MINOR="$(sed -n 's/^python = "\(.*\)"/\1/p' .mise.toml)"
NODE_MAJOR="$(sed -n 's/^node = "\(.*\)"/\1/p' .mise.toml)"
RUST_MINOR="$(sed -n 's/^rust = "\(.*\)"/\1/p' .mise.toml)"
GO_MINOR="$(sed -n 's/^go = "\(.*\)"/\1/p' .mise.toml)"
BUF_MINOR="$(sed -n 's/^buf = "\(.*\)"/\1/p' .mise.toml)"
K6_MINOR="$(sed -n 's/^k6 = "\(.*\)"/\1/p' .mise.toml)"
# JDK pinned as temurin-<major>; only the major is user-visible in `java -version`
JAVA_MAJOR="$(sed -n 's/^java = "temurin-\(.*\)"/\1/p' .mise.toml)"
GRADLE_MINOR="$(sed -n 's/^gradle = "\(.*\)"/\1/p' .mise.toml)"
HELM_MINOR="$(sed -n 's/^helm = "\(.*\)"/\1/p' .mise.toml)"
KUBECONFORM_MINOR="$(sed -n 's/^kubeconform = "\(.*\)"/\1/p' .mise.toml)"
KIND_MINOR="$(sed -n 's/^kind = "\(.*\)"/\1/p' .mise.toml)"
KUBECTL_MINOR="$(sed -n 's/^kubectl = "\(.*\)"/\1/p' .mise.toml)"
MIN_RAM_GB=4
# Measured: ~4.2 GiB for every profile plus the observability stack, so 8 GiB
# is the point below which the Kind path starts to hurt.
MIN_RAM_GB_KIND=8
MIN_DISK_GB=5

failures=0
warnings=0

pass() { printf 'ok    %s\n' "$1"; }
fail() {
  printf 'FAIL  %s\n      fix: %s\n' "$1" "$2"
  failures=$((failures + 1))
}
warn() {
  printf 'warn  %s\n      hint: %s\n' "$1" "$2"
  warnings=$((warnings + 1))
}

require_cmd() {
  local cmd="$1" hint="$2"
  if command -v "$cmd" >/dev/null 2>&1; then
    pass "$cmd ($(command -v "$cmd"))"
  else
    fail "$cmd not found" "$hint"
  fi
}

echo "devops-demo doctor"
echo

# --- mise banner --------------------------------------------------------
# Runs before the per-tool checks below. When several pinned tools are
# absent, eight separate "install X" hints bury the one answer that fixes
# all of them at once, so say it first and say it once. Silent when the
# toolchain is already complete.
mise_missing=""
mise_missing_count=0
for managed in python3 node cargo go buf k6 java gradle helm kubeconform kind kubectl; do
  if ! command -v "$managed" >/dev/null 2>&1; then
    mise_missing="${mise_missing:+${mise_missing} }${managed}"
    mise_missing_count=$((mise_missing_count + 1))
  fi
done

if [ "$mise_missing_count" -gt 0 ]; then
  banner_shell="${SHELL:-/bin/bash}"
  banner_shell="${banner_shell##*/}"
  echo "----------------------------------------------------------------"
  printf '  %d pinned tool(s) not on PATH: %s\n\n' "$mise_missing_count" "$mise_missing"
  if command -v mise >/dev/null 2>&1; then
    echo "  mise is installed and .mise.toml pins every one of them."
    echo "  Install the pinned versions in one step:"
    echo
    echo "      mise install"
  else
    echo "  All of them are pinned in .mise.toml, and mise installs the"
    echo "  whole set in one step -- you do not need to chase them"
    echo "  individually, or match versions by hand:"
    echo
    echo "      curl https://mise.run | sh"
    echo "      mise install"
  fi
  # '$' passed as an argument, not written inline: it is literal text for the
  # reader to copy, and inline it reads as an unexpanded command substitution.
  printf '      eval "%s(mise activate %s)"   # add to your shell rc\n' '$' "$banner_shell"
  echo
  echo "  Then re-run: make doctor"
  echo "----------------------------------------------------------------"
  echo
fi

# --- required commands -------------------------------------------------
require_cmd git "install git via your package manager"
require_cmd make "install GNU make (xcode-select --install / apt install make)"
require_cmd docker "install Docker Desktop or OrbStack (macOS) / docker-ce (Linux)"
require_cmd python3 "run 'mise install' (see .mise.toml) or install Python ${PYTHON_MINOR}"
require_cmd node "run 'mise install' (see .mise.toml) or install Node ${NODE_MAJOR}"
require_cmd npm "ships with Node; run 'mise install'"
require_cmd cargo "run 'mise install' (see .mise.toml) or install Rust ${RUST_MINOR}"
require_cmd go "run 'mise install' (see .mise.toml) or install Go ${GO_MINOR}"
require_cmd buf "run 'mise install' (see .mise.toml) or install buf ${BUF_MINOR} (https://buf.build/docs/installation)"
require_cmd k6 "run 'mise install' (see .mise.toml) or install k6 ${K6_MINOR} (https://grafana.com/docs/k6/latest/set-up/install-k6/)"
require_cmd java "run 'mise install' (see .mise.toml) or install Temurin JDK ${JAVA_MAJOR}"
require_cmd gradle "run 'mise install' (see .mise.toml) or install Gradle ${GRADLE_MINOR} (the reports service also ships a committed wrapper)"
require_cmd helm "run 'mise install' (see .mise.toml) or install Helm ${HELM_MINOR} (https://helm.sh/docs/intro/install/)"
require_cmd kubeconform "run 'mise install' (see .mise.toml) or install kubeconform ${KUBECONFORM_MINOR}"
require_cmd kind "run 'mise install' (see .mise.toml) or install kind ${KIND_MINOR} (https://kind.sigs.k8s.io/)"
require_cmd kubectl "run 'mise install' (see .mise.toml) or install kubectl ${KUBECTL_MINOR}"

# --- docker daemon and compose v2 --------------------------------------
if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    pass "docker daemon reachable"
  else
    fail "docker daemon not reachable" "start Docker Desktop / OrbStack / dockerd"
  fi
  if docker compose version >/dev/null 2>&1; then
    pass "docker compose v2 ($(docker compose version --short 2>/dev/null))"
  else
    fail "docker compose v2 plugin missing" "install docker-compose-plugin (compose v1 is unsupported)"
  fi
fi

# --- versions -----------------------------------------------------------
if command -v python3 >/dev/null 2>&1; then
  py="$(python3 -c 'import sys; print(".".join(map(str, sys.version_info[:2])))')"
  if [ "$py" = "$PYTHON_MINOR" ]; then
    pass "python ${py}"
  else
    warn "python ${py}, expected ${PYTHON_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v node >/dev/null 2>&1; then
  nv="$(node --version | sed 's/^v//' | cut -d. -f1)"
  if [ "$nv" = "$NODE_MAJOR" ]; then
    pass "node ${nv}"
  else
    warn "node ${nv}, expected ${NODE_MAJOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v cargo >/dev/null 2>&1; then
  rv="$(cargo --version | awk '{print $2}' | cut -d. -f1,2)"
  if [ "$rv" = "$RUST_MINOR" ]; then
    pass "cargo (rust ${rv})"
  else
    warn "rust ${rv}, expected ${RUST_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v go >/dev/null 2>&1; then
  gv="$(go env GOVERSION | sed 's/^go//' | cut -d. -f1,2)"
  if [ "$gv" = "$GO_MINOR" ]; then
    pass "go ${gv}"
  else
    warn "go ${gv}, expected ${GO_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v buf >/dev/null 2>&1; then
  bv="$(buf --version | cut -d. -f1,2)"
  if [ "$bv" = "$BUF_MINOR" ]; then
    pass "buf ${bv}"
  else
    warn "buf ${bv}, expected ${BUF_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v k6 >/dev/null 2>&1; then
  kv="$(k6 version | sed -n 's/^k6 v\([0-9]*\.[0-9]*\).*/\1/p')"
  if [ "$kv" = "$K6_MINOR" ]; then
    pass "k6 ${kv}"
  else
    warn "k6 ${kv}, expected ${K6_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v java >/dev/null 2>&1; then
  jv="$(java -version 2>&1 | sed -n 's/.*version "\([0-9]*\).*/\1/p' | head -n1)"
  if [ "$jv" = "$JAVA_MAJOR" ]; then
    pass "java ${jv}"
  else
    warn "java ${jv}, expected ${JAVA_MAJOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v gradle >/dev/null 2>&1; then
  grv="$(gradle --version 2>/dev/null | sed -n 's/^Gradle \([0-9]*\.[0-9]*\).*/\1/p')"
  if [ "$grv" = "$GRADLE_MINOR" ]; then
    pass "gradle ${grv}"
  else
    warn "gradle ${grv}, expected ${GRADLE_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v helm >/dev/null 2>&1; then
  hv="$(helm version --short 2>/dev/null | sed 's/^v//' | cut -d. -f1,2)"
  if [ "$hv" = "$HELM_MINOR" ]; then
    pass "helm ${hv}"
  else
    warn "helm ${hv}, expected ${HELM_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

if command -v kubeconform >/dev/null 2>&1; then
  kcv="$(kubeconform -v 2>&1 | sed 's/^v//' | cut -d. -f1,2)"
  if [ "$kcv" = "$KUBECONFORM_MINOR" ]; then
    pass "kubeconform ${kcv}"
  else
    warn "kubeconform ${kcv}, expected ${KUBECONFORM_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

# kind has printed its version in two shapes -- "kind version 0.32.0" and
# "kind v0.32.0 go1.26.3 darwin/arm64" -- so pick the first token that LOOKS
# like a version instead of trusting a field position.
if command -v kind >/dev/null 2>&1; then
  kiv="$(kind --version 2>/dev/null | tr ' ' '\n' | sed -n 's/^v\{0,1\}\([0-9][0-9.]*\)$/\1/p' | head -n1 | cut -d. -f1,2)"
  if [ "$kiv" = "$KIND_MINOR" ]; then
    pass "kind ${kiv}"
  else
    warn "kind ${kiv}, expected ${KIND_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

# Client-only: `kubectl version` without --client contacts a cluster, which
# doctor must never require (it runs before `make kind-up`).
if command -v kubectl >/dev/null 2>&1; then
  kbv="$(kubectl version --client -o yaml 2>/dev/null | sed -n 's/^ *gitVersion: v\([0-9]*\.[0-9]*\).*/\1/p' | head -n1)"
  if [ "$kbv" = "$KUBECTL_MINOR" ]; then
    pass "kubectl ${kbv}"
  else
    warn "kubectl ${kbv}, expected ${KUBECTL_MINOR}" "mise install, then activate mise in your shell: eval \"\$(mise activate zsh)\" in ~/.zshrc"
  fi
fi

# --- recommended tools --------------------------------------------------
if command -v mise >/dev/null 2>&1; then
  pass "mise ($(mise --version 2>/dev/null | head -n1))"
else
  warn "mise not installed" "https://mise.jdx.dev -- pins the toolchain from .mise.toml"
fi

if command -v prek >/dev/null 2>&1; then
  pass "prek ($(prek --version 2>/dev/null | head -n1))"
elif command -v pre-commit >/dev/null 2>&1; then
  pass "pre-commit (fallback for prek; same config)"
else
  warn "no git hooks runner" "install prek (or: pip install pre-commit), then 'make pre-commit-install'"
fi

# --- resources ----------------------------------------------------------
case "$(uname -s)" in
  Darwin) ram_gb=$(($(sysctl -n hw.memsize) / 1024 / 1024 / 1024)) ;;
  Linux) ram_gb=$(($(awk '/MemTotal/ {print $2}' /proc/meminfo) / 1024 / 1024)) ;;
  *) ram_gb=0 ;;
esac
if [ "$ram_gb" -ge "$MIN_RAM_GB" ]; then
  pass "RAM ${ram_gb} GiB"
else
  warn "RAM ${ram_gb} GiB, recommended >= ${MIN_RAM_GB} GiB" "the core compose profile needs ~2 GiB for containers"
fi

# The Kind path is a separate, much larger ask: a control plane, three nodes
# and the Prometheus Operator stack beside the services. Measured at ~4.2 GiB
# with every profile deployed, so warn well before a machine gets there.
if [ "$ram_gb" -lt "$MIN_RAM_GB_KIND" ]; then
  warn "RAM ${ram_gb} GiB is thin for the Kind path (recommended >= ${MIN_RAM_GB_KIND} GiB)" \
    "compose is unaffected; for Kubernetes deploy fewer profiles (make kind-deploy PROFILE=core) -- see docs/runbooks/kubernetes-bring-up.md"
fi

disk_gb=$(($(df -k . | awk 'NR==2 {print $4}') / 1024 / 1024))
if [ "$disk_gb" -ge "$MIN_DISK_GB" ]; then
  pass "disk ${disk_gb} GiB free"
else
  warn "disk ${disk_gb} GiB free, recommended >= ${MIN_DISK_GB} GiB" "images and volumes need room"
fi

# --- repo consistency -----------------------------------------------------
echo
if bash scripts/check-toolchain-drift.sh; then
  pass "toolchain consumers agree with .mise.toml"
else
  fail "toolchain version drift" "align the files listed above with .mise.toml"
fi

# --- summary ------------------------------------------------------------
echo
if [ "$failures" -gt 0 ]; then
  echo "doctor: ${failures} failure(s), ${warnings} warning(s)"
  exit 1
fi
echo "doctor: healthy (${warnings} warning(s))"
