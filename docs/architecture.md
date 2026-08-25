# Architecture

**Status:** Draft v0.1  
**Related:** [prd.md](./prd.md), [crd.md](./crd.md), [decisions.md](./decisions.md)

## 1. Overview

Mirage is an in-cluster operator. Desired state is a namespaced `PreviewEnvironment` custom resource (typically in `mirage-system`). The controller reconciles Kubernetes objects (and later optional Argo CD Applications) until status reflects success, failure, or expiry.

```text
                    ┌─────────────────────┐
  GitHub PR events  │  CI / Action        │  (Phase 4+)
  + image build     │  build → GHCR       │
                    │  upsert/delete CR   │
                    └──────────┬──────────┘
                               │ PreviewEnvironment in mirage-system
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                     Kubernetes API                            │
│  PreviewEnvironment (mirage.dev) — Namespaced                 │
└──────────────────────────────┬───────────────────────────────┘
                               │ watch / reconcile
                               ▼
                    ┌─────────────────────┐
                    │   Mirage controller │
                    │  (controller-runtime)│
                    └──────────┬──────────┘
                               │
         finalizer-managed     │     ownerRefs (namespaced only)
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
         Namespace        Deployment         Service
    (targetNamespace)                      Ingress (Phase 3)
                                           Quota/LimitRange
                                               │
                                               ▼
                                           status.url
                                           status.expiresAt

  Optional Phase 6: create Argo CD Application instead of
  (or in addition to) direct Deployment apply.
```

## 2. Components

| Component | Role |
|-----------|------|
| **CRD `PreviewEnvironment`** | Namespaced API for one preview |
| **Mirage manager** | Reconcile loop, status, finalizers |
| **Child resources** | target Namespace (finalizer), Deployment, Service, Ingress, Quota… |
| **GitHub integration** | External; builds image + creates CRs (not in reconcile hot path) |
| **AI assistant** | External; reads status/events; writes comments/annotations |

## 3. Control flow (happy path)

1. CR created with `spec.image`, `spec.targetNamespace`, optional `spec.ttlSeconds`.
2. Reconcile ensures namespace + workload exist; sets `status.expiresAt` from TTL.
3. When pods Ready (and Ingress if enabled), set `Ready=True` and URL.
4. On CR delete or TTL expiry: finalizer deletes children + target namespace; CR is removed.

## 4. Failure flow

1. Apply, conflict, or rollout fails → `Ready=False`, Reason/Message set, requeue with backoff.
2. External AI job (later) may comment on the PR; controller does not call LLM.

## 5. Trust boundaries

- **Cluster admin** installs Mirage + RBAC.
- **CI identity** may only create/update/delete `PreviewEnvironment` in allowed namespaces (e.g. `mirage-system`) — see Phase 4 RBAC manifest.
- **Workload** in preview namespaces should be constrained (quotas early; optional NetworkPolicy in Phase 5).

See [security.md](./security.md).

## 6. Extensibility

- **Backends:** `direct` (default) vs `argocd` (Phase 6).
- **Sources:** image digest from CI first; git+build in-cluster is out of scope for early phases.
- **AI:** annotation/status fields as contracts; implementation outside manager.

## 7. Non-requirements of this diagram

- No multi-cluster agent mesh in v1.
- No persistent data plane beyond normal K8s objects.
- No Gateway API in the first ingress path (nginx + nip.io on kind).
