# RPC Operator

Kubernetes Operator für [Redpanda Connect](https://docs.redpanda.com/redpanda-connect/) Pipelines.
Data Engineers konfigurieren Pipelines visuell oder als YAML über eine eingebettete Web-UI
und deployen sie per Klick in einen Kubernetes-Cluster.

## Local Development

### Voraussetzungen

| Tool | Version |
|---|---|
| Go | ≥ 1.24 |
| Node.js | ≥ 20 (für UI-Build) |
| kubectl | ≥ 1.11 |
| Kubernetes-Cluster | ≥ 1.11 (kubeconfig konfiguriert) |

### Schnellstart

```bash
# 1. CRDs im Cluster installieren (einmalig)
make install

# 2. UI bauen
make ui-build

# 3. Operator starten — nutzt den aktuellen kubeconfig-Context
go run ./cmd/main.go
```

Der Operator verbindet sich mit dem aktiven kubeconfig-Context.
Die Web-UI ist danach unter **http://localhost:8082** erreichbar.

```bash
# Context prüfen / wechseln
kubectl config current-context
kubectl config use-context <context-name>
```

### CLI-Flags des Operators

Der Operator wird mit `go run ./cmd/main.go [flags]` gestartet. Alle Flags
haben sinnvolle Defaults — die folgenden sind im Dev-Setup am häufigsten relevant.

| Flag | Default | Zweck |
|---|---|---|
| `--api-bind-address` | `:8082` | Adresse für REST-API + eingebettete UI. Leer (`""`) deaktiviert den API-Server (nur Operator-Loop). |
| `--health-probe-bind-address` | `:8081` | Liveness/Readiness-Probes (`/healthz`, `/readyz`). |
| `--auth-enabled` | `true` | F43-Master-Switch. `--auth-enabled=false` reproduziert v0.7-Verhalten (kein Login, alle Requests via Operator-SA). Für Hot-Reload-Dev typischerweise `false` setzen. |
| `--prometheus-url` | _leer_ | Prometheus-Basis-URL für den Throughput-Graph (F15). Leer deaktiviert nur den Graph, alles andere läuft. Bsp.: `--prometheus-url=http://prometheus-operated.cattle-monitoring-system.svc:9090` |
| `--watch-namespaces` | _leer_ | F21-Allowlist als Komma-Liste. Leer = cluster-wide (sieht alle Pipelines). Bsp.: `--watch-namespaces=rpc-operator-poc,default` |
| `--leader-elect` | `false` | Für Multi-Replica-Setups. Im Single-Pod-Dev aus lassen. |
| `--metrics-bind-address` | `0` | Operator-Self-Metrics (controller-runtime). `0` = aus (Default; PodMonitor-pro-Pipeline aus F36 ist davon unberührt). `:8443` aktiviert HTTPS mit authn/authz. |
| `--zap-log-level` | `debug` | Log-Level (`info`, `debug`, `error`); `opts.Development=true` im Code setzt den Default auf `debug`. |
| `--zap-encoder` | `console` | `console` (menschenlesbar) oder `json` (für strukturierte Logs). |

Dev-typischer Start mit allen relevanten Flags:

```bash
go run ./cmd/main.go \
  --auth-enabled=false \
  --watch-namespaces=rpc-operator-poc \
  --prometheus-url=http://prometheus-operated.cattle-monitoring-system.svc:9090
```

> **Produktions-Flags:** `--metrics-secure`, `--metrics-cert-*`, `--webhook-cert-*`, `--enable-http2` sind für Helm-Deployments im Cluster gedacht und im Dev-Setup normalerweise nicht nötig. Vollständige Liste: `go run ./cmd/main.go --help`.

### UI-Entwicklung mit Hot-Reload

Für Frontend-Änderungen reicht ein separater Vite-Dev-Server — kein Neustart
des Operators nötig. Vite proxied `/api`-Requests automatisch an `:8082`.

```bash
# Terminal 1 — Operator
go run ./cmd/main.go

# Terminal 2 — Vite Dev Server (Hot-Reload)
make ui-dev
# → http://localhost:5173
```

### Tests ausführen

```bash
# Alle Go-Tests
make test

# Nur ein Paket
go test ./internal/render/...

# Linter
make lint

# TypeScript Type-Check
cd ui && npx tsc --noEmit
```

### Pipeline manuell testen

```bash
# Deployte Pipelines anzeigen
kubectl get pipeline.rpc.operator.io -n rpc-operator-poc

# Pods der Pipelines
kubectl get pods -n rpc-operator-poc

# Logs einer Pipeline
kubectl logs -n rpc-operator-poc <pod-name>

# Pipeline löschen
kubectl delete pipeline.rpc.operator.io <name> -n rpc-operator-poc
```

### Nützliche Make-Targets

```bash
make help          # Alle Targets mit Beschreibung
make ui-build      # React-UI bauen (→ internal/api/static/)
make ui-dev        # Vite Dev Server starten
make build         # Go-Binary inkl. UI bauen (→ bin/manager)
make test          # Go-Tests ausführen
make lint          # golangci-lint
make install       # CRDs im Cluster installieren
make uninstall     # CRDs aus Cluster entfernen
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

Pull-Secret für die private Registry, Konfigurationsoptionen und 5-Minuten-
Quickstart: [`charts/rpc-operator/README.md`](charts/rpc-operator/README.md).

### Auth-Modi

Ab v0.8 ist die UI per Default mit Bearer-Token-Auth geschützt. Drei Modi
gesteuert über Helm-Werte:

| Modus | Helm | Verhalten |
|---|---|---|
| **A. Auth aus** (v0.7-Kompatibilität) | `--set auth.enabled=false` | Kein Login; alle Requests laufen unter Operator-SA. **Niemals** mit Public-Ingress kombinieren. |
| **B. Token-Auth** (Default v0.8) | `auth.enabled=true` (Default) | Login mit K8s-Bearer-Token (paste oder Kubeconfig-Upload — nur Token wird extrahiert, Cert-Auth wird abgelehnt). Backend forwarded den Token per-Request an den Apiserver; native K8s-RBAC entscheidet. |
| **C. + anonyme GETs** (F42) | `auth.enabled=true` + `anonymous.read.enabled=true` (+ optional `anonymous.logs.enabled=true`) | Wie B; zusätzlich GETs auf Pipelines/Catalog/Namespaces/Metrics ohne Token (laufen unter Operator-SA innerhalb der F21-Allowlist). Logs nur, wenn der separate `anonymous.logs.enabled`-Schalter gesetzt ist — Log-Inhalte können Payloads/Secrets enthalten. UI zeigt einen Read-Only-Banner und blendet Edit/Deploy/Delete-Buttons aus. |

Token besorgen für Mode B:

```bash
kubectl --context <ctx> create token <serviceaccount> -n <namespace>
```

Mode C aktivieren (Statusboard-/Public-Display-Use-Case):

```bash
# Reads anonym, Logs vertraulich:
helm install rpc-operator ./charts/rpc-operator \
  --set auth.enabled=true \
  --set anonymous.read.enabled=true

# Plus anonyme Live-Logs:
helm install rpc-operator ./charts/rpc-operator \
  --set auth.enabled=true \
  --set anonymous.read.enabled=true \
  --set anonymous.logs.enabled=true
```

> **Sicherheits-Warnung (F42):** Anonyme Reads zeigen Pipeline-Specs inkl. `spec.secretRefs`-Namen (nicht -Werten) und `spec.rawYAML`. Pipelines mit sensiblen Bloblang-Mappings können dabei Metadaten exponieren. F42 ist für Demo-/Statusboard-Cluster gedacht, nicht für Produktionspipelines mit Compliance-Anforderungen. Die Kombination `auth.enabled=false` + `anonymous.*.enabled=true` ist ein Konfigurationsfehler — Helm-Render schlägt mit `fail` ab.

> **Log-Stream-Hinweis:** Browser können bei `new WebSocket()` keine Header
> setzen, daher wandert das Token zum `/logs`-Endpoint in der URL
> (`?token=…`). Bei Reverse-Proxies und Ingress-Controllern darauf achten,
> dass Access-Logs den Query-String nicht persistent loggen.

---

## Container Image

Das Operator-Image (Manager + UI in einem Binary) wird bei jedem Release-Tag und bei jedem
Main-Push automatisch über Forgejo Actions gebaut und in die Forgejo-Registry gepusht.

**Image:** `forgejo.thecloudroute.com/tom/rpc-operator`

**Tags:**

| Tag             | Bedeutung                                     |
|-----------------|-----------------------------------------------|
| `vX.Y.Z`        | Release-Build von einem Git-Tag               |
| `latest`        | Aktuellster Release-Tag                       |
| `main`          | Letzter Commit auf `main`-Branch              |
| `main-<sha7>`   | Spezifischer Main-Commit (für Bisect / Pin)   |

**Architekturen:** `linux/amd64` (arm64 folgt, sobald ein arm64-Forgejo-Runner verfügbar ist).

**Pull:**

```bash
docker pull forgejo.thecloudroute.com/tom/rpc-operator:latest
```

**Lokal bauen** (Maintainer, mit Docker-Daemon):

```bash
make docker-build IMG=forgejo.thecloudroute.com/tom/rpc-operator:dev
```

> **CI-Build:** Forgejo Actions baut das Image mit
> [Kaniko](https://github.com/GoogleContainerTools/kaniko) ohne Docker-Daemon
> (`.forgejo/workflows/image.yml`). Multi-Arch ist nicht aktiviert, weil Kaniko
> einen Single-Arch-Builder ist und derzeit nur ein amd64-Runner verfügbar ist.

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

