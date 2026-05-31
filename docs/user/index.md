# What is the RPC Operator?

> **Audience:** Both
> **Prerequisites:** none

The RPC Operator is a Kubernetes operator for running [Redpanda Connect](https://docs.redpanda.com/redpanda-connect/) data pipelines as native Kubernetes resources. You define a pipeline in YAML, apply it with `kubectl`, and the operator takes care of running, monitoring, and restarting the pipeline pod for you.

## What it provides

- **CRD-based pipelines** — each `Pipeline` CR maps to one dedicated pod running a Redpanda Connect process
- **PipelineCluster mode** — share a pool of long-running Redpanda Connect instances across many lightweight streams
- **Namespace-scoped access control** — limit the operator to specific namespaces via the allowlist
- **Bearer-token and OIDC authentication** — protect the embedded API with Kubernetes-native tokens or an external identity provider
- **Prometheus metrics** — per-pipeline throughput and error-rate metrics via a `PodMonitor`

## Primary use cases

| Use case | What you use |
|---|---|
| One-off or low-volume pipeline | `Pipeline` CR with `spec.rawYAML` |
| Many short-lived streams sharing infrastructure | `PipelineCluster` + `spec.clusterRef` |
| Platform team deploying for multiple data teams | Namespace allowlist + RBAC per team |

## Not in scope (v1)

- The visual pipeline editor (UI) — covered separately once it reaches GA
- `PipelineProject` grouping — in design, not yet shipped
- Structured `spec.input` / `spec.processors` / `spec.output` fields — use `spec.rawYAML` instead

## Where to go next

- [Who should read what?](audience.md) — pick the right documentation track for your role
- [Prerequisites](getting-started/prerequisites.md) → [Install via Helm](getting-started/install.md) — if you want to get started immediately
