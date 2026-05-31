# Upgrades and uninstall

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

## Upgrading the operator

Use `helm upgrade` with the new chart version. The operator is designed for in-place upgrades — running pipelines are not interrupted during an operator upgrade because the reconciliation loop simply re-adopts existing pods.

```bash
helm upgrade rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system \
  --set image.tag=<new-tag> \
  --reuse-values
```

!!! tip
    `--reuse-values` carries forward all values you set at install time. Pass `--reset-values` instead if you want to start from chart defaults.

## Upgrading the CRDs

Helm does not upgrade CRDs automatically on `helm upgrade`. Apply them manually before upgrading:

```bash
kubectl apply -f charts/rpc-operator/crds/
```

Check that the CRDs were updated:

```bash
kubectl get crd pipelines.rpc.operator.io -o yaml | grep -E "generation:|resourceVersion:"
```

## Verify the upgrade

```bash
kubectl -n rpc-operator-system rollout status deployment/rpc-operator
kubectl -n rpc-operator-system get pods
```

Run a smoke test by applying the [hello-pipeline](../getting-started/first-pipeline.md) sample.

## Uninstall the operator

```bash
helm uninstall rpc-operator -n rpc-operator-system
```

!!! warning
    Uninstalling the Helm release does **not** remove the CRDs or any `Pipeline`/`PipelineCluster` CRs. If you want a clean removal:

```bash
# 1. Delete all Pipeline and PipelineCluster CRs (removes pods too)
kubectl delete pipelines.rpc.operator.io --all --all-namespaces
kubectl delete pipelineclusters.rpc.operator.io --all --all-namespaces

# 2. Uninstall the Helm release
helm uninstall rpc-operator -n rpc-operator-system

# 3. Remove CRDs
kubectl delete crd pipelines.rpc.operator.io pipelineclusters.rpc.operator.io

# 4. Remove the namespace
kubectl delete namespace rpc-operator-system
```
