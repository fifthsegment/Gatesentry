#!/usr/bin/env bash
###############################################################################
# Generate ephemeral CA + server certificates for GateSentry test bed.
#
# Creates:
#   JVJCA.crt / JVJCA.key           — self-signed CA (internal-ca)
#   httpbin.org.crt / httpbin.org.key — server cert signed by the CA
#
# All certs are written to the same directory as this script (tests/fixtures/).
# They are listed in .gitignore and must NOT be committed.
#
# Usage:
#   bash tests/fixtures/gen_test_certs.sh           # generate once
#   bash tests/fixtures/gen_test_certs.sh --force   # regenerate even if exist
###############################################################################

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CA_KEY="${SCRIPT_DIR}/JVJCA.key"
CA_CERT="${SCRIPT_DIR}/JVJCA.crt"
SERVER_KEY="${SCRIPT_DIR}/httpbin.org.key"
SERVER_CERT="${SCRIPT_DIR}/httpbin.org.crt"
DAYS_VALID=365

FORCE=false
[[ "${1:-}" == "--force" ]] && FORCE=true

# Skip if certs already exist (unless --force)
if [[ "$FORCE" == false && -f "$CA_CERT" && -f "$SERVER_CERT" && -f "$SERVER_KEY" ]]; then
    echo "[gen_test_certs] Certificates already exist — skipping (use --force to regenerate)"
    exit 0
fi

echo "[gen_test_certs] Generating ephemeral test certificates in ${SCRIPT_DIR}/"

# ── 1. CA key + self-signed cert ────────────────────────────────────────────
openssl genrsa -out "$CA_KEY" 2048 2>/dev/null

openssl req -new -x509 \
    -key "$CA_KEY" \
    -out "$CA_CERT" \
    -days "$DAYS_VALID" \
    -subj "/CN=internal-ca/C=SG/L=Singapore/O=JVJ 28 Inc." \
    2>/dev/null

echo "[gen_test_certs] CA certificate:  ${CA_CERT}"

# ── 2. Server key + CSR + CA-signed cert ────────────────────────────────────
openssl genrsa -out "$SERVER_KEY" 2048 2>/dev/null

openssl req -new \
    -key "$SERVER_KEY" \
    -out "${SCRIPT_DIR}/httpbin.org.csr" \
    -subj "/CN=httpbin.org/C=SG/L=Singapore/O=JVJ 28 Inc." \
    2>/dev/null

# SAN extension for httpbin.org + localhost
cat > "${SCRIPT_DIR}/_san.cnf" <<EOF
[v3_req]
subjectAltName = DNS:httpbin.org, DNS:localhost, IP:127.0.0.1
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
EOF

openssl x509 -req \
    -in "${SCRIPT_DIR}/httpbin.org.csr" \
    -CA "$CA_CERT" \
    -CAkey "$CA_KEY" \
    -CAcreateserial \
    -out "$SERVER_CERT" \
    -days "$DAYS_VALID" \
    -extensions v3_req \
    -extfile "${SCRIPT_DIR}/_san.cnf" \
    2>/dev/null

echo "[gen_test_certs] Server certificate: ${SERVER_CERT}"

# ── Cleanup temp files ──────────────────────────────────────────────────────
rm -f "${SCRIPT_DIR}/httpbin.org.csr" "${SCRIPT_DIR}/_san.cnf" "${SCRIPT_DIR}/JVJCA.srl"

echo "[gen_test_certs] Done — certificates valid for ${DAYS_VALID} days"
