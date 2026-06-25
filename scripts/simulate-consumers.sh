#!/usr/bin/env bash
#
# Simulate many Platform Mesh consumers.
#
# Creates COUNT consumer workspaces in kcp, each of which:
#   1. binds the crypto-broker API (APIBinding)
#   2. creates a CryptoBroker instance (the "tenant's" broker)
#   3. (optional) deploys a minimal target app into the workspace's synced
#      namespace on the service cluster so the broker reaches the Ready state.
#
# Each workspace is mapped by the api-syncagent to its own namespace on the
# service cluster (the workspace's logical cluster id), so this exercises the
# real multi-tenant isolation model.
#
# Environment variables:
#   KCP_KUBECONFIG  Path to the kcp admin kubeconfig (required).
#   KCP_SERVER      kcp base server URL (default: https://kcp.api.portal.localhost:8443).
#   PROVIDER_PATH   Provider workspace path (default: root:providers:crypto-broker-provider).
#   COUNT           Number of consumer workspaces to create (default: 10).
#   PREFIX          Workspace name prefix (default: sim-consumer).
#   PROFILE         CryptoBroker profile (default: FIPS-140-3-128bit).
#   ENVIRONMENT     CryptoBroker environment (default: prod).
#   TARGET_DEPLOY   Target deployment name (default: crypto-broker-consumer-app).
#   WITH_APP        If "true", deploy a minimal target app per consumer (default: false).
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
KCP_SERVER="${KCP_SERVER:-https://kcp.api.portal.localhost:8443}"
PROVIDER_PATH="${PROVIDER_PATH:-root:providers:crypto-broker-provider}"
COUNT="${COUNT:-10}"
PREFIX="${PREFIX:-sim-consumer}"
PROFILE="${PROFILE:-FIPS-140-3-128bit}"
ENVIRONMENT="${ENVIRONMENT:-prod}"
TARGET_DEPLOY="${TARGET_DEPLOY:-crypto-broker-consumer-app}"
WITH_APP="${WITH_APP:-false}"

# Shared with the Taskfile's consumer-app:* tasks.
export KCP_KUBECONFIG KCP_SERVER PROVIDER_PATH

ROOT_SERVER="${KCP_SERVER}/clusters/root"

kc() { KUBECONFIG="${KCP_KUBECONFIG}" kubectl "$@"; }

echo "[*] Simulating ${COUNT} consumer(s) with prefix '${PREFIX}' (WITH_APP=${WITH_APP})"

for i in $(seq 1 "${COUNT}"); do
  ws="${PREFIX}-${i}"
  ws_server="${KCP_SERVER}/clusters/root:${ws}"
  echo "[*] (${i}/${COUNT}) workspace ${ws}"

  # 1. Create the consumer workspace under :root.
  kc create-workspace "${ws}" --ignore-existing --server="${ROOT_SERVER}" >/dev/null

  # 2. Bind the crypto-broker API into the workspace (shared with the Taskfile).
  WS_SERVER="${ws_server}" "${SCRIPT_DIR}/bind-crypto-broker-api.sh" >/dev/null

  # 3. Wait until the API is available (binding accepted), then create the
  #    CryptoBroker instance (shared with the Taskfile).
  if ! WS_SERVER="${ws_server}" NAME="broker" PROFILE="${PROFILE}" \
       TARGET_DEPLOY="${TARGET_DEPLOY}" ENVIRONMENT="${ENVIRONMENT}" \
       "${SCRIPT_DIR}/create-crypto-broker.sh"; then
    echo "    [!] APIBinding not ready in time for ${ws}; skipping CryptoBroker." >&2
    continue
  fi

  # 4. Optionally deploy a minimal target app into the synced namespace so the
  #    broker can reach the Ready state.
  if [ "${WITH_APP}" = "true" ]; then
    # Resolve the workspace's synced namespace (shared with the Taskfile).
    cluster="$("${SCRIPT_DIR}/resolve-consumer-namespace.sh" "${ws}")"
    if [ -z "${cluster}" ]; then
      echo "    [!] Could not resolve synced namespace for ${ws}; skipping app." >&2
      continue
    fi

    # Wait for the syncagent to create the namespace on the service cluster.
    ns_ready="false"
    for _ in $(seq 1 30); do
      if kubectl get namespace "${cluster}" >/dev/null 2>&1; then
        ns_ready="true"
        break
      fi
      sleep 2
    done

    if [ "${ns_ready}" != "true" ]; then
      echo "    [!] Synced namespace ${cluster} did not appear; skipping app." >&2
      continue
    fi

    cat <<EOF | kubectl apply -n "${cluster}" -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${TARGET_DEPLOY}
  labels:
    app: ${TARGET_DEPLOY}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ${TARGET_DEPLOY}
  template:
    metadata:
      labels:
        app: ${TARGET_DEPLOY}
    spec:
      containers:
        - name: app
          image: registry.k8s.io/pause:3.9
EOF
    echo "    [+] target app ${TARGET_DEPLOY} deployed in namespace ${cluster}"
  fi
done

echo "[+] Done. Created ${COUNT} consumer workspace(s)."
echo "[i] Inspect with: kubectl get cryptobrokers -A"
