# Image registry

> **Audience:** Platform admins
> **Prerequisites:** [Install via Helm](../getting-started/install.md)

The default operator image (`ghcr.io/insidegreen/rpc-operator`) and the default pipeline image (`docker.redpanda.com/redpandadata/connect:4`) are both public — no pull secret is needed for a standard install.

This page covers the two cases where authentication is required: custom pipeline images from private registries and air-gapped clusters.

## Pipeline pod image from a private registry

Each `Pipeline` CR runs a Redpanda Connect container. Override the image per pipeline using [`spec.image`](../authoring/anatomy.md#specimage):

```yaml
spec:
  image: my-private-registry.example.com/redpanda-connect:custom
  rawYAML: |
    ...
```

If the pipeline image is in a private registry, create a pull secret in the **pipeline namespace** and patch the pipeline's ServiceAccount — or inject it via a mutating webhook outside the operator's scope.

## Air-gapped clusters

For air-gapped environments:

1. Mirror `docker.redpanda.com/redpandadata/connect:4` and `ghcr.io/insidegreen/rpc-operator` to your internal registry
2. Set `image.repository` in Helm values to your internal registry path; add `imagePullSecrets` if your registry requires authentication
3. Override `spec.image` in each Pipeline CR to point to the mirrored Redpanda Connect image
