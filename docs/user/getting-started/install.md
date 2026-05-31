# Install via Helm

> **Audience:** Platform admins
> **Prerequisites:** [Prerequisites](prerequisites.md)

This page walks you through installing the RPC Operator with Helm. The chart installs the operator deployment, the CRDs (`Pipeline` and `PipelineCluster`), RBAC, and a service on port 8082.

## Add the chart

The chart lives alongside the source code. Clone the repository or copy the `charts/rpc-operator/` directory to your Helm environment.

## Basic install

```bash
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace
```

This installs with the default values:

- **Auth:** enabled (Mode B — Bearer token)
- **Image:** pulled from GitHub Container Registry (requires a pull secret; see below)
- **Namespaces:** operator watches all namespaces
- **Prometheus:** not connected

## Image pull secret

The default image is hosted on GitHub Container Registry (`ghcr.io`). Create a pull secret before installing:

```bash
kubectl create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io \
  --docker-username=<your-github-username> \
  --docker-password=<your-PAT> \
  -n rpc-operator-system
```

Then include it in the install:

```bash
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace \
  --set 'imagePullSecrets[0].name=ghcr-pull' \
  --set image.tag=main
```

!!! tip
    `image.tag=main` tracks the latest nightly build. For production, pin to a specific tag.

## Verify the installation

```bash
kubectl -n rpc-operator-system get pods
# NAME                           READY   STATUS    RESTARTS   AGE
# rpc-operator-<hash>   1/1     Running   0          30s

kubectl get crd pipelines.rpc.operator.io
# NAME                        CREATED AT
# pipelines.rpc.operator.io   2026-...
```

Access the API:

```bash
kubectl -n rpc-operator-system port-forward svc/rpc-operator 8082:8082
# In another terminal:
curl http://localhost:8082/api/v1/namespaces
# {"namespaces":["default","rpc-operator-poc",...]}
```

!!! note
    With the default `auth.enabled=true`, the `namespaces` endpoint requires a Bearer token. See [Authentication modes](../operating/auth.md) for how to mint one, or set `--set auth.enabled=false` for a dev install.

## Common install variants

=== "Dev (auth off)"
    ```bash
    helm install rpc-operator ./charts/rpc-operator \
      -n rpc-operator-system --create-namespace \
      --set auth.enabled=false
    ```

=== "Restrict to one namespace"
    ```bash
    helm install rpc-operator ./charts/rpc-operator \
      -n rpc-operator-system --create-namespace \
      --set 'operator.watchNamespaces[0]=data-eng'
    ```

=== "With Prometheus"
    ```bash
    helm install rpc-operator ./charts/rpc-operator \
      -n rpc-operator-system --create-namespace \
      --set operator.prometheusUrl=http://prometheus-operated.cattle-monitoring-system.svc:9090
    ```

For the full list of Helm values, see [Helm values reference](../operating/helm-values.md).

## Uninstall

See [Upgrades and uninstall](../operating/upgrade-uninstall.md).
