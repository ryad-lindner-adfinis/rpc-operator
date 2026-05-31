# Pipeline anatomy

> **Audience:** Pipeline authors
> **Prerequisites:** [Deploy your first Pipeline](../getting-started/first-pipeline.md)

A `Pipeline` Custom Resource wraps a Redpanda Connect configuration. This page explains the structure of a Pipeline manifest and describes every field you interact with day-to-day.

## Minimal example

```yaml
apiVersion: rpc.operator.io/v1alpha1
kind: Pipeline
metadata:
  name: kafka-to-stdout
  namespace: rpc-operator-poc
spec:
  rawYAML: |
    input:
      kafka_franz:
        seed_brokers: ["kafka:9092"]
        topics: ["events"]
        consumer_group: my-consumer
    pipeline:
      processors:
        - mapping: |
            root = this
    output:
      stdout: {}
```

## `spec.rawYAML`

The `spec.rawYAML` field holds a complete Redpanda Connect configuration as a multi-line YAML string. Use the `|` block scalar for multi-line values.

The operator injects an HTTP server block automatically if one is not present in your YAML — this is required for the Prometheus metrics endpoint. You do not need to add it yourself.

See the [Redpanda Connect documentation](https://docs.redpanda.com/redpanda-connect/configuration/about/) for all supported input, processor, and output components.

!!! note
    Structured fields (`spec.input`, `spec.processors`, `spec.output`) exist in the CRD but are used only by the visual editor (not yet GA). Use `spec.rawYAML` for all hand-authored pipelines.

## `spec.image`

Override the Redpanda Connect image per pipeline. Useful for pinning versions or using a mirrored image:

```yaml
spec:
  image: docker.redpanda.com/redpandadata/connect:4.36.0
  rawYAML: |
    ...
```

Default: `docker.redpanda.com/redpandadata/connect:4`

## `spec.replicas`

Currently fixed at `1`. Multi-replica support is planned for a future release. Omit this field or set it to `1`.

## `spec.secretRefs`

Inject Kubernetes Secret keys as environment variables into the pipeline pod. See [Secrets via secretKeyRef](secrets.md).

## `spec.stopped`

Set to `true` to pause the pipeline without deleting the CR. The operator deletes the pod and keeps `status.phase` as `Stopped`. Set back to `false` to resume. See [Stop and re-run](stop-rerun.md).

## `spec.clusterRef`

Name of a `PipelineCluster` in the same namespace. When set, the pipeline runs as a stream on the cluster instead of in its own pod. See [Running pipelines on a cluster](../clusters/cluster-ref.md).

## Status fields

After applying, the operator populates `status`:

| Field | Description |
|---|---|
| `status.phase` | `Pending`, `Running`, `Failed`, or `Stopped` |
| `status.podName` | Name of the pipeline pod (pod mode only) |
| `status.assignedCluster` | PipelineCluster name (stream mode only) |
| `status.assignedInstance` | Cluster pod hosting the stream (stream mode only) |
| `status.streamID` | Deployed stream ID, equal to the pipeline name (stream mode only) |

```bash
kubectl -n rpc-operator-poc get pipelines.rpc.operator.io kafka-to-stdout -o yaml
```

For the complete field reference including all constraints and status values, see [Pipeline CRD](../reference/pipeline-crd.md).
