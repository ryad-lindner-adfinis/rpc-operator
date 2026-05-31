# Contributing

Thank you for your interest in contributing to the RPC Operator!

## Development Workflow

1. **Fork and branch:** Create a feature branch from `main` for your changes.
2. **Local setup:** Follow the [README quickstart](README.md#local-development) to set up your development environment.
3. **Code quality:** Run tests and linters before submitting:
   ```bash
   make test      # Go tests
   make lint      # Linter
   cd ui && npx tsc --noEmit  # TypeScript type check
   ```
4. **Documentation:** User documentation changes require a CRD reference drift check (see below).

## User Documentation

The user documentation lives in `docs/user/` and is built with MkDocs Material. See
[README](README.md#user-documentation) for the published site URL.

### CRD Reference Drift Check

If you modify the `Pipeline` or `PipelineCluster` CRD spec fields (`api/v1alpha1/pipeline_types.go`
or `api/v1alpha1/pipelinecluster_types.go`), verify that the user documentation stays in sync:

```bash
make docs-check-reference
```

This script compares CRD Go struct fields against markdown headings in the reference documentation
and reports any mismatches. All field additions, removals, or renames **must** be reflected in
`docs/user/reference/` before merging. The check is enforced in CI.

## Commit and PR

Push to your fork and open a pull request against `main`. CI gates require:
- Tests passing (`make test`)
- Linter clean (`make lint`)
- Docs build successfully (`mkdocs build --strict`)
- Reference drift check clean (if CRD fields changed)

The CI image includes Node.js, Python, and Go toolchains; see `.forgejo/Dockerfile.ci` for the build environment.
