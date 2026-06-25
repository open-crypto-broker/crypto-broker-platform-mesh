#!/usr/bin/env bash
#
# Ensure the crypto-broker APIBinding exists in a kcp consumer workspace.
#
# Idempotent: if any APIBinding in the workspace already references the
# open-crypto-broker.io export (e.g. one created by a portal/marketplace
# subscription, often named 'open-crypto-broker.io-open-crypto-broker.io'), it is
# reused. Creating a second binding to the same export would surface a duplicate
# nav entry in the WebUI, so a new binding is only created when none references
# the export yet.
#
# Environment:
#   KCP_KUBECONFIG  Path to the kcp admin kubeconfig (required).
#   WS_SERVER       kcp server URL for the consumer workspace cluster (required),
#                   e.g. https://kcp.api.portal.localhost:8443/clusters/root:my-ws
#   PROVIDER_PATH   Provider workspace path
#                   (default: root:providers:crypto-broker-provider).
set -euo pipefail

KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
WS_SERVER="${WS_SERVER:?WS_SERVER must be set}"
PROVIDER_PATH="${PROVIDER_PATH:-root:providers:crypto-broker-provider}"

existing="$(KUBECONFIG="${KCP_KUBECONFIG}" kubectl get apibindings \
  --server="${WS_SERVER}" \
  -o jsonpath='{range .items[?(@.spec.reference.export.name=="open-crypto-broker.io")]}{.metadata.name}{"\n"}{end}' \
  2>/dev/null | head -n1)"

if [ -n "${existing}" ]; then
  echo "[*] Reusing existing APIBinding '${existing}' (already bound to open-crypto-broker.io)."
  exit 0
fi

echo "[*] No existing APIBinding found; creating 'open-crypto-broker.io'..."
cat <<EOF | KUBECONFIG="${KCP_KUBECONFIG}" kubectl apply --server="${WS_SERVER}" -f -
apiVersion: apis.kcp.io/v1alpha1
kind: APIBinding
metadata:
  name: open-crypto-broker.io
spec:
  reference:
    export:
      name: open-crypto-broker.io
      path: "${PROVIDER_PATH}"
  permissionClaims:
    - resource: namespaces
      state: Accepted
      all: true
EOF
