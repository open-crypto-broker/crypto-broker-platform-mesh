# Platform Mesh Deployment Guide

This guide explains how to deploy and test the Crypto Broker service in Platform Mesh, including deploying a sample application that uses it.

## Architecture Overview

Platform Mesh uses a **kcp (Kubernetes-like Control Plane) provider/consumer model** with three interacting layers:

```ascii
┌─────────────────────────────────────────────────────────────────────────┐
│                        kcp Control Plane                                │
│                                                                         │
│  ┌──────────────────────────────────────┐  ┌────────────────────────┐   │
│  │   Provider Workspace                 │  │  Consumer Workspace    │   │
│  │   root:providers:crypto-broker-...   │  │  (your app workspace)  │   │
│  │                                      │  │                        │   │
│  │  • APIExport (open-crypto-broker.io) │  │  • APIBinding          │   │
│  │  • ProviderMetadata                  │  │  • CryptoBroker CR     │   │
│  │  • ContentConfiguration (UI)         │  │                        │   │
│  └──────────────────────────────────────┘  └────────────────────────┘   │
│                    │                                    │               │
│                    │         api-syncagent              │               │
│                    │         (bidirectional sync)       │               │
└────────────────────┼────────────────────────────────────┼───────────────┘
                     │                                    │
                     ▼                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     Service Cluster (Kubernetes)                        │
│                                                                         │
│  ┌──────────────────────────────┐  ┌─────────────────────────────────┐  │
│  │  crypto-broker-operator      │  │  Your Application Pod           │  │
│  │                              │  │                                 │  │
│  │  Watches CryptoBroker CRs    │  │  ┌───────────┐ ┌────────────┐   │  │
│  │  Injects sidecar into target │─▶│  │  App      │ │ CB Server  │   │  │
│  │  Manages profile ConfigMaps  │  │  │  Container│ │ (sidecar)  │   │  │
│  │                              │  │  │           │ │            │   │  │
│  └──────────────────────────────┘  │  │     ◄──unix socket──►    │   │  │
│                                    │  │  /tmp/open-crypto-broker/│   │  │
│  ┌──────────────────────────────┐  │  └───────────┘ └────────────┘   │  │
│  │  api-syncagent               │  │                                 │  │
│  │  Syncs CRs between kcp ←→ k8s│  └─────────────────────────────────┘  │
│  └──────────────────────────────┘                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

### Layer Interactions

| Layer | Component | Role |
| --- | --- | --- |
| **kcp (Control Plane)** | APIExport | Exposes `open-crypto-broker.io` API group to consumers |
| **kcp (Control Plane)** | ContentConfiguration | Defines the Platform Mesh portal UI (list/detail/create views) |
| **kcp (Control Plane)** | ProviderMetadata | Marketplace listing (name, description, contacts) |
| **Service Cluster** | api-syncagent | Syncs `CryptoBroker` CRs bidirectionally between kcp and the cluster |
| **Service Cluster** | crypto-broker-operator | Watches `CryptoBroker` CRs, injects server sidecar into target Deployments |
| **Service Cluster** | CryptoBroker CRD | Defines the custom resource schema |
| **Application Pod** | crypto-broker-server (sidecar) | Provides cryptographic operations via Unix socket |
| **Application Pod** | App container(s) | Connect to the broker at `/tmp/open-crypto-broker/crypto-broker-server.sock` |

### How Sidecar Injection Works

1. A `CryptoBroker` CR is created (either via kcp/portal or directly in the cluster)
2. The operator reads the CR's `spec.profile` and loads the corresponding profile from its catalog
3. The operator creates a `ConfigMap` with the profile content (`Profiles.yaml`)
4. The operator patches the target Deployment to add:
   - A `crypto-broker-server` sidecar container (with profile + socket volumes)
   - A shared `emptyDir` volume (`crypto-broker-socket`) mounted at `/tmp/open-crypto-broker/`
   - The socket volume mount is injected into **all existing containers** so they can access the broker
5. The server starts and creates the Unix socket; app containers connect via gRPC

### Security Model: Trust Boundary

The Crypto Broker's security concept relies on the assumption that the
application/client library talks to the broker **only over a Unix Domain Socket
(UDS)**, never over the network — so the app to broker hop needs no TLS or other
in-transit protection. **This assumption also holds for Platform Mesh.** It is
enforced by how the operator injects the sidecar, not just by convention:

- **Single pod, in-process path.** The `crypto-broker-server` is injected as a
  **sidecar container in the same pod** as the application. The injected sidecar
  declares **no `ports` / `containerPort`**, and there is **no `Service`, no
  `hostNetwork`, and no TCP listener** anywhere in the manifests. The only
  channel is the UDS at `/tmp/open-crypto-broker/crypto-broker-server.sock`,
  shared via a pod-local `emptyDir` volume. The socket is unreachable from other
  pods, other tenants' namespaces, or the network.
- **Multi-tenant isolation is preserved.** Each consumer workspace maps to its
  own service-cluster namespace and its own pod (see the namespace model below),
  so one tenant's socket is never visible to another tenant.

What changes versus a VM/Cloud Foundry deployment is **where the trust boundary
sits**: it moves from "the same instance/process group" to **"the same
Kubernetes pod."**

- **All containers co-located in the app pod are trusted.** The operator
  deliberately mounts the socket volume into **every** container in the pod, so
  any sibling container can use the broker. A compromised sidecar/sibling
  container is therefore in-scope and must be treated as trusted — the direct
  analogue of "co-located processes are trusted" in the VM model.
- **Node-level access is out of the in-pod model.** The socket lives on a
  disk-backed `emptyDir` in the kubelet's pod directory (not `medium: Memory`).
  A **root user on the node**, or a privileged pod mounting the node filesystem
  via `hostPath`, could reach it. This is a node/cluster-admin threat, identical
  to any on-node UDS.
- **Sidecar hardening.** The injected server runs as non-root (uid 1000) with
  `readOnlyRootFilesystem: true` and `allowPrivilegeEscalation: false`.

---

## Step-by-Step: Deploy Everything Together

### Prerequisites

> Important:
>
> The Platform Mesh environment (the kind service cluster **and** kcp) is **not** provided by this repository. It is brought up by the separate **platform-mesh/helm-charts** local-setup. You must run that first, otherwise `task pm:setup` will fail because kcp or the cluster is not available.
>
> ```bash
> # In your platform-mesh/helm-charts folder
> task local-setup
> ```
>
> This creates the kind cluster named `platform-mesh`, starts kcp (exposed at `https://kcp.api.portal.localhost:8443`), and writes the admin kubeconfig to `.secret/kcp/admin.kubeconfig`. See the helm-charts `local-setup/README.md` for full instructions and prerequisites (Docker/Podman, kind, helm, kubectl, the kubectl-kcp plugin, mkcert).

Before running the setup, the following must be in place:

- **platform-mesh local-setup completed** — kind service cluster `platform-mesh` and kcp are running (`task local-setup` in platform-mesh/helm-charts)
- kcp admin kubeconfig available at `<path-to-platform-mesh>/platform-mesh/helm-charts/.secret/kcp/admin.kubeconfig`. If your helm-charts checkout lives elsewhere, set `PM_HELM_CHARTS_DIR=<path>` (the kubeconfig path is derived from it) or `KCP_KUBECONFIG=<path>` (the full path, takes precedence). For a persistent per-machine setting, copy `.env.example` to `.env` (gitignored) and edit it there — the Taskfile loads `.env` automatically.
- `kubectl` current context is `kind-platform-mesh`
- `kubectl-kcp` plugin and `helm` installed
- Operator image available in the kind cluster — build it from source with `task operator:build-local` (for contributors / local testing), or pull `ghcr.io/open-crypto-broker/operator:latest` and `kind load` it
- CLI image available (`ghcr.io/open-crypto-broker/cli-go:latest`)

You can verify all of the above at once with the preflight check, which `task pm:setup` also runs automatically and which fails fast with an actionable message if anything is missing:

```bash
task pm:preflight
```

### Step 1: Set Up the Platform Mesh Provider

This creates the kcp workspace, deploys the operator, syncagent, and registers the API. It first runs `pm:preflight` to verify the prerequisites above.

```bash
task pm:setup
```

`pm:setup` orchestrates several internal steps (create the kcp provider
workspace, deploy the operator + CRD + profiles, deploy the syncagent, publish
the resource, and register the API + UI). They are intentionally not exposed as
separate public tasks — run `pm:setup` and let it sequence them.

### Step 2: Deploy the Crypto Broker with a Consumer App

Everything needed to attach a broker to an app in a consumer workspace is done by
a **single task**, `consumer-app:deploy`. It resolves the workspace's synced
namespace, deploys the consumer app into it, binds the crypto-broker API, and
creates the `CryptoBroker` — in the right order — so the result is visible in the
Platform Mesh portal.

```bash
task consumer-app:deploy WORKSPACE=root:orgs:<organization>:<user> PROFILE=FIPS-140-3-256bit
```

> Important:
>
> The workspace must **already exist** — the task never creates it. Create the
> organization / workspace first via the Platform Mesh WebUI (the operator wires
> up the backing `Account` and IAM), or with `kubectl create-workspace` for a
> plain kcp workspace. If the workspace is missing, the task exits with an error.

`WORKSPACE` accepts either a **direct child of `:root`** (e.g. `WORKSPACE=my-ws`)
or a **full nested path** (e.g. `WORKSPACE=root:orgs:<organization>:<user>`).

| Var | Default | Purpose |
| --- | --- | --- |
| `WORKSPACE` | _(required)_ | The consumer kcp workspace the broker/app live in |
| `PROFILE` | `FIPS-140-3-128bit` | Crypto profile applied to **both** the app and the broker (they must match) |
| `NAME` | `consumer-app-with-broker` | Name of the `CryptoBroker` resource |
| `DEPLOY_APP` | `true` | Deploy the bundled consumer app; set `false` to target an existing Deployment |
| `TARGET_DEPLOY` | `crypto-broker-consumer-app` | Deployment the broker injects the sidecar into |
| `ENVIRONMENT` | `dev` | Value written to the `CryptoBroker` spec |

The bundled consumer app is a Deployment with two Go-CLI containers:

- `cli-hash`: continuously hashes data (`--loop 500`)
- `cli-sign`: continuously signs data (`--loop 700`)

Both wait for the crypto-broker socket to become available (60s timeout with
retry). Once the broker is `Ready`, the operator has injected the
`crypto-broker-server` sidecar and mounted the shared socket volume into every
container.

If the target app is **already deployed** in the synced namespace, skip the app
deploy and just create the broker against the existing Deployment:

```bash
task consumer-app:deploy WORKSPACE=root:orgs:<organization>:<user> \
  DEPLOY_APP=false TARGET_DEPLOY=my-existing-app PROFILE=FIPS-140-3-256bit
```

### Step 3: Verify Everything Works

The `consumer-app:status` task diagnoses the consumer app and its
`CryptoBroker` end to end: it resolves the workspace's synced namespace, shows
the `CryptoBroker` on both the kcp (portal) and service-cluster side, the target
Deployment, pod status (3/3 containers ready: cli-hash, cli-sign, sidecar),
whether the sidecar was injected, recent events and app logs:

```bash
task consumer-app:status WORKSPACE=root:orgs:<organization>:<user>
```

Useful variables: `NAME`, `NAMESPACE` (default `default`), `TARGET_DEPLOY`
(default `crypto-broker-consumer-app`) and `LOG_LINES` (default `10`).

`task pm:status` also lists the broker in its workspace namespace.

### Step 4: Clean Up

The `consumer-app:remove` task reverses `consumer-app:deploy`: it deletes the
`CryptoBroker` from the workspace (so the operator strips the injected sidecar)
and then removes the consumer app Deployment from the workspace's synced
namespace:

```bash
task consumer-app:remove WORKSPACE=root:orgs:<organization>:<user>
```

Pass `DELETE_APP=false` to leave the app Deployment in place and only remove the
`CryptoBroker`.

---

## Running the CLI as a Long-Running Application

The Go CLI does **not** require `kubectl exec`. It runs as a proper long-running process using the `--loop <ms>` flag, which makes it continuously execute operations in a loop until `SIGTERM` is received.

This is the same pattern used in the Cloud Foundry deployment, where the CLI runs as the main process and sidecar processes simultaneously:

| Flag | Behavior |
| --- | --- |
| `--loop 500` | Execute the command every 500ms indefinitely |
| `--loop 700` | Execute the command every 700ms indefinitely |
| (no --loop) | Execute once and exit |

The CLI automatically connects to the broker via the Unix socket at `/tmp/open-crypto-broker/crypto-broker-server.sock`. The client library has built-in retry logic and a 60-second connection timeout, so the CLI containers will wait for the sidecar to be ready.

---

## Viewing Logs in the Platform Mesh Web UI

The Platform Mesh portal provides a **resource-level view** for CryptoBroker instances:

### What You Can See in the Portal

- **List view**: All CryptoBroker instances with Name, Profile, Target Deployment, and State
- **Detail view**: Full resource details including socket path and status messages
- **Create view**: Form to create new CryptoBroker instances with profile selection

### What the Portal Does NOT Show

The Platform Mesh portal shows **resource state** (the CryptoBroker CR status), not container logs. To view application/sidecar logs (in the workspace's synced namespace):

```bash
kubectl logs deploy/crypto-broker-consumer-app -c cli-hash -n <namespace>
kubectl logs deploy/crypto-broker-consumer-app -c cli-sign -n <namespace>
kubectl logs deploy/crypto-broker-consumer-app -c crypto-broker-server -n <namespace>
```

### CryptoBroker Status in the Portal

The operator updates the CR status which is visible in the portal:

| State | Meaning |
| --- | --- |
| `Provisioning` | Operator is processing the request |
| `Ready` | Sidecar injected, socket available |
| `Error` | Something went wrong (check `status.message`) |

---

## Attaching a Broker: the Namespace Model and the WebUI

Whether you attach a broker from the CLI (`consumer-app:deploy`) or from the
Platform Mesh portal, the same rule decides **which namespace** the
`CryptoBroker` and its target Deployment must live in. Getting this right is the
single most common source of confusion.

### The namespace model

- **kcp** is a control plane (an API surface), not a real cluster. Consumers
  create `CryptoBroker` resources in their **kcp workspace**.
- The **api-syncagent** copies each `CryptoBroker` _down_ onto the service
  cluster, into a **namespace generated per consumer workspace** — the
  workspace's logical cluster id (e.g. `1wchn0n7fayzf6ev`). This per-workspace
  namespace is what gives each tenant isolation.
- The **operator** is namespace-local: it looks for the target Deployment **in
  the same namespace as the `CryptoBroker`** and injects the sidecar there.

So the rule is: **the target Deployment must live in the same namespace as the
`CryptoBroker` that targets it.** A broker can never reach a Deployment in
another namespace.

```text
kcp consumer workspace  --(syncagent copies CR down)-->  service-cluster namespace <hash>
                                                          ├─ synced CryptoBroker
                                                          └─ target Deployment  ← must be here
```

### From the CLI: `consumer-app:deploy`

`consumer-app:deploy` (see Step 2 above) is the canonical way to attach a
broker from the command line. It creates the `CryptoBroker` **in a consumer kcp
workspace**, so the api-syncagent projects it down into the workspace's synced
namespace and syncs status back up — which is what makes it **visible in the
WebUI**. With `DEPLOY_APP=true` (the default) it also deploys the target app into
that namespace first, guaranteeing the namespace rule above is satisfied.

```bash
# app + broker in one step
task consumer-app:deploy WORKSPACE=root:orgs:<organization>:<user> PROFILE=FIPS-140-3-256bit

# broker only, against a Deployment you already have in the synced namespace
task consumer-app:deploy WORKSPACE=root:orgs:<organization>:<user> \
  DEPLOY_APP=false TARGET_DEPLOY=my-existing-app PROFILE=FIPS-140-3-256bit
```

Afterwards the broker appears in the portal under that account, and in
`task pm:status` (in the workspace's hashed namespace).

### From the WebUI portal

Creating a `CryptoBroker` in the portal is the GUI equivalent of the broker step
that `consumer-app:deploy` performs. The only requirement is the namespace rule above: the
**target Deployment must already exist in the workspace's synced namespace**,
because the operator does not watch Deployments.

The simplest way to satisfy that from the CLI is to let `consumer-app:deploy` deploy the
app (it lands it in the right namespace), then create the broker in the portal
targeting `crypto-broker-consumer-app`. If you prefer to find the namespace
yourself:

```bash
bash scripts/resolve-consumer-namespace.sh <your-workspace>
# prints the workspace's synced namespace, e.g. 1wchn0n7fayzf6ev
```

`<your-workspace>` accepts either a **direct child of `:root`** (e.g. `my-ws`) or
a **full nested path** (e.g. `root:orgs:<organization>:<user>`).

> Note:
>
> The generated namespace name is intentionally opaque (a per-workspace hash).
> This is the correct behaviour for multi-tenant isolation — it is **not**
> changed to a static name, because that would put every tenant's resources in
> one shared namespace.

### Walkthrough: demonstrate the single-owner guard

This proves that a **second** `CryptoBroker` targeting the same Deployment is
rejected — the socket / sidecar can only be owned by one `CryptoBroker`. You can
reproduce it by running `consumer-app:deploy` twice against the same Deployment.

1. **Deploy the app and the first broker in one step** into your workspace:

   ```bash
   task consumer-app:deploy WORKSPACE=<your-workspace> PROFILE=FIPS-140-3-128bit
   ```

   Confirm the first broker is `Ready` with the sidecar injected (3/3
   containers):

   ```bash
   task consumer-app:status WORKSPACE=<your-workspace>
   ```

2. **Deploy a second broker against the same app.** Run `consumer-app:deploy`
   again with a different `NAME` and `DEPLOY_APP=false` so it reuses the existing
   `crypto-broker-consumer-app` Deployment instead of redeploying it:

   ```bash
   task consumer-app:deploy WORKSPACE=<your-workspace> \
     NAME=second-broker DEPLOY_APP=false PROFILE=FIPS-140-3-256bit
   ```

   The operator detects the Deployment is already managed and sets this second CR
   to:

   ```text
   State:   Error
   Message: deployment "crypto-broker-consumer-app" is already managed by another
            CryptoBroker (active profile configmap: "crypto-broker-profile-<first>")
   ```

   The first broker keeps working untouched. `consumer-app:status` lists both
   CRs (first `Ready`, second `Error`):

   ```bash
   task consumer-app:status WORKSPACE=<your-workspace>
   ```

This single-owner guard is enforced by the operator: it only injects a sidecar
when the Deployment has none, and refuses any further `CryptoBroker` that points
at an already-injected Deployment.

---

## Simulating Many Tenants

To exercise the real multi-tenant model (e.g. hundreds of consumers, each with
their own isolated crypto-broker instance), use the simulation tasks. Each
consumer gets its own kcp workspace, `APIBinding`, and `CryptoBroker`, which the
syncagent maps to a dedicated namespace on the service cluster.

```bash
# Create 50 consumer workspaces, each with a CryptoBroker (control-plane only)
task consumer:simulate COUNT=50

# Also deploy a minimal target app per consumer so brokers reach Ready
# (heavier — one pod per consumer; keep COUNT modest on a local kind cluster)
task consumer:simulate COUNT=10 WITH_APP=true

# Inspect all synced brokers across every tenant namespace
kubectl get cryptobrokers -A

# Tear the simulated consumers down again
task consumer:simulate:remove COUNT=50
```

Available variables: `COUNT` (default 10), `PREFIX` (default `sim-consumer`),
`PROFILE`, `ENVIRONMENT`, `TARGET_DEPLOY`, `WITH_APP` (default `false`).

> Note:
>
> With `WITH_APP=false` (the default) the brokers will show `Error`
> (`deployment "crypto-broker-consumer-app" not found`) — this is expected, since
> no workload exists yet. It still demonstrates per-tenant workspace creation,
> API binding, and CR synchronization into isolated namespaces.

---

## Customizing the Sample App

The crypto profile is a **platform/deployment decision** made on the
`CryptoBroker`, not in app code. Pass `PROFILE=<name>` to `consumer-app:deploy`
and it is applied to **both** the broker and the app (the app reads it from the
`CRYPTO_BROKER_PROFILE` env var, which Kubernetes expands as
`$(CRYPTO_BROKER_PROFILE)` in the container command), so the two always match:

```bash
task consumer-app:deploy WORKSPACE=<your-workspace> PROFILE=FIPS-140-3-192bit
```

Available profiles include `FIPS-140-3-128bit`, `FIPS-140-3-192bit`,
`FIPS-140-3-256bit`, `Default`, and `KSA-MODERATE-2020`.
