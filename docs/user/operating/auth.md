# Authentication modes

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

The RPC Operator API (`:8082`) supports three authentication modes. Choose one when you install the operator; you can change it with a Helm upgrade.

## Mode A — Auth off (dev / demo only)

```bash
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace \
  --set auth.enabled=false
```

All API requests run under the operator's ServiceAccount. No login is required. **Never expose this mode publicly.** Use it only for local development or isolated demo clusters.

## Mode B — Bearer token (default)

```bash
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace
# auth.enabled=true is the default
```

Users log in by pasting a Kubernetes service account token. The operator forwards the token to the apiserver on every request; native RBAC decides what the user can do.

### Create a user token

Replace `<pipeline-namespace>` with the namespace where your pipelines will run (e.g. `rpc-operator-poc` in the getting-started guide).

```bash
# 1. Create a ServiceAccount for the user
kubectl -n <pipeline-namespace> create serviceaccount alice

# 2. Bind RBAC — pipeline editor role
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: rpc-pipeline-editor
  namespace: <pipeline-namespace>
rules:
- apiGroups: ["rpc.operator.io"]
  resources: ["pipelines", "pipelines/status"]
  verbs: ["get","list","watch","create","update","patch","delete"]
- apiGroups: [""]
  resources: ["pods","pods/log","events","configmaps"]
  verbs: ["get","list","watch"]
EOF

kubectl -n <pipeline-namespace> create rolebinding alice-pipelines \
  --role=rpc-pipeline-editor --serviceaccount=<pipeline-namespace>:alice

# 3. Optional: allow the namespace dropdown in the UI to list namespaces
kubectl create clusterrolebinding alice-ns-list \
  --clusterrole=view --serviceaccount=<pipeline-namespace>:alice

# 4. Mint a short-lived token
kubectl -n <pipeline-namespace> create token alice --duration=8h
```

!!! warning "Broad cluster access"
    The `view` ClusterRole grants read access to resources across **all namespaces**. If least-privilege is required, create a custom ClusterRole granting only `namespaces` list/get instead.

Paste the token into the login screen or pass it as `Authorization: Bearer <token>` for API calls.

!!! warning
    Rancher clusters use their own kubeconfig tokens (`kubeconfig-u-…`) that route through Rancher's auth proxy — the Kubernetes apiserver does not accept them directly. Use ServiceAccount tokens (as above) or OIDC SSO instead.

## Mode C — Anonymous reads (status board)

Mode C adds unauthenticated read access on top of Mode B. Writes (create, update, delete) still require a token.

```bash
# Anonymous reads only:
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace \
  --set auth.enabled=true \
  --set anonymous.read.enabled=true

# Anonymous reads + live log stream:
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace \
  --set auth.enabled=true \
  --set anonymous.read.enabled=true \
  --set anonymous.logs.enabled=true
```

!!! warning
    Anonymous reads expose `spec.rawYAML` and `spec.secretRefs` names (not values). Use Mode C only on demo or status-board clusters — not on clusters with compliance requirements.

!!! note
    Browsers cannot set headers on `new WebSocket()`, so the logs endpoint accepts the token as a URL query parameter: `?token=<token>`. Ensure your ingress does not log query strings if they may contain tokens.

## OIDC SSO (additive)

OIDC login is an additional option that runs alongside Mode B. See [OIDC SSO](oidc.md).
