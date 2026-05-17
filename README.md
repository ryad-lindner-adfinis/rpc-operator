# RPC Operator

> **Primary remote:** [forgejo.thecloudroute.com/tom/rpc-operator](https://forgejo.thecloudroute.com/tom/rpc-operator) — development, issues, and PRs live here.
> [github.com/insidegreen/rpc-operator](https://github.com/insidegreen/rpc-operator) is a read-only mirror (internal docs excluded). Do not commit directly on GitHub.

Kubernetes operator for [Redpanda Connect](https://docs.redpanda.com/redpanda-connect/) pipelines.
Data engineers configure pipelines visually or as YAML through an embedded web UI
and deploy them to a Kubernetes cluster with a single click.

## Local Development

### Prerequisites

| Tool | Version |
|---|---|
| Go | ≥ 1.24 |
| Node.js | ≥ 20 (for UI build) |
| kubectl | ≥ 1.11 |
| Kubernetes cluster | ≥ 1.11 (kubeconfig configured) |

### Quickstart

```bash
# 1. Install the CRDs into the cluster (once)
make install

# 2. Build the UI
make ui-build

# 3. Start the operator — uses the current kubeconfig context
go run ./cmd/main.go
```

The operator connects to the active kubeconfig context.
The web UI is then available at **http://localhost:8082**.

```bash
# Inspect / switch context
kubectl config current-context
kubectl config use-context <context-name>
```

### Operator CLI Flags

The operator is started with `go run ./cmd/main.go [flags]`. All flags have
sensible defaults — the ones below are the most commonly relevant in a dev setup.

| Flag | Default | Purpose |
|---|---|---|
| `--api-bind-address` | `:8082` | Address for the REST API + embedded UI. Empty (`""`) disables the API server (operator loop only). |
| `--health-probe-bind-address` | `:8081` | Liveness/readiness probes (`/healthz`, `/readyz`). |
| `--auth-enabled` | `true` | F43 master switch. `--auth-enabled=false` reproduces v0.7 behavior (no login, all requests via operator SA). Typically set to `false` for hot-reload dev. |
| `--prometheus-url` | _empty_ | Prometheus base URL for the throughput graph (F15). Empty disables only the graph; everything else still works. Example: `--prometheus-url=http://prometheus-operated.cattle-monitoring-system.svc:9090` |
| `--watch-namespaces` | _empty_ | F21 allowlist as a comma-separated list. Empty = cluster-wide (sees all pipelines). Example: `--watch-namespaces=rpc-operator-poc,default` |
| `--leader-elect` | `false` | For multi-replica setups. Leave off in single-pod dev. |
| `--metrics-bind-address` | `0` | Operator self-metrics (controller-runtime). `0` = off (default; the per-pipeline PodMonitor from F36 is unaffected). `:8443` enables HTTPS with authn/authz. |
| `--zap-log-level` | `debug` | Log level (`info`, `debug`, `error`); `opts.Development=true` in code sets the default to `debug`. |
| `--zap-encoder` | `console` | `console` (human-readable) or `json` (for structured logs). |

Typical dev startup with all relevant flags:

```bash
go run ./cmd/main.go \
  --auth-enabled=false \
  --watch-namespaces=rpc-operator-poc \
  --prometheus-url=http://prometheus-operated.cattle-monitoring-system.svc:9090
```

> **Production flags:** `--metrics-secure`, `--metrics-cert-*`, `--webhook-cert-*`, `--enable-http2` are intended for in-cluster Helm deployments and are normally not needed in dev. Full list: `go run ./cmd/main.go --help`.

### UI Development with Hot Reload

For frontend changes a separate Vite dev server is enough — no operator restart
required. Vite automatically proxies `/api` requests to `:8082`.

```bash
# Terminal 1 — operator
go run ./cmd/main.go

# Terminal 2 — Vite dev server (hot reload)
make ui-dev
# → http://localhost:5173
```

### Running Tests

```bash
# All Go tests
make test

# A single package
go test ./internal/render/...

# Linter
make lint

# TypeScript type check
cd ui && npx tsc --noEmit
```

### Test a Pipeline Manually

```bash
# Show deployed pipelines
kubectl get pipeline.rpc.operator.io -n rpc-operator-poc

# Pipeline pods
kubectl get pods -n rpc-operator-poc

# Pipeline logs
kubectl logs -n rpc-operator-poc <pod-name>

# Delete a pipeline
kubectl delete pipeline.rpc.operator.io <name> -n rpc-operator-poc
```

### Useful Make Targets

```bash
make help          # All targets with descriptions
make ui-build      # Build the React UI (→ internal/api/static/)
make ui-dev        # Start the Vite dev server
make build         # Build the Go binary including the UI (→ bin/manager)
make test          # Run Go tests
make lint          # golangci-lint
make install       # Install CRDs into the cluster
make uninstall     # Remove CRDs from the cluster
```

---

## Installation via Helm

```bash
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace \
  --set 'imagePullSecrets[0].name=forgejo-pull' \
  --set image.tag=main \
  --set examples.enabled=true
```

Pull secret for the private registry, configuration options, and a five-minute
quickstart: [`charts/rpc-operator/README.md`](charts/rpc-operator/README.md).

### Auth Modes

Starting with v0.8 the UI is protected by Bearer token auth by default. Three
modes, controlled via Helm values:

| Mode | Helm | Behavior |
|---|---|---|
| **A. Auth off** (v0.7 compatibility) | `--set auth.enabled=false` | No login; all requests run under the operator SA. **Never** combine with a public ingress. |
| **B. Token auth** (default v0.8) | `auth.enabled=true` (default) | Login with a K8s bearer token (paste or kubeconfig upload — only the token is extracted; cert-auth is rejected). The backend forwards the token per request to the apiserver; native K8s RBAC decides. |
| **C. + anonymous GETs** (F42) | `auth.enabled=true` + `anonymous.read.enabled=true` (+ optional `anonymous.logs.enabled=true`) | Like B; additionally allows GETs on pipelines/catalog/namespaces/metrics without a token (running under the operator SA within the F21 allowlist). Logs are only exposed if the separate `anonymous.logs.enabled` switch is set — log content can contain payloads/secrets. The UI shows a read-only banner and hides edit/deploy/delete buttons. |

Obtain a token for Mode B:

```bash
# 1) Create a per-user ServiceAccount and bind RBAC for pipelines
kubectl -n <namespace> create serviceaccount <user>

kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: rpc-pipeline-editor
  namespace: <namespace>
rules:
- apiGroups: ["rpc.operator.io"]
  resources: ["pipelines", "pipelines/status"]
  verbs: ["get","list","watch","create","update","patch","delete"]
- apiGroups: [""]
  resources: ["pods","pods/log","events","configmaps"]
  verbs: ["get","list","watch"]
EOF

kubectl -n <namespace> create rolebinding <user>-pipelines \
  --role=rpc-pipeline-editor --serviceaccount=<namespace>:<user>

# The F21 namespace-allowlist dropdown needs cluster-wide `list namespaces`:
kubectl create clusterrolebinding <user>-ns-list \
  --clusterrole=view --serviceaccount=<namespace>:<user>

# 2) Mint a token and paste it into the login screen
kubectl -n <namespace> create token <user> --duration=24h
```

> **Rancher clusters:** Tokens from a Rancher-issued kubeconfig
> (`kubeconfig-u-…:…`) are Rancher auth-proxy credentials and are not
> accepted by the apiserver directly (`token rejected by apiserver:
> Unauthorized`). Login only works with apiserver-native tokens — create a
> ServiceAccount per user as shown above. An SSO solution via a shared
> OIDC IdP will arrive with F20b (v1.0).

Enable Mode C (status-board / public-display use case):

```bash
# Anonymous reads, logs kept private:
helm install rpc-operator ./charts/rpc-operator \
  --set auth.enabled=true \
  --set anonymous.read.enabled=true

# Plus anonymous live logs:
helm install rpc-operator ./charts/rpc-operator \
  --set auth.enabled=true \
  --set anonymous.read.enabled=true \
  --set anonymous.logs.enabled=true
```

> **Security warning (F42):** Anonymous reads expose pipeline specs including `spec.secretRefs` names (not values) and `spec.rawYAML`. Pipelines with sensitive Bloblang mappings may leak metadata in the process. F42 is intended for demo / status-board clusters, not for production pipelines with compliance requirements. The combination `auth.enabled=false` + `anonymous.*.enabled=true` is a configuration error — `helm` render fails with `fail`.

> **Log-stream note:** Browsers cannot set headers on `new WebSocket()`, so
> the token is passed to the `/logs` endpoint as a URL parameter
> (`?token=…`). Make sure reverse proxies and ingress controllers do not
> persist the query string in their access logs.

---

## Container Image

The operator image (manager + UI in a single binary) is built and pushed to the
Forgejo registry automatically by Forgejo Actions on every release tag and
every push to `main`.

**Image:** `forgejo.thecloudroute.com/tom/rpc-operator`

**Tags:**

| Tag             | Meaning                                       |
|-----------------|-----------------------------------------------|
| `vX.Y.Z`        | Release build from a Git tag                  |
| `latest`        | Most recent release tag                       |
| `main`          | Latest commit on the `main` branch            |
| `main-<sha7>`   | Specific main commit (for bisect / pinning)   |

**Architectures:** `linux/amd64` (arm64 will follow once an arm64 Forgejo runner is available).

**Pull:**

```bash
docker pull forgejo.thecloudroute.com/tom/rpc-operator:latest
```

**Build locally** (maintainers, with a Docker daemon):

```bash
make docker-build IMG=forgejo.thecloudroute.com/tom/rpc-operator:dev
```

> **CI build:** Forgejo Actions builds the image with
> [Kaniko](https://github.com/GoogleContainerTools/kaniko) without a Docker
> daemon (`.forgejo/workflows/image.yml`). Multi-arch is not enabled because
> Kaniko is a single-arch builder and currently only an amd64 runner is
> available.

---

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/rpc-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/rpc-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/rpc-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/rpc-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
