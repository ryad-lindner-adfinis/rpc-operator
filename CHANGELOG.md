# Changelog

All notable changes to this project are documented here.

## User Documentation v1 — 2026-05-31

**Commit:** `e51ae30`

First complete user documentation site for the RPC Operator. Covers all
operator features through v0.9, organized for two audiences: platform
administrators and pipeline authors.

### Sections

- **Introduction** — What the RPC Operator is, who should read what, and architecture overview with Mermaid diagrams
- **Getting Started** — Prerequisites, Helm install, first pipeline walkthrough, production readiness checklist
- **Operating the Operator** — Helm values reference, authentication modes (A/B/C), OIDC SSO, namespace allowlist, pull secrets, Prometheus integration, upgrades and uninstall
- **Authoring Pipelines** — Pipeline anatomy, secrets via secretKeyRef, deploying and redeploying, stop and re-run
- **PipelineClusters & Streams** — When to use a PipelineCluster, defining a cluster, running stream pipelines, migrating between clusters
- **Operations** — Reading logs (kubectl + WebSocket API + stream filtering), metrics and PodMonitor, troubleshooting guide
- **Reference** — Complete Pipeline CRD (16 fields), PipelineCluster CRD (8 fields), Helm values (47 rows), operator CLI flags (26 flags)

### Infrastructure

- MkDocs Material 9.5 with syntax highlighting, tabbed code blocks, Mermaid diagrams, full-text search
- CRD drift-check CI gate (`.drift-check-enabled`): fails if a CRD field is added to Go code without a corresponding `### fieldName` section in the reference docs
