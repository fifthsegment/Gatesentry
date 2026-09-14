#!/usr/bin/env bash
set -euo pipefail

required_go=go1.24.10
required_node_major=24
required_yarn=4.10.3

command -v go >/dev/null 2>&1 || { echo "error: Go $required_go is required" >&2; exit 1; }
actual_go=$(GOTOOLCHAIN="$required_go" go env GOVERSION)
[[ "$actual_go" == "$required_go" ]] || { echo "error: Go $required_go is required, found $actual_go" >&2; exit 1; }

if [[ "${1:-all}" == go ]]; then
  echo "Verified Go toolchain: $actual_go"
  exit 0
fi

command -v node >/dev/null 2>&1 || { echo "error: Node.js $required_node_major.x is required" >&2; exit 1; }
command -v yarn >/dev/null 2>&1 || { echo "error: Yarn $required_yarn is required" >&2; exit 1; }
actual_node=$(node --version)
actual_yarn=$(yarn --version)
actual_node_major=${actual_node#v}
actual_node_major=${actual_node_major%%.*}
[[ "$actual_node_major" == "$required_node_major" ]] || { echo "error: Node.js $required_node_major.x is required, found $actual_node" >&2; exit 1; }
[[ "$actual_yarn" == "$required_yarn" ]] || { echo "error: Yarn $required_yarn is required, found $actual_yarn" >&2; exit 1; }

echo "Verified toolchains: $actual_go, Node.js $actual_node, Yarn $actual_yarn"
