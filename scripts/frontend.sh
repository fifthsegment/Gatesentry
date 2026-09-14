#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
UI_DIR="$ROOT/ui"
DIST_DIR="$UI_DIR/dist"
EMBED_DIR="$ROOT/application/webserver/frontend/files"
BLOCKPAGE_CSS="$ROOT/application/webserver/frontend/blockpage/material.css"

require_yarn() {
  command -v node >/dev/null 2>&1 || { echo "error: Node.js 24.x is required" >&2; exit 1; }
  local expected_yarn actual_yarn node_version node_major
  expected_yarn=$(node -p "require('$UI_DIR/package.json').packageManager.split('@').pop()")
  node_version=$(node --version)
  node_major=${node_version#v}
  node_major=${node_major%%.*}
  [[ "$node_major" == 24 ]] || { echo "error: Node.js 24.x is required, found $node_version" >&2; exit 1; }
  command -v yarn >/dev/null 2>&1 || {
    echo "error: Yarn $expected_yarn is required; enable the packageManager declared in ui/package.json" >&2
    exit 1
  }
  actual_yarn=$(yarn --version)
  [[ "$actual_yarn" == "$expected_yarn" ]] || { echo "error: Yarn $expected_yarn is required, found $actual_yarn" >&2; exit 1; }
}

install_ui() {
  require_yarn
  (cd "$UI_DIR" && yarn install --immutable)
}

validate_assets() {
  local dir=${1:-$EMBED_DIR}
  [[ -s "$dir/index.html" ]] || { echo "error: missing or empty dashboard index: $dir/index.html" >&2; exit 1; }
  grep -Eqi '<!doctype html|<html' "$dir/index.html" || { echo "error: dashboard index is not HTML" >&2; exit 1; }
  grep -Eq "src=[\"']/?fs/bundle\\.js[\"']" "$dir/index.html" || { echo "error: dashboard index does not reference fs/bundle.js" >&2; exit 1; }
  local asset asset_path
  while IFS= read -r asset; do
    asset_path=${asset#/}
    [[ -s "$dir/$asset_path" ]] || { echo "error: missing or empty dashboard asset referenced by index: $dir/$asset_path" >&2; exit 1; }
  done < <(grep -Eo '(src|href)="[^"]+"' "$dir/index.html" | cut -d '"' -f 2 | grep '^/' || true)
  grep -Eq "href=[\"']/?vite\\.svg[\"']" "$dir/index.html" || { echo "error: dashboard index does not reference vite.svg" >&2; exit 1; }
  [[ -s "$dir/fs/bundle.js" ]] || { echo "error: missing or empty dashboard script: $dir/fs/bundle.js" >&2; exit 1; }
  find "$dir/fs" -type f -name '*.css' -size +0c -print -quit | grep -q . || { echo "error: dashboard has no nonempty stylesheet" >&2; exit 1; }
  [[ -s "$dir/vite.svg" ]] || { echo "error: missing or empty dashboard icon: $dir/vite.svg" >&2; exit 1; }
  ! find "$dir" -type f -empty ! -name .gitkeep -print -quit | grep -q . || { echo "error: dashboard contains an empty asset" >&2; exit 1; }
}

validate_blockpage_assets() {
  [[ -s "$BLOCKPAGE_CSS" ]] || { echo "error: missing or empty block-page stylesheet: $BLOCKPAGE_CSS" >&2; exit 1; }
  grep -q '\.mdl-' "$BLOCKPAGE_CSS" || { echo "error: block-page stylesheet is invalid: $BLOCKPAGE_CSS" >&2; exit 1; }
}

build_and_sync() {
  require_yarn
  validate_blockpage_assets
  rm -rf "$DIST_DIR"
  (cd "$UI_DIR" && yarn build)
  validate_assets "$DIST_DIR"

  local stage old
  stage=$(mktemp -d "$ROOT/application/webserver/frontend/.files.XXXXXX")
  old="$ROOT/application/webserver/frontend/.files.old.$$"
  cleanup_sync() {
    rm -rf "${stage:-}"
    if [[ -n "${old:-}" && -d "$old" ]]; then
      rm -rf "$EMBED_DIR"
      mv "$old" "$EMBED_DIR"
    fi
  }
  trap cleanup_sync EXIT
  cp -a "$DIST_DIR"/. "$stage"/
  touch "$stage/.gitkeep"
  validate_assets "$stage"

  [[ "$EMBED_DIR" == "$ROOT/application/webserver/frontend/files" ]] || { echo "error: refusing unsafe asset destination: $EMBED_DIR" >&2; exit 1; }
  [[ -d "$EMBED_DIR" ]] || { echo "error: embedded asset directory is missing: $EMBED_DIR" >&2; exit 1; }
  mv "$EMBED_DIR" "$old"
  mv "$stage" "$EMBED_DIR"
  stage=""
  validate_assets "$EMBED_DIR"
  rm -rf "$old"
  old=""
  trap - EXIT
}

case "${1:-build}" in
  install) install_ui ;;
  validate) validate_assets; validate_blockpage_assets ;;
  sync) build_and_sync ;;
  build) install_ui; build_and_sync ;;
  *) echo "usage: $0 [build|install|sync|validate]" >&2; exit 2 ;;
esac
