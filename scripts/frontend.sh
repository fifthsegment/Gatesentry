#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
UI_DIR="$ROOT/ui"
DIST_DIR="$UI_DIR/dist"
EMBED_DIR="$ROOT/application/webserver/frontend/files"

require_yarn() {
  command -v node >/dev/null 2>&1 || { echo "error: Node.js is required" >&2; exit 1; }
  local expected_yarn
  expected_yarn=$(node -p "require('$UI_DIR/package.json').packageManager.split('@').pop()")
  command -v yarn >/dev/null 2>&1 || {
    echo "error: Yarn $expected_yarn is required; install or enable the packageManager declared in ui/package.json" >&2
    exit 1
  }
  local actual
  actual=$(yarn --version)
  if [[ "$actual" != "$expected_yarn" ]]; then
    echo "error: Yarn $expected_yarn is required, found $actual" >&2
    exit 1
  fi
}

install_ui() {
  require_yarn
  (cd "$UI_DIR" && yarn install --immutable)
}

validate_assets() {
  local dir=${1:-$EMBED_DIR}
  [[ -s "$dir/index.html" ]] || { echo "error: missing or empty dashboard index: $dir/index.html" >&2; exit 1; }
  grep -Eqi '<!doctype html|<html' "$dir/index.html" || { echo "error: dashboard index is not HTML" >&2; exit 1; }
  find "$dir" -type f -name '*.js' -size +0c -print -quit | grep -q . || { echo "error: dashboard has no nonempty JavaScript asset" >&2; exit 1; }
  find "$dir" -type f -name '*.css' -size +0c -print -quit | grep -q . || { echo "error: dashboard has no nonempty CSS asset" >&2; exit 1; }
}

build_and_sync() {
  install_ui
  rm -rf "$DIST_DIR"
  (cd "$UI_DIR" && yarn build)
  validate_assets "$DIST_DIR"

  local stage
  stage=$(mktemp -d "$ROOT/application/webserver/frontend/.files.XXXXXX")
  trap 'rm -rf "${stage:-}"' EXIT
  cp -a "$DIST_DIR"/. "$stage"/
  validate_assets "$stage"

  mkdir -p "$EMBED_DIR"
  find "$EMBED_DIR" -mindepth 1 ! -name .gitkeep -delete
  cp -a "$stage"/. "$EMBED_DIR"/
  validate_assets "$EMBED_DIR"
  rm -rf "$stage"
  trap - EXIT
}

case "${1:-build}" in
  install) install_ui ;;
  validate) validate_assets ;;
  build) build_and_sync ;;
  *) echo "usage: $0 [build|install|validate]" >&2; exit 2 ;;
esac
