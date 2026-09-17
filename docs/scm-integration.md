# Multi-SCM integration — GitHub, GitLab, Bitbucket

Mirage reports preview **commit status** and **PR/MR comments** from CI via `mirage-notify`.
The controller never calls SCM APIs (deterministic reconcile).

## Providers

| Provider | Status API | Comment API | Typical token |
|----------|------------|-------------|---------------|
| **GitHub** | Commit statuses | Issue/PR comments | `GITHUB_TOKEN` / App installation token |
| **GitLab** | Commit statuses | MR notes | `CI_JOB_TOKEN` / PAT / OIDC-brokered bearer |
| **Bitbucket** | Build statuses | PR comments | Repository / workspace access token |

Self-hosted: set `MIRAGE_SCM_BASE_URL` (e.g. `https://gitlab.example.com/api/v4`, `https://github.example.com/api/v3`, `https://api.bitbucket.org/2.0`).

## Auth modes

### `MIRAGE_SCM_AUTH=token` (default)

Uses `MIRAGE_SCM_TOKEN` or the provider’s CI token env vars (`GITHUB_TOKEN`, `GITLAB_TOKEN` / `CI_JOB_TOKEN`, `BITBUCKET_TOKEN`).

### `MIRAGE_SCM_AUTH=oidc` (SSO / short-lived)

For environments where SSO/OIDC mints a short-lived API bearer (identity broker, cloud IAM, GitLab OIDC → vault, etc.):

```bash
export MIRAGE_SCM_AUTH=oidc
export MIRAGE_OIDC_TOKEN_FILE=/var/run/secrets/oidc/token
# or: export MIRAGE_OIDC_TOKEN="$(cat ...)"
mirage-notify status --state pending --description "Deploying preview"
```

Notes:
- **GitHub Actions → cluster**: prefer GitHub OIDC to cloud IAM for kube access (not for GitHub API). GitHub API still uses `GITHUB_TOKEN` or a GitHub App.
- **GitLab OIDC**: can mint cloud credentials; for GitLab API, `CI_JOB_TOKEN` or a project access token is usual.
- **Bitbucket**: use workspace/repo access tokens; OAuth clients can mint bearers for `auth=oidc`.

## CLI

```bash
go build -o bin/mirage-notify ./cmd/mirage-notify

# Commit status (appears on the PR/MR checks list)
mirage-notify status --state pending --description "Deploying preview"
mirage-notify status --state success --target-url "https://pr-42.preview.example.com" --description "Preview ready"

# Upsert PR/MR comment (idempotent via HTML marker)
mirage-notify comment --pr 42 --phase Ready --url "https://..." --image "ghcr.io/org/app@sha256:..." --namespace preview-pr-42
```

### Required env by provider

**GitHub:** `MIRAGE_SCM_PROVIDER=github`, `MIRAGE_SCM_OWNER`, `MIRAGE_SCM_REPO` (or `GITHUB_REPOSITORY`), token.

**GitLab:** `MIRAGE_SCM_PROVIDER=gitlab`, `MIRAGE_SCM_PROJECT_ID` (or `CI_PROJECT_ID` / `CI_PROJECT_PATH`), token.

**Bitbucket:** `MIRAGE_SCM_PROVIDER=bitbucket`, `MIRAGE_SCM_WORKSPACE`, `MIRAGE_SCM_REPO`, token.

Also set commit/PR: `MIRAGE_SCM_SHA`, `MIRAGE_SCM_PR` (or CI equivalents).

## CI templates

| Platform | File |
|----------|------|
| GitHub Actions | [`.github/workflows/preview.yml`](../.github/workflows/preview.yml) |
| GitLab CI | [`.gitlab-ci.yml`](../.gitlab-ci.yml) |
| Bitbucket Pipelines | [`bitbucket-pipelines.yml`](../bitbucket-pipelines.yml) |

## CRD source fields

```yaml
spec:
  source:
    provider: github   # github | gitlab | bitbucket
    repo: https://github.com/org/app
    pullRequest: 42
    commitSHA: abcdef
    branch: feature/x
    projectID: group/app          # GitLab
    workspace: my-workspace       # Bitbucket
    repoSlug: app                 # Bitbucket (optional)
```

## Cluster auth (OIDC)

Prefer short-lived kube credentials over long-lived `KUBE_CONFIG` secrets:

1. CI obtains an OIDC token from the platform.
2. Exchange for cloud IAM / vault → short-lived kubeconfig or exec plugin.
3. `kubectl apply` the `PreviewEnvironment`.
4. `mirage-notify` reports status/comments using the SCM token path above.

Sample least-privilege CI Role: [`config/rbac/ci_role.yaml`](../config/rbac/ci_role.yaml).
