# Reading logs

> **Audience:** Both
> **Prerequisites:** [Deploy your first Pipeline](../getting-started/first-pipeline.md)

## Pod mode — kubectl logs

In pod mode, each pipeline runs in a dedicated pod. Read logs directly:

```bash
# One-shot (last 100 lines)
kubectl -n <pipeline-namespace> logs <pipeline-pod> --tail=100

# Follow (stream)
kubectl -n <pipeline-namespace> logs <pipeline-pod> -f

# Find the pod name from the Pipeline status
kubectl -n <pipeline-namespace> get pipelines.rpc.operator.io my-pipeline -o jsonpath='{.status.podName}'
```

## Pod mode — operator API

The operator exposes a WebSocket endpoint that streams structured log lines filtered to a specific pipeline:

```bash
# With a Bearer token (Mode B):
kubectl -n rpc-operator-system port-forward svc/rpc-operator 8082:8082 &
TOK=$(kubectl -n <pipeline-namespace> create token alice --duration=1h)
websocat "ws://localhost:8082/api/v1/namespaces/<pipeline-namespace>/pipelines/my-pipeline/logs?token=$TOK"
```

!!! note
    Browsers cannot set the `Authorization` header on a WebSocket upgrade. The operator therefore accepts the token as a `?token=` query parameter on this endpoint. Ensure your ingress does not log query strings.

## Stream mode — per-stream log filtering

When a pipeline runs as a stream on a `PipelineCluster`, its logs are mixed with those of other streams on the same instance pod. The operator API filters log lines to only those belonging to the specific stream (by matching the `stream` JSON field in the log output).

```bash
# Same WebSocket endpoint works in stream mode:
websocat "ws://localhost:8082/api/v1/namespaces/<pipeline-namespace>/pipelines/kafka-to-s3/logs?token=$TOK"
```

!!! note
    The `spec.jsonLogging: true` flag on the `PipelineCluster` is required for per-stream filtering. With JSON logging off, the operator cannot distinguish which log line belongs to which stream.

## Log levels

Redpanda Connect uses structured JSON logs. The log level is not directly configurable via the Pipeline CR; it is set in the `spec.rawYAML`:

```yaml
rawYAML: |
  logger:
    level: DEBUG
  input:
    ...
```

Default level: `INFO`.
