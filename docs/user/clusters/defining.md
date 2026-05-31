# Defining a PipelineCluster

> **Audience:** Platform admins
> **Prerequisites:** [When to use a PipelineCluster](when-to-use.md)

A `PipelineCluster` is a Kubernetes resource that represents a pool of Redpanda Connect instances running in streams mode.

## Minimal example

```yaml
apiVersion: rpc.operator.io/v1alpha1
kind: PipelineCluster
metadata:
  name: etl-small
  namespace: rpc-operator-poc
spec:
  replicas: 2
```

Apply it:

```bash
kubectl apply -f pipelinecluster.yaml
```

Watch it become Ready:

```bash
kubectl -n rpc-operator-poc get pipelineclusters.rpc.operator.io -w
# NAME        DESIRED   READY   PHASE     AGE
# etl-small   2         2       Ready     30s
```

## `spec.replicas`

Number of Redpanda Connect instances in the cluster. Each instance is a StatefulSet pod (e.g. `etl-small-0`, `etl-small-1`). Default: 1.

Increase replicas to distribute streams across more instances:

```yaml
spec:
  replicas: 4
```

!!! note
    The operator uses a simple round-robin assignment when a new stream is created. There is no automatic rebalancing when instances are added or removed — streams stay on their assigned instance until manually migrated. See [Migrating between clusters](migrating.md).

## `spec.image`

Override the Redpanda Connect image. Default: `docker.redpanda.com/redpandadata/connect:4`.

```yaml
spec:
  image: docker.redpanda.com/redpandadata/connect:4.36.0
```

## `spec.jsonLogging`

Force structured JSON logs on all instances. Default: `true`. Required for per-stream log filtering in the operator API. Leave this at its default unless you have a specific reason to disable it.

## `spec.resources`

Set CPU/memory requests and limits per instance:

```yaml
spec:
  replicas: 2
  resources:
    requests:
      cpu: "500m"
      memory: "512Mi"
    limits:
      cpu: "2"
      memory: "2Gi"
```

## Status

| Field | Description |
|---|---|
| `status.phase` | `Pending`, `Ready`, or `Degraded` |
| `status.readyReplicas` | Number of instances currently Ready |

A cluster is `Degraded` when one or more instances are not Ready (pod crashlooping, pending, etc.).

## Next steps

- [Running pipelines on a cluster](cluster-ref.md) — deploy a pipeline as a stream on this cluster
