# Prometheus integration

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

The RPC Operator automatically creates a `PodMonitor` resource for every pipeline pod. Prometheus scrapes throughput and error-rate metrics from each pod's `/metrics` endpoint.

## Prerequisites

- Prometheus Operator installed (or kube-prometheus-stack)
- The `PodMonitor` CRD available in the cluster (`kubectl get crd podmonitors.monitoring.coreos.com`)
- Prometheus configured to scrape `PodMonitor` resources in pipeline namespaces

## Connect the operator to Prometheus

Set `operator.prometheusUrl` to your Prometheus base URL. This enables the throughput graph in the embedded UI:

```bash
helm upgrade rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system \
  --set operator.prometheusUrl=http://prometheus-operated.cattle-monitoring-system.svc:9090
```

!!! note
    `prometheusUrl` is used only by the operator's API to proxy metric queries to the UI. The `PodMonitor` is created automatically regardless of this setting — Prometheus scrapes pipeline pods even without this value.

## What is scraped

Each pipeline pod exposes Redpanda Connect's built-in Prometheus metrics on port 4195 at `/metrics`. The metrics include:

| Metric pattern | Description |
|---|---|
| `redpanda_connect_input_received_total` | Messages received by the input |
| `redpanda_connect_output_sent_total` | Messages successfully sent by the output |
| `redpanda_connect_output_error_total` | Output errors |
| `redpanda_connect_processor_error_total` | Processor errors |

The operator API surfaces two derived metrics via its `/metrics` endpoint:
- `throughput` — `rate(input_received_total[1m])`
- `error_rate` — `rate(output_error_total[1m])`

## PodMonitor per pipeline

The operator creates one `PodMonitor` per `Pipeline` CR in the same namespace, named `<pipeline-name>-monitor`. It targets the pipeline pod by its `rpc.operator.io/pipeline` label and scrapes port 4195.

You can inspect a PodMonitor:

```bash
kubectl -n <pipeline-namespace> get podmonitors
kubectl -n <pipeline-namespace> describe podmonitor <pipeline-name>-monitor
```

## Verify metrics are flowing

```bash
# Port-forward to the pipeline pod's metrics endpoint:
kubectl -n <pipeline-namespace> port-forward pod/<pipeline-pod> 4195:4195
curl http://localhost:4195/metrics | grep redpanda_connect_input
```
