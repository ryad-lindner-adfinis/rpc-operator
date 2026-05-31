# Helm values reference

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

This page summarises all values in `charts/rpc-operator/values.yaml`. For the canonical list with inline comments, read the file directly.

## Image

| Value | Default | Description |
|---|---|---|
| `image.repository` | `forgejo.thecloudroute.com/tom/rpc-operator` | Container image repository |
| `image.tag` | `""` (= Chart.appVersion) | Image tag; `main` for nightly builds |
| `image.pullPolicy` | `IfNotPresent` | Kubernetes pull policy |
| `imagePullSecrets` | `[]` | List of pull secret names |
| `replicaCount` | `1` | Operator replica count |

## Operator configuration

| Value | Default | Description |
|---|---|---|
| `operator.prometheusUrl` | `""` | Prometheus base URL for the throughput graph. Empty disables only the graph. Example: `http://prometheus-operated.cattle-monitoring-system.svc:9090` |
| `operator.watchNamespaces` | `[]` | Namespace allowlist. Empty = cluster-wide. |

## Authentication

| Value | Default | Description |
|---|---|---|
| `auth.enabled` | `true` | Master auth switch. `false` = no login (v0.7 compat). Never combine with a public ingress. |
| `anonymous.read.enabled` | `false` | Allow unauthenticated GETs on pipelines/catalog/namespaces/metrics. Requires `auth.enabled=true`. |
| `anonymous.logs.enabled` | `false` | Also expose the live-log WebSocket without a token. Requires `anonymous.read.enabled=true`. |

## OIDC

| Value | Default | Description |
|---|---|---|
| `oidc.enabled` | `false` | Enable OIDC PKCE login button in the UI |
| `oidc.issuer` | `""` | OIDC issuer URL (e.g. `https://keycloak.example.com/realms/platform`) |
| `oidc.clientID` | `""` | OAuth 2.0 public client ID |
| `oidc.scopes` | `openid,email,offline_access` | Requested scopes |
| `oidc.redirectURL` | `""` | Callback URL registered at the IdP (must include `/api/v1/auth/callback`) |
| `oidc.uiRedirectURL` | `""` | Where the browser lands after the callback. Empty = `/` |

## Features

| Value | Default | Description |
|---|---|---|
| `features.visualEditor.enabled` | `false` | Enable the visual pipeline editor (experimental) |
| `features.projects.nats.image` | `""` | Override the NATS image for PipelineProject |
| `features.projects.nats.tag` | `""` | Override the NATS image tag; empty = use whatever tag is already in `image` |

## Service and networking

| Value | Default | Description |
|---|---|---|
| `service.type` | `ClusterIP` | Kubernetes service type |
| `service.port` | `8082` | Service port (API + embedded UI) |
| `ingress.enabled` | `false` | Create an Ingress for the UI |
| `ingress.className` | `""` | IngressClass |
| `ingress.host` | `rpc-operator.example.com` | Hostname for the Ingress |
| `ingress.annotations` | `{}` | Annotations on the Ingress resource |
| `ingress.tls` | `[]` | TLS configuration for the Ingress |

## Resilience

| Value | Default | Description |
|---|---|---|
| `leaderElection.enabled` | `false` | Enable leader election for multi-replica setups |
| `metrics.enabled` | `false` | Enable operator self-metrics on `:8443` HTTPS |

## Workload configuration

| Value | Default | Description |
|---|---|---|
| `resources.limits.cpu` | `500m` | CPU limit for the operator pod |
| `resources.limits.memory` | `256Mi` | Memory limit for the operator pod |
| `resources.requests.cpu` | `50m` | CPU request for the operator pod |
| `resources.requests.memory` | `64Mi` | Memory request for the operator pod |
| `nodeSelector` | `kubernetes.io/arch: arm64` | Pins to arm64 nodes (matches the dev cluster). Remove or update to match your cluster architecture. |
| `tolerations` | `[]` | Pod tolerations |
| `affinity` | `{}` | Pod affinity rules |
| `podAnnotations` | `{}` | Annotations on the operator pod |
| `podLabels` | `{}` | Extra labels on the operator pod |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.name` | `""` | ServiceAccount name; generated if empty |

## Security context

| Value | Default | Description |
|---|---|---|
| `podSecurityContext.runAsNonRoot` | `true` | Enforce non-root user for the pod |
| `podSecurityContext.seccompProfile.type` | `RuntimeDefault` | Seccomp profile |
| `containerSecurityContext.readOnlyRootFilesystem` | `true` | Read-only root filesystem |
| `containerSecurityContext.allowPrivilegeEscalation` | `false` | Block privilege escalation |

## Misc

| Value | Default | Description |
|---|---|---|
| `examples.enabled` | `false` | Install sample pipelines into the release namespace. Useful for first-run smoke tests. |
