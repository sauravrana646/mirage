# GitHub Actions preview

Reference workflow that builds an image, applies a `PreviewEnvironment`, and cleans up when the PR closes.

Canonical copy: [`config/samples/github-actions-preview.yaml`](../../config/samples/github-actions-preview.yaml). Production workflow in-repo: [`.github/workflows/preview.yml`](../../.github/workflows/preview.yml).

## Use

1. Install Mirage and the least-privilege CI Role (`config/rbac/ci_role.yaml`).
2. Store a base64 kubeconfig (or prefer OIDC) as secret `KUBE_CONFIG` limited to the `mirage-ci` SA.
3. Copy `github-actions-preview.yaml` into your app repo as `.github/workflows/preview.yml` (adjust image registry and ports).

```bash
# Inspect the sample from this repo
less github-actions-preview.yaml
```

SCM notify / status reporting: [scm-integration.md](../../docs/scm-integration.md). Hardening: [production.md](../../docs/production.md).
