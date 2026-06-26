#!/usr/bin/env bash
#
# Delete a CryptoBroker from a kcp consumer workspace (reverse of
# create-crypto-broker.sh). Removing the CryptoBroker is what makes the operator
# strip the injected crypto-broker sidecar from the target Deployment again, so
# this is the meaningful first step when tearing a consumer app down.
#
# Environment:
#   KCP_KUBECONFIG  Path to the kcp admin kubeconfig (required).
#   WS_SERVER       kcp server URL for the consumer workspace cluster (required).
#   NAME            CryptoBroker resource name (default: broker).
#   NAMESPACE       Workspace namespace it lives in (default: default).
#
# Idempotent: a missing CryptoBroker is treated as success.
set -euo pipefail

KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
WS_SERVER="${WS_SERVER:?WS_SERVER must be set}"
NAME="${NAME:-broker}"
NAMESPACE="${NAMESPACE:-default}"

echo "[*] Deleting CryptoBroker '${NAME}' from workspace namespace '${NAMESPACE}'..."
KUBECONFIG="${KCP_KUBECONFIG}" kubectl delete cryptobroker "${NAME}" \
  -n "${NAMESPACE}" --server="${WS_SERVER}" --ignore-not-found
echo "[+] CryptoBroker '${NAME}' removed (if it existed)."
