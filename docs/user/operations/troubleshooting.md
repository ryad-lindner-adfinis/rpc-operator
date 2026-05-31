# Troubleshooting

> **Audience:** Both
> **Prerequisites:** none

## Pipeline stuck in `Pending`

The pod is waiting for scheduling. Common causes:

- **Image pull failure:** the Redpanda Connect image is unavailable or requires a pull secret
  ```bash
  kubectl -n <pipeline-namespace> describe pod <pipeline-pod> | grep -A 10 "Events:"
  # Look for: "Failed to pull image"
  ```
- **Resource constraints:** the pod cannot be scheduled due to CPU/memory limits
  ```bash
  kubectl -n <pipeline-namespace> describe pod <pipeline-pod> | grep "Insufficient"
  ```

## Pipeline in `Failed` phase

The pod exited with a non-zero code. Check the pod logs:

```bash
kubectl -n <pipeline-namespace> logs <pipeline-pod> --previous
```

Common causes:
- **Invalid YAML in `spec.rawYAML`:** Redpanda Connect rejects the config at startup. The log will contain `failed to init config`. See [Pipeline anatomy](../authoring/anatomy.md) for `rawYAML` syntax.
- **Missing environment variable:** a `${ENV_VAR}` reference in `rawYAML` has no corresponding `secretRef` entry. See [Secrets via secretKeyRef](../authoring/secrets.md).
- **Network errors:** the input or output cannot connect to its target (Kafka, HTTP, etc.).

## Pipeline exits immediately (Stopped, not Failed)

Finite inputs (like `generate` with `count > 0`) run to completion and exit 0. The operator sets `status.phase` to `Stopped`. This is expected — the pipeline finished successfully.

## Pod keeps restarting

The operator uses `restartPolicy: OnFailure`. If your pipeline exits non-zero repeatedly, the pod's restart count climbs and it enters `CrashLoopBackOff`. Fix the underlying error (invalid config, bad credentials, unreachable target) and the pod will recover automatically.

## `PipelineCluster` in `Degraded` phase

One or more instances are not Ready:

```bash
kubectl -n <pipeline-namespace> get pipelineclusters.rpc.operator.io
kubectl -n <pipeline-namespace> describe pipelineclusters.rpc.operator.io etl-small
kubectl -n <pipeline-namespace> get pods -l rpc.operator.io/cluster=etl-small
```

Check logs on the degraded instance pod.

## Operator not reconciling

If pipelines are created/updated but nothing happens:

```bash
kubectl -n rpc-operator-system logs deployment/rpc-operator --tail=50
```

Look for reconciliation errors. Common cause: the operator's ServiceAccount lost RBAC permissions.

## `make docs-check-reference` fails with "missing in docs"

A CRD field was added to Go code but not documented in the reference. Add a `### fieldName (type, required/optional)` entry under the `## Spec` or `## Status` section of the appropriate reference page. See [Pipeline CRD](../reference/pipeline-crd.md).

## Getting help

- Check [github.com/insidegreen/rpc-operator/issues](https://github.com/insidegreen/rpc-operator/issues) for known issues
- File a bug at the same location
