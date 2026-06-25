#!/usr/bin/env bash
#
# Resolve a consumer kcp workspace to the service-cluster namespace that its
# resources are synced into (the workspace's logical-cluster id).
#
# Usage:
#   resolve-consumer-namespace.sh <workspace>
#
# <workspace> may be a direct child of :root (e.g. "my-ws") or a full colon path
# (e.g. "root:orgs:acme:my-ws"). The last colon-separated segment is the
# workspace name; everything before it is the parent cluster path.
#
# Environment:
#   KCP_KUBECONFIG   Path to the kcp admin kubeconfig (required).
#   KCP_SERVER       kcp base URL (default: https://localhost:8443).
#
# Prints the synced namespace (logical-cluster id) to stdout, or nothing if the
# workspace does not exist. Always exits 0 so callers can decide how to handle a
# missing workspace.
set -euo pipefail

WS_INPUT="${1:?usage: resolve-consumer-namespace.sh <workspace>}"
KCP_KUBECONFIG="${KCP_KUBECONFIG:?KCP_KUBECONFIG must be set}"
KCP_SERVER="${KCP_SERVER:-https://localhost:8443}"

# Split the workspace into a leaf name + parent cluster path. The last
# colon-separated segment is the workspace name; everything before it is the
# parent path, normalized to start with the literal "root".
case "$WS_INPUT" in
  *:*)
    LEAF="${WS_INPUT##*:}"
    PARENT="${WS_INPUT%:*}"
    case "$PARENT" in
      root|root:*) ;;
      *) PARENT="root:$PARENT" ;;
    esac
    ;;
  *)
    LEAF="$WS_INPUT"
    PARENT="root"
    ;;
esac

KUBECONFIG="$KCP_KUBECONFIG" kubectl get workspace "$LEAF" \
  --server="${KCP_SERVER}/clusters/${PARENT}" \
  -o jsonpath='{.spec.cluster}' 2>/dev/null || true
