# Running pipelines on a cluster

> **Audience:** Pipeline authors
> **Prerequisites:** [Defining a PipelineCluster](defining.md)

Once a `PipelineCluster` is Ready, you can run pipelines as streams on it by setting `spec.clusterRef` to the cluster's name.

## Deploy a stream pipeline

```yaml
apiVersion: rpc.operator.io/v1alpha1
kind: Pipeline
metadata:
  name: kafka-to-s3
  namespace: rpc-operator-poc
spec:
  clusterRef: etl-small
  rawYAML: |
    input:
      kafka_franz:
        seed_brokers: ["kafka:9092"]
        topics: ["uploads"]
        consumer_group: s3-sink
    output:
      aws_s3:
        bucket: my-bucket
        path: "${!metadata("kafka_topic")}/${!timestamp_unix()}.json"
```

The operator assigns the stream to one of the cluster's instances. Check the assignment:

```bash
kubectl -n rpc-operator-poc get pipelines.rpc.operator.io kafka-to-s3 -o yaml | grep -A 5 status
# status:
#   phase: Running
#   assignedCluster: etl-small
#   assignedInstance: etl-small-1
#   streamID: kafka-to-s3
```

## Logs and metrics in stream mode

Logs and metrics work the same as in pod mode but are scoped to the specific stream:

- **Logs:** `kubectl -n rpc-operator-poc logs etl-small-1` shows all streams on that instance. The operator API filters by `streamID` when you fetch logs for a specific pipeline.
- **Metrics:** the operator builds `rate(metric{pod="etl-small-1", stream="kafka-to-s3"}[1m])` PromQL queries automatically.

See [Reading logs](../operations/logs.md) and [Metrics and PodMonitor](../operations/metrics.md).

## Constraints

- `spec.clusterRef` and `spec.stopped` can be combined: setting `spec.stopped: true` on a stream pipeline removes the stream from the cluster without deleting the Pipeline CR.
- The `PipelineCluster` must be in the same namespace as the `Pipeline`.
- `spec.replicas` does not apply to stream pipelines; the number of instances is controlled by `PipelineCluster.spec.replicas`.

For the complete `spec.clusterRef` field reference, see [Pipeline CRD — clusterRef](../reference/pipeline-crd.md#clusterref-string-optional).
