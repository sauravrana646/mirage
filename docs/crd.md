# CRD design — `PreviewEnvironment`

**API group (proposed):** `mirage.dev`  
**Kind:** `PreviewEnvironment`  
**Scope:** Cluster or Namespaced — **decision pending** (see [decisions.md](./decisions.md)); default lean: **Namespaced**.

This is a sketch for Phase 1–2. Fields will evolve; update this doc and log breaking changes in decisions.

## Spec (v1alpha1 sketch)

```yaml
apiVersion: mirage.dev/v1alpha1
kind: PreviewEnvironment
metadata:
  name: pr-42-myapp
  namespace: mirage-system   # if namespaced CR
spec:
  # Identity / source
  source:
    repo: https://github.com/org/app
    pullRequest: 42
    commitSHA: abcdef0123

  # What to run
  image: ghcr.io/org/app:pr-42
  # optional later: helm chart / kustomize path

  # Lifecycle
  ttlSeconds: 86400          # 24h; controller enforces expiry
  # expiresAt: "2026-08-26T12:00:00Z"  # alternative absolute time

  # Placement
  targetNamespace: preview-pr-42   # created if missing

  # Runtime tweaks
  replicas: 1
  env: []
  resources: {}

  # Backend (Phase 6)
  backend: direct              # direct | argocd

  # Ingress (Phase 3)
  ingress:
    enabled: true
    host: pr-42.preview.example.com
```

## Status (sketch)

```yaml
status:
  phase: Ready                 # Pending | Ready | Failed | Expiring
  url: https://pr-42.preview.example.com
  expiresAt: "2026-08-26T12:00:00Z"
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      reason: WorkloadReady
      message: Deployment available
      lastTransitionTime: "..."
```

## Ownership model

- CR owns Namespace (or owns workloads inside a dedicated namespace).
- Prefer **one namespace per preview** for blast-radius isolation.

## Samples

Phase 2 will add `config/samples/mirage_v1alpha1_previewenvironment.yaml`.

## Validation (later)

- Kubebuilder markers / CEL for required image, non-negative TTL.
- Webhook only if markers are insufficient.
