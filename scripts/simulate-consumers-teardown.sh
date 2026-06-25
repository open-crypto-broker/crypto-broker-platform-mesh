#!/usr/bin/env bash
#
# Tear down consumer workspaces created by simulate-consumers.sh.
#
# Deletes the simulated consumer workspaces in kcp. The api-syncagent removes
# the associated synced namespaces (and their resources) on the service cluster
# once the source workspaces are gone.
#
# Environment variables:
#   KCP_KUBECONFIG  Path to the kcp admin kubeconfig (required).
#   KCP_SERVER      kcp base server URL (default: https://localhost:8443).
#   COUNT           Number of consumer workspaces to delete (default: 10).
#   PREFIX          Workspace name prefix (default: sim-consumer).
#
set -euo pipefail

KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
KCP_SERVER="${KCP_SERVER:-https://localhost:8443}"
COUNT="${COUNT:-10}"
PREFIX="${PREFIX:-sim-consumer}"

ROOT_SERVER="${KCP_SERVER}/clusters/root"

kc() { KUBECONFIG="${KCP_KUBECONFIG}" kubectl "$@"; }

echo "[*] Deleting ${COUNT} consumer workspace(s) with prefix '${PREFIX}'"

for i in $(seq 1 "${COUNT}"); do
  ws="${PREFIX}-${i}"
  echo "[*] (${i}/${COUNT}) deleting workspace ${ws}"
  kc delete workspace "${ws}" --server="${ROOT_SERVER}" --ignore-not-found >/dev/null 2>&1 || true
done

echo "[+] Done. Deleted ${COUNT} consumer workspace(s)."
