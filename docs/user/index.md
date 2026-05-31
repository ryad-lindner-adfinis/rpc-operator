# RPC Operator User Documentation

Welcome to the RPC Operator documentation. This guide covers installation, configuration, pipeline authoring, and operations.

## Quick Links

- [Install the Operator](getting-started/install.md)
- [Deploy Your First Pipeline](getting-started/first-pipeline.md)
- [Pipeline CRD Reference](reference/pipeline-crd.md)
- [PipelineCluster CRD Reference](reference/pipelinecluster-crd.md)

## What is the RPC Operator?

The RPC Operator is a Kubernetes operator for managing Redpanda Connect pipelines. It provides:

- **Simple deployment** of data pipelines as Kubernetes resources
- **Native K8s integration** via Custom Resource Definitions (CRDs)
- **Multi-cluster support** via PipelineCluster for distributed stream processing
- **Built-in monitoring** with Prometheus integration

## For Platform Administrators

See the [Operating the Operator](operating/helm-values.md) section for deployment, authentication, monitoring, and cluster configuration.

## For Pipeline Authors

See the [Authoring Pipelines](authoring/anatomy.md) section for pipeline configuration, processors, input/output connectors, and error handling.
