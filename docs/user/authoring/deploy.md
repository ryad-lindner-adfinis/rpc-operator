# Deploying and redeploying

> **Audience:** Pipeline authors
> **Prerequisites:** [Pipeline anatomy](anatomy.md)

## Deploy a pipeline

Apply the manifest with `kubectl`:

```bash
kubectl apply -f my-pipeline.yaml
```

The operator creates a pod within a few seconds. Watch the status:

```bash
kubectl -n rpc-operator-poc get pipelines.rpc.operator.io -w
```

## Update a pipeline

To update `spec.rawYAML` or any other field, edit the manifest and re-apply:

```bash
kubectl apply -f my-pipeline.yaml
```

The operator detects the change and **replaces the pod** — there is a brief interruption while the old pod terminates and the new pod starts. Redpanda Connect's at-least-once delivery guarantees still apply: messages in-flight at the time of the pod restart may be reprocessed.

!!! warning
    Updating `spec.rawYAML` always causes a pod restart. Design pipelines to be idempotent at the output side if pod restarts could cause duplicate writes.

## Force a restart without changing YAML

The operator only restarts the pod when the spec content changes (rawYAML, image, or secretRefs). To force a restart without touching those fields, use the stop-and-resume cycle:

```bash
# Stop the pipeline (deletes the pod)
kubectl -n rpc-operator-poc patch pipelines.rpc.operator.io my-pipeline \
  --type=merge -p '{"spec":{"stopped":true}}'

# Wait for Stopped phase
kubectl -n rpc-operator-poc get pipelines.rpc.operator.io my-pipeline -o jsonpath='{.status.phase}'

# Resume (creates a fresh pod)
kubectl -n rpc-operator-poc patch pipelines.rpc.operator.io my-pipeline \
  --type=merge -p '{"spec":{"stopped":false}}'
```

See [Stop and re-run](stop-rerun.md) for details.

## Delete a pipeline

```bash
kubectl delete pipelines.rpc.operator.io my-pipeline -n rpc-operator-poc
# or
kubectl delete -f my-pipeline.yaml
```

Deleting the CR deletes the pod, ConfigMap, and PodMonitor.

!!! tip
    To stop the pipeline while keeping the CR (and its history), use `spec.stopped: true` instead. See [Stop and re-run](stop-rerun.md).

## GitOps / CI/CD

Because pipelines are Kubernetes resources, they work naturally with GitOps tooling (ArgoCD, Flux) or any CI pipeline that runs `kubectl apply`. Store your pipeline YAML in git and let your CD system apply changes on merge.
