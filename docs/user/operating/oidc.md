# OIDC SSO

> **Audience:** Platform admins
> **Prerequisites:** [Authentication modes](auth.md)

OIDC SSO adds a "Log in with SSO" button to the UI's login screen. It is additive: the Bearer-token login stays available as a fallback. The operator uses the OAuth 2.0 PKCE flow — no client secret required.

## How it works

1. The user clicks "Log in with SSO" — the operator redirects to the IdP's authorization endpoint
2. The user authenticates at the IdP; the IdP redirects back to `/api/v1/auth/callback`
3. The operator exchanges the code for tokens; the `id_token` is stored in an in-memory session
4. On every API request, the operator forwards the `id_token` as a Bearer token to the Kubernetes apiserver

!!! warning
    The refresh-token cache is in-memory and not shared across replicas. A single-replica deployment is required for session persistence. This is an F20b limitation; it will be addressed in a future release.

## Apiserver prerequisites

Your Kubernetes apiserver must be started with:

```text
--oidc-issuer-url=https://keycloak.example.com/realms/platform
--oidc-client-id=rpc-operator
```

These flags must match the `oidc.issuer` and `oidc.clientID` Helm values exactly.

## IdP registration

Register a **public OAuth 2.0 client** (no client secret) at your IdP with:

- **Redirect URI:** `https://rpc-operator.example.com/api/v1/auth/callback`
- **Allowed scopes:** `openid email offline_access` (the default request set)
- **PKCE:** required (S256)

!!! note "Scope requirements per IdP"
    - **Keycloak** (production): needs `openid email offline_access`; add an audience mapper if the apiserver rejects tokens with a 401
    - **Dex** (test): use `openid email` only (Dex does not support `offline_access` in all configurations)

## Install with OIDC

```bash
helm upgrade --install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system \
  --set auth.enabled=true \
  --set oidc.enabled=true \
  --set oidc.issuer=https://keycloak.example.com/realms/platform \
  --set oidc.clientID=rpc-operator \
  --set oidc.redirectURL=https://rpc-operator.example.com/api/v1/auth/callback \
  --set oidc.uiRedirectURL=https://rpc-operator.example.com/ \
  --set ingress.enabled=true \
  --set ingress.host=rpc-operator.example.com
```

!!! warning
    `oidc.enabled=true` requires `auth.enabled=true`, a non-empty `oidc.issuer`, `oidc.clientID`, and `oidc.redirectURL`. The chart fails rendering (`helm install` exits non-zero) if any of these are missing.

## Without `offline_access`

If your IdP does not support `offline_access`, remove it from `oidc.scopes`:

```bash
--set oidc.scopes=openid,email
```

Without a refresh token, the operator cannot silently re-authenticate expired sessions. The UI will redirect the user through a full SSO login on each expiry.

## Verify

After installing, port-forward and open the UI:

```bash
kubectl -n rpc-operator-system port-forward svc/rpc-operator 8082:8082
# Open http://localhost:8082 — you should see a "Log in with SSO" button
```
