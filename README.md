# Crypto Broker Integration with Platform Mesh

Integration of the [Crypto Broker](https://github.com/open-crypto-broker) with
[Platform Mesh](https://platform-mesh.io). This repository packages everything
needed to offer the Crypto Broker as a Platform Mesh "app": a Kubernetes
operator, the kcp control-plane manifests (APIExport, provider metadata, portal
UI), the api-syncagent wiring, crypto profiles, and a consumer workload.

## Prerequisites

The Platform Mesh environment (the kind service cluster **and** kcp) is **not**
provided by this repository. Bring it up first with the separate
[`platform-mesh/helm-charts`](https://github.com/platform-mesh) local-setup:

```bash
# In your platform-mesh/helm-charts folder
task local-setup
```

This creates the kind cluster `platform-mesh`, starts kcp (at
`https://kcp.api.portal.localhost:8443`), and writes the admin kubeconfig to
`.secret/kcp/admin.kubeconfig`.

The tasks here need to find the generated kubeconfig from Platform Mesh. If your
checkout lives elsewhere, set **one** of the following in the `.env` file
(no Taskfile edits required):

- `PM_HELM_CHARTS_DIR` — path to your `platform-mesh/helm-charts` checkout; the
  kubeconfig path is derived from it, or
- `KCP_KUBECONFIG` — the full path to the admin kubeconfig directly (takes
  precedence).

For a one-off run, pass it inline, e.g. `PM_HELM_CHARTS_DIR=/path task pm:setup`.
To set it persistently for your machine, copy `.env.example` to `.env` (which is
gitignored) and edit it there — the Taskfile loads `.env` automatically.

Required tooling: `kubectl`, the `kubectl-kcp` plugin, `helm`, `docker`, `kind`,
and `go` (to build the operator image).

## Quick start

```bash
# 1. Verify prerequisites (also run automatically by `task pm:setup`)
task pm:preflight

# 2. Make the operator image available in the kind cluster.
#    Contributors / local testing: build it from source and load it into kind:
task operator:build-local
#    (Alternatively, pull the published image instead of building:
#     docker pull ghcr.io/open-crypto-broker/operator:latest && \
#     kind load docker-image ghcr.io/open-crypto-broker/operator:latest --name platform-mesh)

# 3. Set up the provider (workspace + APIExport + operator + syncagent)
task pm:setup

# 4. Deploy a CryptoBroker in a consumer workspace, after an organization and workspace have been created. (WebUI-visible)
# For more options have a look at the task description or the docs/GUIDE.md file.
task consumer-app:deploy WORKSPACE=root:orgs:<org>:<workspace>

# 5. Check status
task pm:status

# Remove the provider when done
task pm:remove
```

Run `task -l` to list all available tasks. See [docs/GUIDE.md](docs/GUIDE.md)
for the full architecture overview, the CLI-vs-WebUI deployment paths, the
multi-tenant namespace model, and multi-consumer simulation.

## Repository layout

| Path | Contents |
| --- | --- |
| `operator/` | The `crypto-broker-operator` Go module (watches `CryptoBroker` CRs, injects the server sidecar). |
| `crds/` | The `CryptoBroker` CustomResourceDefinition. |
| `kcp/` | kcp control-plane manifests: APIExport, provider metadata, and portal ContentConfiguration. |
| `service-cluster/` | Operator deployment + RBAC, api-syncagent values/RBAC, and the PublishedResource. |
| `profiles/` | Crypto profile catalog (Default, FIPS-140-3-*, KSA-*) loaded into the operator's ConfigMap. |
| `consumer-app/` | Consumer workload Deployment and a `CryptoBroker` instance targeting it. |
| `scripts/` | Multi-consumer simulation helpers. |
| `docs/` | The detailed deployment guide. |
| `Taskfile.yaml` | All deployment, operator, configuration, and consumer-app tasks. |
