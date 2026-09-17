# Simple preview

Minimal path: platform `PreviewTemplate` plus a single-image `PreviewEnvironment`.

Canonical samples live in [`config/samples/`](../../config/samples/).

## Apply

```bash
# Template + PE (recommended)
kubectl apply -f previewtemplate_standard.yaml

# Or a standalone PE without a template
kubectl apply -f previewenvironment.yaml
```

Requires Mirage CRDs and a running manager — see [getting started](../../docs/getting-started.md).

## What you get

| File | Contents |
|------|----------|
| `previewtemplate_standard.yaml` | `PreviewTemplate/standard-web-app` + `PreviewEnvironment/pr-142` |
| `previewenvironment.yaml` | Single PE (`preview-sample`) with unprivileged nginx image |

```bash
./bin/mirage list
./bin/mirage diagnose pr-142
```
