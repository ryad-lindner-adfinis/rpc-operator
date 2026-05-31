# Helm Values Reference

> **Audience:** Platform admins
> **Prerequisites:** none

Complete reference for all values in `charts/rpc-operator/values.yaml`. For context and usage examples, see [Helm values reference](../operating/helm-values.md).

## Full values table

| Value | Type | Default | Description |
|---|---|---|---|
| `image.repository` | string | `ghcr.io/insidegreen/rpc-operator` | Operator image repository |
| `image.tag` | string | `""` | Image tag; empty = Chart.appVersion |
| `image.pullPolicy` | string | `IfNotPresent` | Pull policy |
| `imagePullSecrets` | list | `[]` | Pull secret names |
| `replicaCount` | integer | `1` | Operator pod replica count |
| `operator.prometheusUrl` | string | `""` | Prometheus base URL for graph |
| `operator.watchNamespaces` | list | `[]` | Namespace allowlist (empty = cluster-wide) |
| `auth.enabled` | boolean | `true` | Enable Bearer-token authentication |
| `anonymous.read.enabled` | boolean | `false` | Allow unauthenticated GETs |
| `anonymous.logs.enabled` | boolean | `false` | Allow unauthenticated log WebSocket |
| `features.visualEditor.enabled` | boolean | `false` | Enable visual pipeline editor (experimental) |
| `features.projects.nats.image` | string | `""` | NATS image override for PipelineProject |
| `features.projects.nats.tag` | string | `""` | NATS image tag override |
| `oidc.enabled` | boolean | `false` | Enable OIDC PKCE login |
| `oidc.issuer` | string | `""` | OIDC issuer URL |
| `oidc.clientID` | string | `""` | OIDC client ID |
| `oidc.scopes` | string | `openid,email,offline_access` | Requested scopes |
| `oidc.redirectURL` | string | `""` | OAuth 2.0 redirect URI |
| `oidc.uiRedirectURL` | string | `""` | Post-callback browser redirect |
| `leaderElection.enabled` | boolean | `false` | Enable leader election |
| `metrics.enabled` | boolean | `false` | Enable operator self-metrics |
| `service.type` | string | `ClusterIP` | Kubernetes service type |
| `service.port` | integer | `8082` | Service port |
| `ingress.enabled` | boolean | `false` | Create Ingress |
| `ingress.className` | string | `""` | IngressClass name |
| `ingress.host` | string | `rpc-operator.example.com` | Ingress hostname |
| `ingress.annotations` | object | `{}` | Ingress annotations |
| `ingress.tls` | list | `[]` | TLS configuration |
| `resources.limits.cpu` | string | `500m` | CPU limit for the operator pod |
| `resources.limits.memory` | string | `256Mi` | Memory limit for the operator pod |
| `resources.requests.cpu` | string | `50m` | CPU request for the operator pod |
| `resources.requests.memory` | string | `64Mi` | Memory request for the operator pod |
| `nodeSelector` | object | `kubernetes.io/arch: arm64` | Node selector labels. Remove or update to match your cluster architecture. |
| `tolerations` | list | `[]` | Pod tolerations |
| `affinity` | object | `{}` | Pod affinity rules |
| `podAnnotations` | object | `{}` | Operator pod annotations |
| `podLabels` | object | `{}` | Operator pod extra labels |
| `serviceAccount.create` | boolean | `true` | Create ServiceAccount |
| `serviceAccount.name` | string | `""` | ServiceAccount name (generated if empty) |
| `podSecurityContext.runAsNonRoot` | boolean | `true` | Enforce non-root user |
| `podSecurityContext.seccompProfile.type` | string | `RuntimeDefault` | Seccomp profile |
| `containerSecurityContext.readOnlyRootFilesystem` | boolean | `true` | Read-only root filesystem |
| `containerSecurityContext.allowPrivilegeEscalation` | boolean | `false` | Block privilege escalation |
| `containerSecurityContext.capabilities.drop` | list | `[ALL]` | Linux capabilities to drop |
| `examples.enabled` | boolean | `false` | Install sample pipelines for smoke tests |
