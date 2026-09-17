# Preview platform (P0)

Mirage is evolving from a single-workload operator into a **preview environment platform**.

## PreviewTemplate

Platform teams own defaults; developers only specify what changes per PR.

```yaml
apiVersion: mirage.dev/v1alpha1
kind: PreviewTemplate
metadata:
  name: standard-web-app
spec:
  resources:
    preset: small   # small | medium | large
  networkPolicy: baseline
  ingress:
    enabled: true
    domain: preview.example.com
  ttlSeconds: 86400
  security:
    profile: restricted
```

```yaml
kind: PreviewEnvironment
spec:
  templateRef:
    name: standard-web-app
  source:
    provider: github
    repo: org/myapp
    pullRequest: 142
  image: ghcr.io/org/myapp@sha256:...
  targetNamespace: preview-pr-142
```

Explicit PreviewEnvironment fields always win over the template.

## Multi-service previews

```yaml
spec:
  services:
    - name: api
      image: ghcr.io/acme/api@sha256:...
      port: 8080
    - name: frontend
      image: ghcr.io/acme/frontend@sha256:...
      port: 3000
      ingress:
        enabled: true
        host: frontend-pr-142.preview.example.com
    - name: worker
      image: ghcr.io/acme/worker@sha256:...
```

Each service gets a Deployment + Service (and optional Ingress) in `targetNamespace`. Status reports per-service readiness under `status.services`.

Single-service `spec.image` remains fully supported.

## Status conditions

| Condition | Meaning |
|-----------|---------|
| `NamespaceReady` | Target namespace owned and present |
| `WorkloadReady` | All preview Deployments available |
| `NetworkReady` | NetworkPolicy applied (or disabled by policy) |
| `RouteReady` | Ingress hosts configured when enabled |
| `Ready` / `Progressing` / `Expired` | Overall lifecycle |

Phase `Provisioning` is used while workloads are rolling out.

## Cost & resource metrics

The operator exposes Prometheus gauges for per-preview requested resources and age (labels: `name`, `namespace`, `target_namespace` — expected low cardinality for previews):

| Metric | Meaning |
|--------|---------|
| `mirage_preview_cpu_requested` | Requested CPU cores (defaults applied when unset) |
| `mirage_preview_memory_requested_bytes` | Requested memory bytes |
| `mirage_preview_storage_requested_bytes` | Storage requests (`0` until PVC/deps are tracked) |
| `mirage_preview_age_seconds` | Seconds since `CreationTimestamp` |
| `mirage_active_previews` | Count of previews currently tracked in metrics |

`mirage cost NAME` prints a deterministic USD estimate from the same request model × age × illustrative hourly rates (CPU/memory/ingress). Namespace ResourceQuota remains the hard cap; per-preview estimates sum service requests. OpenCost integration is a future option for actual allocation.

## CLI

See [cli.md](./cli.md). Boundary preserved: reconcile stays deterministic; SCM (`mirage-notify`) and AI (`ai/`) remain side paths.
