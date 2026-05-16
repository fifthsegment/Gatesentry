#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# GateSentry Docker Publish Script
#
# Builds and pushes the GateSentry Docker image to either:
#   1. Docker Hub        (default)
#   2. Private Nexus     (--nexus)
#
# Usage:
#   ./docker-publish.sh [OPTIONS]
#
# Options:
#   --nexus              Push to private Nexus registry instead of Docker Hub
#   --version VERSION    Override version tag (default: from git tag or 'dev')
#   --no-build           Skip build, just tag and push an existing image
#   --no-latest          Don't push the 'latest' tag
#   --dry-run            Show what would be done without executing
#   -h, --help           Show this help
#
# Environment variables:
#   Docker Hub:
#     DOCKERHUB_USERNAME   Docker Hub username
#     DOCKERHUB_TOKEN      Docker Hub personal access token (preferred)
#     DOCKERHUB_PASSWORD   Docker Hub password (fallback if TOKEN not set)
#
#   Nexus:
#     NEXUS_USERNAME       Nexus registry username
#     NEXUS_PASSWORD       Nexus registry password
#
# Examples:
#   # Push to Docker Hub
#   DOCKERHUB_USERNAME=jbarwick DOCKERHUB_TOKEN=dckr_pat_xxx ./docker-publish.sh
#
#   # Push to Nexus
#   NEXUS_USERNAME=admin NEXUS_PASSWORD=xxx ./docker-publish.sh --nexus
#
#   # Push specific version without rebuilding
#   ./docker-publish.sh --nexus --version 1.20.6.1 --no-build
# =============================================================================

# ── Configuration ────────────────────────────────────────────────────────────

IMAGE_NAME="gatesentry"
DOCKERHUB_REPO="jbarwick/gatesentry"
DOCKERHUB_README="DOCKERHUB_README_V2.md"
# Strip any scheme (https://, http://) — Docker refs are just host:port
NEXUS_REGISTRY="${NEXUS_SERVER#https://}"
NEXUS_REGISTRY="${NEXUS_REGISTRY#http://}"
NEXUS_REPO="${NEXUS_REGISTRY}/${IMAGE_NAME}"

# ── Defaults ─────────────────────────────────────────────────────────────────

TARGET="dockerhub"
VERSION=""
DO_BUILD=true
PUSH_LATEST=true
DRY_RUN=false

# ── Parse arguments ──────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case "$1" in
        --nexus)
            TARGET="nexus"
            shift
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --no-build)
            DO_BUILD=false
            shift
            ;;
        --no-latest)
            PUSH_LATEST=false
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            head -42 "$0" | tail -38
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ── Detect version from git tag if not supplied ──────────────────────────────

if [[ -z "$VERSION" ]]; then
    VERSION=$(git describe --tags --exact-match 2>/dev/null || true)
    if [[ -z "$VERSION" ]]; then
        VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
    fi
    # Strip leading 'v' if present (v1.20.6.1 → 1.20.6.1)
    VERSION="${VERSION#v}"
fi

echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║  GateSentry Docker Publish                                   ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
echo "  Target:   ${TARGET}"
echo "  Version:  ${VERSION}"
echo "  Build:    ${DO_BUILD}"
echo "  Latest:   ${PUSH_LATEST}"
echo "  Dry run:  ${DRY_RUN}"
echo ""

# ── Helper: run or print ─────────────────────────────────────────────────────

run() {
    if [[ "$DRY_RUN" == true ]]; then
        echo "[DRY RUN] $*"
    else
        "$@"
    fi
}

# ── Step 1: Build ────────────────────────────────────────────────────────────

LOCAL_IMAGE="gatesentry-gatesentry"  # matches docker-compose service name

if [[ "$DO_BUILD" == true ]]; then
    echo "── Building binary and Docker image ──────────────────────────"
    run bash build.sh
    run docker compose build
    echo ""
fi

# ── Step 2: Login & Tag & Push ───────────────────────────────────────────────

if [[ "$TARGET" == "nexus" ]]; then
    # ── Nexus ──
    REPO="$NEXUS_REPO"
    REGISTRY="$NEXUS_REGISTRY"

    if [[ -z "${NEXUS_USERNAME:-}" || -z "${NEXUS_PASSWORD:-}" ]]; then
        echo "Error: NEXUS_USERNAME and NEXUS_PASSWORD must be set"
        echo "  export NEXUS_USERNAME=your_username"
        echo "  export NEXUS_PASSWORD=your_password"
        exit 1
    fi

    echo "── Logging in to Nexus: ${REGISTRY} ─────────────────────────"
    if [[ "$DRY_RUN" == true ]]; then
        echo "[DRY RUN] echo <password> | docker login \"${REGISTRY}\" -u ${NEXUS_USERNAME} --password-stdin"
    else
        echo "${NEXUS_PASSWORD}" | docker login "${REGISTRY}" -u "${NEXUS_USERNAME}" --password-stdin
    fi

else
    # ── Docker Hub ──
    REPO="$DOCKERHUB_REPO"
    REGISTRY="docker.io"

    # Accept DOCKERHUB_TOKEN (preferred) or DOCKERHUB_PASSWORD (fallback)
    DOCKERHUB_PASS="${DOCKERHUB_TOKEN:-${DOCKERHUB_PASSWORD:-}}"

    if [[ -z "${DOCKERHUB_USERNAME:-}" || -z "${DOCKERHUB_PASS}" ]]; then
        echo "Error: DOCKERHUB_USERNAME and DOCKERHUB_TOKEN (or DOCKERHUB_PASSWORD) must be set"
        echo "  export DOCKERHUB_USERNAME=your_username"
        echo "  export DOCKERHUB_TOKEN=dckr_pat_xxxx"
        exit 1
    fi

    echo "── Logging in to Docker Hub ─────────────────────────────────"
    if [[ "$DRY_RUN" == true ]]; then
        echo "[DRY RUN] echo <token> | docker login -u ${DOCKERHUB_USERNAME} --password-stdin"
    else
        echo "${DOCKERHUB_PASS}" | docker login -u "${DOCKERHUB_USERNAME}" --password-stdin
    fi
fi

echo ""
echo "── Tagging image ────────────────────────────────────────────────"
run docker tag "${LOCAL_IMAGE}" "${REPO}:${VERSION}"
echo "  ${REPO}:${VERSION}"

if [[ "$PUSH_LATEST" == true ]]; then
    run docker tag "${LOCAL_IMAGE}" "${REPO}:latest"
    echo "  ${REPO}:latest"
fi

echo ""
echo "── Pushing ──────────────────────────────────────────────────────"
run docker push "${REPO}:${VERSION}"

if [[ "$PUSH_LATEST" == true ]]; then
    run docker push "${REPO}:latest"
fi

echo ""
echo "── Done ─────────────────────────────────────────────────────────"
echo ""
echo "  Pushed:  ${REPO}:${VERSION}"
if [[ "$PUSH_LATEST" == true ]]; then
    echo "  Pushed:  ${REPO}:latest"
fi
echo ""

# ── Step 4: Update Docker Hub repository description ─────────────────────────

if [[ "$TARGET" == "dockerhub" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    README_FILE="${SCRIPT_DIR}/${DOCKERHUB_README}"

    if [[ -f "$README_FILE" ]]; then
        echo "── Updating Docker Hub repository description ───────────────"

        SHORT_DESC="DNS-based parental controls, ad blocking, and web filtering for your home network."
        FULL_DESC=$(cat "$README_FILE")

        if [[ "$DRY_RUN" == true ]]; then
            echo "[DRY RUN] PATCH https://hub.docker.com/v2/repositories/${DOCKERHUB_REPO}/"
            echo "  short description: ${SHORT_DESC}"
            echo "  full description:  ${DOCKERHUB_README} ($(wc -c < "$README_FILE") bytes)"
        else
            # Get a JWT token from Docker Hub API
            HUB_TOKEN=$(curl -s -X POST \
                "https://hub.docker.com/v2/users/login/" \
                -H "Content-Type: application/json" \
                -d "{\"username\":\"${DOCKERHUB_USERNAME}\",\"password\":\"${DOCKERHUB_PASS}\"}" \
                | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)

            if [[ -z "$HUB_TOKEN" ]]; then
                echo "  ⚠  Could not obtain Docker Hub API token — skipping README push"
                echo "     (image push still succeeded)"
            else
                # Build JSON payload with proper escaping via python3
                PAYLOAD=$(python3 -c "
import json, sys
with open(sys.argv[1], 'r') as f:
    full = f.read()
print(json.dumps({
    'description': sys.argv[2],
    'full_description': full
}))
" "$README_FILE" "$SHORT_DESC")

                HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
                    -X PATCH \
                    "https://hub.docker.com/v2/repositories/${DOCKERHUB_REPO}/" \
                    -H "Authorization: JWT ${HUB_TOKEN}" \
                    -H "Content-Type: application/json" \
                    -d "$PAYLOAD")

                if [[ "$HTTP_CODE" == "200" ]]; then
                    echo "  ✓ Repository description updated from ${DOCKERHUB_README}"
                else
                    echo "  ⚠  Failed to update description (HTTP ${HTTP_CODE})"
                    echo "     You can update it manually at https://hub.docker.com/r/${DOCKERHUB_REPO}"
                fi
            fi
        fi
        echo ""
    else
        echo "  Note: ${DOCKERHUB_README} not found — skipping description update"
        echo ""
    fi
fi

if [[ "$TARGET" == "nexus" ]]; then
    echo "  Pull with:"
    echo "    docker pull ${REPO}:${VERSION}"
    echo ""
    echo "  Synology Container Manager:"
    echo "    Registry URL:  https://${NEXUS_REGISTRY}"
    echo "    Image:         ${IMAGE_NAME}:${VERSION}"
fi
