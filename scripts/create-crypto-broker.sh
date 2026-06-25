#!/usr/bin/env bash
#
# Create (or update) a CryptoBroker in a kcp consumer workspace.
#
# The APIBinding created just before this may take a moment to become
# established (until then the CryptoBroker CRD is not yet served in the
# workspace), so the apply is retried until it succeeds or RETRIES is exhausted.
# The api-syncagent then projects the CryptoBroker down onto the service cluster
# and syncs its status back up, which is what makes it visible in the WebUI.
#
# Environment:
#   KCP_KUBECONFIG  Path to the kcp admin kubeconfig (required).
#   WS_SERVER       kcp server URL for the consumer workspace cluster (required).
#   NAME            CryptoBroker resource name (default: broker).
#   NAMESPACE       Workspace namespace to create it in (default: default).
#   PROFILE         Crypto profile (default: FIPS-140-3-128bit).
#   TARGET_DEPLOY   Target Deployment name (default: crypto-broker-consumer-app).
#   ENVIRONMENT     CryptoBroker environment (default: dev).
#   RETRIES         Number of apply attempts (default: 30).
#   RETRY_DELAY     Seconds between attempts (default: 2).
#
# Exits 0 once the CryptoBroker is applied, or 1 if the binding never became
# ready in time.
set -euo pipefail

KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
WS_SERVER="${WS_SERVER:?WS_SERVER must be set}"
NAME="${NAME:-broker}"
NAMESPACE="${NAMESPACE:-default}"
PROFILE="${PROFILE:-FIPS-140-3-128bit}"
TARGET_DEPLOY="${TARGET_DEPLOY:-crypto-broker-consumer-app}"
ENVIRONMENT="${ENVIRONMENT:-dev}"
RETRIES="${RETRIES:-30}"
RETRY_DELAY="${RETRY_DELAY:-2}"

for _ in $(seq 1 "${RETRIES}"); do
  if cat <<EOF | KUBECONFIG="${KCP_KUBECONFIG}" kubectl apply --server="${WS_SERVER}" -f - >/dev/null 2>&1
apiVersion: open-crypto-broker.io/v1alpha1
kind: CryptoBroker
metadata:
  name: ${NAME}
  namespace: ${NAMESPACE}
spec:
  profile: ${PROFILE}
  targetDeployment: ${TARGET_DEPLOY}
  environment: ${ENVIRONMENT}
EOF
  then
    exit 0
  fi
  sleep "${RETRY_DELAY}"
done

echo "[!] APIBinding did not become ready in time; CryptoBroker '${NAME}' not created." >&2
exit 1
