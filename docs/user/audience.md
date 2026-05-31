# Who should read what?

> **Audience:** Both
> **Prerequisites:** [What is the RPC Operator?](index.md)

This documentation has two main audiences. Most readers fall clearly into one; the tables below point you to the sections most relevant to your role.

## Platform Administrator / Cluster Admin

You install and operate the RPC Operator for other teams. You control the Kubernetes cluster, manage Helm releases, and handle RBAC.

| Topic | Page |
|---|---|
| First-time installation | [Install via Helm](getting-started/install.md) |
| Choosing an auth model | [Authentication modes](operating/auth.md) |
| Setting up OIDC SSO | [OIDC SSO](operating/oidc.md) |
| Restricting namespaces | [Namespace allowlist](operating/namespace-allowlist.md) |
| Private image registries | [Pull secrets](operating/pull-secrets.md) |
| Connecting Prometheus | [Prometheus integration](operating/prometheus.md) |
| Upgrading or uninstalling | [Upgrades and uninstall](operating/upgrade-uninstall.md) |
| All Helm knobs | [Helm values reference](operating/helm-values.md) |

## Data Engineer / Pipeline Author

You write pipelines and deploy them to a cluster that's already running the operator. You work with `kubectl` and YAML.

| Topic | Page |
|---|---|
| Your first pipeline | [Deploy your first Pipeline](getting-started/first-pipeline.md) |
| Pipeline YAML structure | [Pipeline anatomy](authoring/anatomy.md) |
| Referencing Kubernetes Secrets | [Secrets via secretKeyRef](authoring/secrets.md) |
| Deploying and updating | [Deploying and redeploying](authoring/deploy.md) |
| Pausing a pipeline | [Stop and re-run](authoring/stop-rerun.md) |
| Shared cluster infrastructure | [When to use a PipelineCluster](clusters/when-to-use.md) |
| Reading pipeline logs | [Reading logs](operations/logs.md) |
| Viewing metrics | [Metrics and PodMonitor](operations/metrics.md) |
| Diagnosing problems | [Troubleshooting](operations/troubleshooting.md) |

## Both audiences

The [Reference](reference/pipeline-crd.md) section is for everyone. It documents every field on the `Pipeline` and `PipelineCluster` CRDs, the full Helm values list, and all operator CLI flags.
