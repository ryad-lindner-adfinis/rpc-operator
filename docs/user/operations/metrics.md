# Metrics and PodMonitor

> **Audience:** Both
> **Prerequisites:** [Prometheus integration](../operating/prometheus.md)

## Available metrics

Every pipeline pod (and PipelineCluster instance) exposes Redpanda Connect's built-in Prometheus metrics on port 4195. The operator API exposes four query names:

| Query name | PromQL (pod mode) | Description |
|---|---|---|
| `throughput` | `rate(output_sent{pod="<pod>"}[1m])` | Output messages/sec |
| `error_rate` | `rate(output_error{pod="<pod>"}[1m])` | Output errors/sec |
| `input_rate` | `rate(input_received{pod="<pod>"}[1m])` | Input messages/sec |
| `processor_error_rate` | `rate(processor_error{pod="<pod>"}[1m])` | Processor errors/sec |

In stream mode, the queries add a `stream="<pipeline-name>"` label selector automatically.

## Fetch metrics via the API

```bash
TOK=$(kubectl -n <pipeline-namespace> create token alice --duration=1h)
kubectl -n rpc-operator-system port-forward svc/rpc-operator 8082:8082 &

curl -H "Authorization: Bearer $TOK" \
  "http://localhost:8082/api/v1/namespaces/<pipeline-namespace>/pipelines/my-pipeline/metrics?query=throughput"
```

Returns a JSON object with the Prometheus range-query result (datapoints array).

## PodMonitor

The operator creates a `PodMonitor` for each pipeline, with the same name as the pipeline. Prometheus uses this to discover and scrape the pod's metrics endpoint.

```bash
kubectl -n <pipeline-namespace> get podmonitors
# NAME                AGE
# my-pipeline         5m
```

The PodMonitor is deleted automatically when the Pipeline CR is deleted.
