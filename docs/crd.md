# CRD design — `PreviewEnvironment`

**API group (proposed):** `mirage.dev`  
**Kind:** `PreviewEnvironment`  
**Scope:** **Namespaced** (accepted — see [decisions.md](./decisions.md)).  
**Typical CR namespace:** `mirage-system` (management); workloads run in `spec.targetNamespace`.

This is a sketch for Phase 1–2. Fields will evolve; update this doc and log breaking changes in decisions.

## Spec (v1alpha1 sketch)

```yaml
apiVersion: mirage.dev/v1alpha1
kind: PreviewEnvironment
metadata:
  name: pr-42-myapp
  namespace: mirage-system
spec:
  # Identity / source (optional early; useful for GitHub Phase 4)
  source:
    repo: https://github.com/org/app
    pullRequest: 42
    commitSHA: abcdef0123

  # What to run (required)
  image: ghcr.io/org/app@sha256:...   # prefer digest over mutable tag
  # optional later: helm chart / kustomize path

  # Lifecycle — ttlSeconds only in v1alpha1 (status.expiresAt is computed)
  ttlSeconds: 86400          # 24h; controller enforces expiry

  # Placement — one namespace per preview
  targetNamespace: preview-pr-42   # created if missing; finalizer-managed

  # Runtime tweaks
  replicas: 1
  env: []
  resources: {}

  # Backend (Phase 6)
  backend: direct              # direct | argocd

  # Ingress (Phase 3) — nginx + nip.io on kind by default
  ingress:
    enabled: true
    host: pr-42.<node-ip>.nip.io
```

## Status (sketch)

```yaml
status:
  phase: Ready                 # Pending | Ready | Failed | Expiring
  url: https://pr-42.<node-ip>.nip.io
  expiresAt: "2026-08-26T12:00:00Z"   # set by controller from ttlSeconds
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      reason: WorkloadReady
      message: Deployment available
      lastTransitionTime: "..."
```

### Condition reasons (initial set)

| Reason | When |
|--------|------|
| `WorkloadReady` | Deployment available (and Ingress Ready if enabled) |
| `NamespaceConflict` | `targetNamespace` exists but is not owned by this CR |
| `ImageInvalid` | Missing/empty image or rejected by validation |
| `RolloutFailed` | Deployment not progressing / ImagePullBackOff surfaced |
| `Expiring` | TTL passed; cleanup in progress |

## Ownership model

- CR is Namespaced; it **cannot** owner-reference the Namespace object.
- Controller **finalizer** creates/deletes `spec.targetNamespace`.
- Prefer **ownerReferences** from the CR (via a controller reference on children in that namespace) for Deployment, Service, Ingress, ResourceQuota, LimitRange, NetworkPolicy.
- Label/annotate `targetNamespace` with CR identity (`mirage.dev/owner-uid`, etc.) so conflict detection works.
- Prefer **one namespace per preview** for blast-radius isolation (accepted).

## Samples

Phase 2 will add `config/samples/mirage_v1alpha1_previewenvironment.yaml`.

## Validation (later)

- Kubebuilder markers / CEL: required `image`, non-negative `ttlSeconds`, required `targetNamespace`.
- Webhook only if markers are insufficient.
