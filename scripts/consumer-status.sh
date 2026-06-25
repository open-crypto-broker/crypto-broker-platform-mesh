#!/usr/bin/env bash
#
# Inspect the status of a consumer app + its CryptoBroker for a single kcp
# consumer workspace. Shows both sides of the sync:
#
#   * kcp workspace side  - the CryptoBroker as the user/portal sees it.
#   * service cluster side - the synced CryptoBroker, the target Deployment,
#     its pods, whether the crypto-broker sidecar was injected, and recent
#     events / app logs.
#
# This is a read-only diagnostic; it never creates or mutates resources.
#
# Usage:
#   consumer-status.sh <workspace>
#
# <workspace> may be a direct child of :root (e.g. "my-ws") or a full colon
# path (e.g. "root:orgs:acme:my-ws"), matching the other consumer scripts.
#
# Environment:
#   KCP_KUBECONFIG  Path to the kcp admin kubeconfig (required).
#   KCP_SERVER      kcp base URL (default: https://kcp.api.portal.localhost:8443).
#   NAME            CryptoBroker name to highlight (optional; otherwise all are shown).
#   NAMESPACE       Workspace namespace the CryptoBroker lives in (default: default).
#   TARGET_DEPLOY   Target Deployment name (default: crypto-broker-consumer-app).
#   LOG_LINES       Number of app log lines to show (default: 10).
set -euo pipefail

WS_INPUT="${1:?usage: consumer-status.sh <workspace>}"
KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
KCP_SERVER="${KCP_SERVER:-https://kcp.api.portal.localhost:8443}"
NAME="${NAME:-}"
NAMESPACE="${NAMESPACE:-default}"
TARGET_DEPLOY="${TARGET_DEPLOY:-crypto-broker-consumer-app}"
LOG_LINES="${LOG_LINES:-10}"

# Normalize the workspace to a full root-prefixed path for the kcp server URL,
# mirroring the Taskfile's WORKSPACE_PATH logic.
case "$WS_INPUT" in
  root|root:*) WORKSPACE_PATH="$WS_INPUT" ;;
  *)           WORKSPACE_PATH="root:$WS_INPUT" ;;
esac
WS_SERVER="${KCP_SERVER}/clusters/${WORKSPACE_PATH}"

# Resolve the synced service-cluster namespace (the workspace's logical-cluster
# id). Reuses the shared resolver so the lookup logic stays DRY.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER="$(KCP_KUBECONFIG="$KCP_KUBECONFIG" KCP_SERVER="$KCP_SERVER" \
  bash "${SCRIPT_DIR}/resolve-consumer-namespace.sh" "$WS_INPUT")"

echo "=== Workspace ==="
echo "  input:      ${WS_INPUT}"
echo "  path:       ${WORKSPACE_PATH}"
if [ -z "$CLUSTER" ]; then
  echo "  namespace:  <not found>"
  echo
  echo "[!] Workspace '${WS_INPUT}' could not be resolved to a service-cluster"
  echo "    namespace. Does it exist? Create it via the Platform Mesh WebUI or"
  echo "    'kubectl create-workspace', then re-run this task."
  exit 1
fi
echo "  namespace:  ${CLUSTER}"
echo

echo "=== CryptoBroker (kcp workspace side) ==="
KUBECONFIG="$KCP_KUBECONFIG" kubectl get cryptobrokers -n "$NAMESPACE" \
  --server="$WS_SERVER" -o wide 2>&1 \
  || echo "  (could not list CryptoBrokers in the workspace)"
echo

echo "=== CryptoBroker (service cluster, synced) ==="
kubectl get cryptobrokers -n "$CLUSTER" -o wide 2>&1 \
  || echo "  (no CryptoBroker synced into namespace ${CLUSTER} yet)"
echo

# Show the broker's status conditions/messages, which carry the operator's view
# (e.g. profile mismatch, target Deployment not found, sidecar injection state).
if [ -n "$NAME" ]; then
  echo "=== CryptoBroker '${NAME}' status conditions (service cluster) ==="
  kubectl get cryptobroker -n "$CLUSTER" -o yaml 2>/dev/null \
    | sed -n '/status:/,$p' | head -40 \
    || echo "  (no status available)"
  echo
fi

echo "=== Target Deployment '${TARGET_DEPLOY}' ==="
kubectl get deploy "$TARGET_DEPLOY" -n "$CLUSTER" -o wide 2>&1 \
  || echo "  (Deployment ${TARGET_DEPLOY} not found in namespace ${CLUSTER})"
echo

echo "=== Pods ==="
kubectl get pods -n "$CLUSTER" -l "app=${TARGET_DEPLOY}" -o wide 2>&1 \
  || echo "  (no pods found for app=${TARGET_DEPLOY})"
echo

echo "=== Containers (sidecar injection check) ==="
CONTAINERS="$(kubectl get pod -n "$CLUSTER" -l "app=${TARGET_DEPLOY}" \
  -o jsonpath='{range .items[0].spec.containers[*]}{.name}{"\n"}{end}' 2>/dev/null || true)"
if [ -z "$CONTAINERS" ]; then
  echo "  (no running pod to inspect)"
else
  echo "$CONTAINERS" | sed 's/^/  - /'
  if echo "$CONTAINERS" | grep -q 'crypto-broker-server'; then
    echo "  [+] crypto-broker-server sidecar present (operator injected it)."
  else
    echo "  [!] crypto-broker-server sidecar MISSING. The operator did not inject"
    echo "      it - usually the CryptoBroker is not synced to the service cluster"
    echo "      (check the api-syncagent) or its targetDeployment does not match"
    echo "      '${TARGET_DEPLOY}'."
  fi
fi
echo

echo "=== Recent events (namespace ${CLUSTER}) ==="
kubectl get events -n "$CLUSTER" --sort-by=.lastTimestamp 2>&1 | tail -12 \
  || echo "  (no events)"
echo

echo "=== App logs (last ${LOG_LINES} lines, container cli-hash) ==="
kubectl logs -n "$CLUSTER" -l "app=${TARGET_DEPLOY}" -c cli-hash \
  --tail="$LOG_LINES" 2>&1 \
  || echo "  (no logs available)"
