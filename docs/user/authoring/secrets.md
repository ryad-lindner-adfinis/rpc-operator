# Secrets via secretKeyRef

> **Audience:** Pipeline authors
> **Prerequisites:** [Pipeline anatomy](anatomy.md)

Sensitive values (passwords, API keys, connection strings) should never be written directly into `spec.rawYAML`. Use `spec.secretRefs` to inject Kubernetes Secret keys as environment variables into the pipeline pod, then reference them in your RPC YAML with `${ENV_VAR}`.

## How it works

1. You create a Kubernetes `Secret` with the sensitive value
2. You add a `secretRef` entry to your `Pipeline` spec that maps the secret key to an environment variable name
3. The operator injects that environment variable into the pipeline pod
4. Your RPC YAML references the variable with `${ENV_VAR_NAME}`

## Example

Create the secret:

```bash
kubectl -n rpc-operator-poc create secret generic kafka-creds \
  --from-literal=password=super-secret-password
```

Reference it in the Pipeline:

```yaml
apiVersion: rpc.operator.io/v1alpha1
kind: Pipeline
metadata:
  name: kafka-secure
  namespace: rpc-operator-poc
spec:
  secretRefs:
    - envVar: KAFKA_PASS
      secretName: kafka-creds
      key: password
  rawYAML: |
    input:
      kafka_franz:
        seed_brokers: ["kafka:9092"]
        topics: ["events"]
        sasl:
          - mechanism: SCRAM-SHA-512
            username: my-user
            password: ${KAFKA_PASS}
    output:
      stdout: {}
```

## `secretRefs` fields

Each entry in `spec.secretRefs` has three required fields:

| Field | Description | Constraints |
|---|---|---|
| `envVar` | Environment variable name in the pod | Must match `[A-Za-z_][A-Za-z0-9_]*` |
| `secretName` | Name of the Kubernetes Secret | Must be in the same namespace as the Pipeline |
| `key` | Key within the Secret's `data` map | — |

## Multiple secrets

You can reference multiple secrets and keys in a single pipeline:

```yaml
spec:
  secretRefs:
    - envVar: KAFKA_PASS
      secretName: kafka-creds
      key: password
    - envVar: OUTPUT_API_KEY
      secretName: output-service-creds
      key: api-key
  rawYAML: |
    input:
      kafka_franz:
        password: ${KAFKA_PASS}
        ...
    output:
      http_client:
        headers:
          Authorization: "Bearer ${OUTPUT_API_KEY}"
        ...
```

!!! warning
    If the referenced Secret does not exist in the namespace, the pipeline pod will fail to start with a `CreateContainerConfigError`. Check `kubectl -n <ns> describe pod <pipeline-pod>` for details.

## Next steps

- [Deploying and redeploying](deploy.md) — apply the manifest and update pipelines
