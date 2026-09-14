# crypto-broker-operator Helm chart

Deploys the **crypto-broker operator** onto a service cluster.
The operator watches `CryptoBroker` custom resources and injects the `crypto-broker-server` sidecar into the Deployment named in each resource's `spec.targetDeployment`.
The chart does **not** run a standalone server — the server only ever runs as an injected sidecar.

## What the chart installs

- The `CryptoBroker` CustomResourceDefinition (`crds/`).
- The operator `Deployment`, its `ServiceAccount`, and a cluster-scoped `ClusterRole` / `ClusterRoleBinding`.
- The `crypto-broker-profile-catalog` `ConfigMap`, built from `files/profiles/*.yaml`, mounted into the operator at `PROFILES_CATALOG_DIR`.
- Optionally the target `Namespace` (`namespace.create`).

This chart is the single source of truth for the CRD and the profile catalog, the operator image build also reads the profiles from `files/profiles/`.

## Install

From a checkout:

```shell
helm upgrade --install crypto-broker-operator charts/crypto-broker-operator \
  --namespace crypto-broker-provider --create-namespace
```

From the published OCI registry:

```shell
helm upgrade --install crypto-broker-operator \
  oci://ghcr.io/open-crypto-broker/charts/crypto-broker-operator \
  --version <chart-version> \
  --namespace crypto-broker-provider --create-namespace
```

## Key values

| Value | Default | Purpose |
| --- | --- | --- |
| `operator.image.repository` / `.tag` | `ghcr.io/open-crypto-broker/operator` / `latest` | Operator image |
| `serverImage` | `ghcr.io/open-crypto-broker/server:latest` | Server image the operator injects (`CRYPTO_BROKER_SERVER_IMAGE`) |
| `namespace.name` / `.create` | `crypto-broker-provider` / `true` | Target namespace |
| `profiles.create` / `.configMapName` | `true` / `crypto-broker-profile-catalog` | Profile catalog ConfigMap |
| `serviceAccount.create` / `rbac.create` | `true` / `true` | RBAC toggles |

## CRD lifecycle

Helm installs CRDs under `crds/` only on first install and never upgrades or deletes them.
To update the `CryptoBroker` CRD after a schema change, apply it manually:

```shell
kubectl apply -f charts/crypto-broker-operator/crds/cryptobroker-crd.yaml
```

## Releasing

The chart and the operator image are published to GHCR by two separate, tag-triggered workflows.
They are independent: bump and tag whichever one changed (a chart-only change does not require a new image, and vice versa).

| Artifact | Workflow | Tag pattern | Published to |
| --- | --- | --- | --- |
| Operator image | [`release-operator-image.yaml`](../../.github/workflows/release-operator-image.yaml) | `operator-v*` (e.g. `operator-v0.1.0`) | `ghcr.io/open-crypto-broker/operator` (`:latest`, `:<version>`, `:sha-<commit>`) |
| Helm chart | [`release-helm-chart.yaml`](../../.github/workflows/release-helm-chart.yaml) | `operator-chart-v*` (e.g. `operator-chart-v0.1.0`) | `oci://ghcr.io/open-crypto-broker/charts/crypto-broker-operator` |

```shell
# Release the operator image
git tag operator-v0.1.0 && git push origin operator-v0.1.0

# Release the chart
git tag operator-chart-v0.1.0 && git push origin operator-chart-v0.1.0
```

The image build reuses the same named `profiles` build context as `task operator:build`, so the published image bundles the profile catalog from `files/profiles/`.
