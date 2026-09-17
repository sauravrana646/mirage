# Multi-service preview

One `PreviewEnvironment` with `spec.services[]` (api, frontend, worker). Each service gets a Deployment + Service; optional per-service Ingress.

Apply the template first (or ensure `standard-web-app` already exists), then the multi-service PE.

## Apply

```bash
kubectl apply -f previewtemplate_standard.yaml
kubectl apply -f previewenvironment_multiservice.yaml
```

```bash
./bin/mirage describe pr-142-multi
./bin/mirage logs pr-142-multi --service frontend
./bin/mirage diagnose pr-142-multi
```

Design notes: [preview-platform.md](../../docs/preview-platform.md). Canonical YAML: [`config/samples/previewenvironment_multiservice.yaml`](../../config/samples/previewenvironment_multiservice.yaml).
