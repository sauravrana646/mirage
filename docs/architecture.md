# Architecture

**Status:** Draft v0.1  
**Related:** [prd.md](./prd.md), [crd.md](./crd.md), [decisions.md](./decisions.md)

## 1. Overview

Mirage is an in-cluster operator. Desired state is a `PreviewEnvironment` custom resource. The controller reconciles Kubernetes objects (and later optional Argo CD Applications) until status reflects success, failure, or expiry.

```text
                    ┌─────────────────────┐
  GitHub PR events  │  CI / Action / Hook │  (Phase 4+)
                    └──────────┬──────────┘
                               │ upsert/delete CR
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                     Kubernetes API                            │
│  PreviewEnvironment (mirage.dev)                              │
└──────────────────────────────┬───────────────────────────────┘
                               │ watch / reconcile
                               ▼
                    ┌─────────────────────┐
                    │   Mirage controller │
                    │  (controller-runtime)│
                    └──────────┬──────────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
         Namespace        Deployment         Service
                           Ingress              │
                           (Phase 3)            ▼
                                           status.url

  Optional Phase 6: create Argo CD Application instead of
  (or in addition to) direct Deployment apply.
```

## 2. Components

| Component | Role |
|-----------|------|
| **CRD `PreviewEnvironment`** | API for one preview |
| **Mirage manager** | Reconcile loop, status, finalizers |
| **Child resources** | Namespace, Deployment, Service, Ingress, Quota… |
| **GitHub integration** | External; creates CRs (not in reconcile hot path) |
| **AI assistant** | External; reads status/events; writes comments/annotations |

## 3. Control flow (happy path)

1. CR created with `spec.image`, optional `spec.ttlSeconds`.
2. Reconcile ensures namespace + workload exist and are owned by the CR.
3. When pods Ready (and Ingress if enabled), set `Ready=True` and URL.
4. On CR delete or TTL expiry: finalizer/TTL logic deletes children; remove finalizer.

## 4. Failure flow

1. Apply or rollout fails → `Ready=False`, Reason/Message set, requeue with backoff.
2. External AI job (later) may comment on the PR; controller does not call LLM.

## 5. Trust boundaries

- **Cluster admin** installs Mirage + RBAC.
- **CI identity** may only create/update/delete `PreviewEnvironment` in allowed namespaces (tighten over time).
- **Workload** in preview namespaces should be constrained (quotas, optional NetworkPolicy).

See [security.md](./security.md).

## 6. Extensibility

- **Backends:** `direct` (default) vs `argocd` (Phase 6).
- **Sources:** image digest from CI first; git+build in-cluster is out of scope for early phases.
- **AI:** annotation/status fields as contracts; implementation outside manager.

## 7. Non-requirements of this diagram

- No multi-cluster agent mesh in v1.
- No persistent data plane beyond normal K8s objects.
