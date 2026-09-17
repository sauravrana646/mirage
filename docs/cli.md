# Mirage CLI

Developer-facing CLI for PreviewEnvironments. SCM and AI stay outside the operator reconcile loop; the CLI only reads/writes Kubernetes APIs.

## Build

```bash
make build-cli
# or
go build -o bin/mirage ./cmd/mirage
```

## Commands

```bash
mirage list
mirage describe pr-142
mirage url pr-142
mirage logs pr-142 [--service frontend] [-f]
mirage suspend pr-142
mirage resume pr-142
mirage delete pr-142
mirage create --pr 142 --image ghcr.io/org/app@sha256:... [--template standard-web-app] [--host ...]
mirage diagnose pr-142
mirage diagnose pr-142 --ai
mirage cost pr-142
```

`cost` estimates spend from CPU/memory requests (LimitRange-like defaults `50m`/`64Mi` when unset), ingress count, and preview age using hardcoded illustrative `$/hour` rates. It is **not** cloud billing; OpenCost (or similar) can replace this later.

Flags:

| Flag | Purpose |
|------|---------|
| `-n` / `--namespace` | Namespace of the PreviewEnvironment CR |
| `--kubeconfig` | Explicit kubeconfig path |

`diagnose` prints structured checks (`NamespaceReady`, `WorkloadReady`, `NetworkReady`, `RouteReady`), warning events, and ImagePull/CrashLoop hints. `--ai` points at the existing advisory side-path under `ai/` without calling an LLM from the CLI.
