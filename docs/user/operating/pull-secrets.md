# Pull secrets and image registry

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

The RPC Operator image and the Redpanda Connect pipeline image may live in private registries that require authentication.

## Operator image pull secret

The default operator image is in a private Forgejo registry. Create a pull secret in the operator namespace before installing:

```bash
kubectl create secret docker-registry forgejo-pull \
  --docker-server=forgejo.thecloudroute.com \
  --docker-username=<your-username> \
  --docker-password=<your-PAT> \
  -n rpc-operator-system
```

Reference it in the Helm values:

```bash
helm install rpc-operator ./charts/rpc-operator \
  -n rpc-operator-system --create-namespace \
  --set 'imagePullSecrets[0].name=forgejo-pull'
```

Or in a values file:

```yaml
imagePullSecrets:
  - name: forgejo-pull
```

## Pipeline pod image

Each `Pipeline` CR runs a Redpanda Connect container. The default image is `docker.redpanda.com/redpandadata/connect:4` (public registry, no pull secret needed).

Override the image per pipeline using [`spec.image`](../authoring/anatomy.md#specimage):

```yaml
spec:
  image: my-private-registry.example.com/redpanda-connect:custom
  rawYAML: |
    ...
```

If the pipeline image is in a private registry, create a pull secret in the **pipeline namespace** and patch the pipeline's ServiceAccount — or inject it via a mutating webhook outside the operator's scope.

## Air-gapped clusters

For air-gapped environments:

1. Mirror `docker.redpanda.com/redpandadata/connect:4` and the operator image to your internal registry
2. Set `image.repository` in Helm values to your internal registry path
3. Override `spec.image` in each Pipeline CR to point to the mirrored Redpanda Connect image
