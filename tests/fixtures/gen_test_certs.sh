#!/usr/bin/env bash
###############################################################################
# Generate server certificate for GateSentry test bed.
#
# Requires (already present in tests/fixtures/):
#   JVJCA.crt / JVJCA.key           — CA cert+key (installed in system trust)
#
# Creates:
#   httpbin.org.crt / httpbin.org.key — server cert signed by the CA
#     SANs: DNS:httpbin.org, DNS:localhost, IP:127.0.0.1
#
# The CA cert is already installed as a trusted CA on the system so certs
# signed by it are trusted by Go's tls.Dial, curl, etc.
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
DAYS_VALID=3650

# ── Verify CA cert and key exist ────────────────────────────────────────────
if [[ ! -f "$CA_CERT" ]]; then
    echo "[gen_test_certs] ERROR: CA certificate not found: ${CA_CERT}" >&2
    exit 1
fi
if [[ ! -f "$CA_KEY" ]]; then
    echo "[gen_test_certs] ERROR: CA key not found: ${CA_KEY}" >&2
    exit 1
fi

FORCE=false
[[ "${1:-}" == "--force" ]] && FORCE=true

# Skip if server cert already exists (unless --force)
if [[ "$FORCE" == false && -f "$SERVER_CERT" && -f "$SERVER_KEY" ]]; then
    echo "[gen_test_certs] Server certificate already exists — skipping (use --force to regenerate)"
    exit 0
fi

echo "[gen_test_certs] Using existing CA: ${CA_CERT}"

# ── Generate server key + CSR + CA-signed cert ─────────────────────────────
openssl genrsa -out "$SERVER_KEY" 2048 2>/dev/null

openssl req -new \
    -key "$SERVER_KEY" \
    -out "${SCRIPT_DIR}/httpbin.org.csr" \
    -subj "/CN=httpbin.org/C=SG/L=Singapore/O=JVJ 28 Inc." \
    2>/dev/null

# SAN extension for httpbin.org + localhost + 127.0.0.1
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

echo "[gen_test_certs] Done — server cert valid for ${DAYS_VALID} days"
