# Namespace allowlist

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

By default the RPC Operator watches all namespaces in the cluster (cluster-wide mode). The namespace allowlist restricts the operator to specific namespaces — useful for multi-tenant clusters where different teams each own a namespace.

## Configure the allowlist

Set `operator.watchNamespaces` to a list of namespace names:

```bash
helm upgrade rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system \
  --set 'operator.watchNamespaces[0]=data-eng' \
  --set 'operator.watchNamespaces[1]=analytics'
```

Or in a values file:

```yaml
operator:
  watchNamespaces:
    - data-eng
    - analytics
```

With the allowlist set, the operator:
- Only caches and reconciles `Pipeline` and `PipelineCluster` CRs in listed namespaces
- Only exposes listed namespaces in its API (the namespace dropdown in the UI shows only allowed namespaces)

## Cluster-wide mode (default)

Leave `operator.watchNamespaces` as `[]` (the default) to watch all namespaces. The operator's ClusterRole already has the necessary permissions.

## Effects on authentication

In Mode B (Bearer token), the operator forwards the user's token to the apiserver for every request. Kubernetes RBAC determines what the user can actually see and do — the allowlist is an additional filter at the operator level, not a replacement for RBAC.

In Mode A (auth off) and Mode C (anonymous reads), the operator's own ServiceAccount performs requests. The allowlist still limits which namespaces are visible.

!!! note
    Per-namespace ServiceAccount Roles are the recommended RBAC setup for multi-tenant clusters. See [Authentication modes](auth.md) for how to create per-user role bindings.
