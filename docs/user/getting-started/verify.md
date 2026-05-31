# Verify and next steps

> **Audience:** Both
> **Prerequisites:** [Deploy your first Pipeline](first-pipeline.md)

Use this checklist to confirm that your installation is production-ready before you hand it off to pipeline authors.

## Platform admin checklist

- [ ] **Operator pod is Running and stays Running**

    ```bash
    kubectl -n rpc-operator-system get pods
    # rpc-operator-<hash>   1/1   Running   0   ...
    ```

- [ ] **CRDs are installed**

    ```bash
    kubectl get crd pipelines.rpc.operator.io pipelineclusters.rpc.operator.io
    ```

- [ ] **Authentication mode is configured** — see [Authentication modes](../operating/auth.md)

- [ ] **Namespace allowlist is set** (if applicable) — see [Namespace allowlist](../operating/namespace-allowlist.md)

- [ ] **Prometheus integration is working** (if applicable) — see [Prometheus integration](../operating/prometheus.md)

## Pipeline author checklist

- [ ] **You can apply a pipeline manifest** — try the [hello-pipeline](first-pipeline.md) example
- [ ] **You have RBAC in your namespace** — you can `kubectl get pipelines.rpc.operator.io` without a 403
- [ ] **You can read logs** — see [Reading logs](../operations/logs.md)

## What to do next

=== "Platform admins"
    - [Helm values reference](../operating/helm-values.md) — full list of configuration options
    - [Authentication modes](../operating/auth.md) — harden auth for your environment
    - [OIDC SSO](../operating/oidc.md) — add SSO for multi-user clusters

=== "Pipeline authors"
    - [Pipeline anatomy](../authoring/anatomy.md) — write real pipelines
    - [Secrets via secretKeyRef](../authoring/secrets.md) — inject credentials securely
    - [PipelineClusters & Streams](../clusters/when-to-use.md) — scale up with stream mode
