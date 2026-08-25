# Product Requirements Document (PRD)

**Product:** Mirage  
**Owner:** Saurav Rana  
**Status:** Draft v0.1  
**Last updated:** 2026-08-25

## 1. Summary

Mirage is a Kubernetes operator that creates **ephemeral preview environments** for pull requests and deletes them when the PR is closed or a TTL expires. Users declare desire via a CRD (`PreviewEnvironment`); the controller reconciles cluster state to match.

## 2. Problem

- PR previews are valuable for review, but setup is often manual or brittle (scripts, one-off Argo apps).
- Leftover namespaces and load balancers waste money and create security noise.
- ApplicationSet PR generators help, but many teams want a **single, ownable controller** they understand and can extend (including later AI assist).

## 3. Goals

| ID | Goal |
|----|------|
| G1 | One CR ≈ one preview env (namespace + workload + access URL) |
| G2 | Reliable cleanup (finalizers + TTL) |
| G3 | Teachable Go controller design (status, requeue, ownership) |
| G4 | Path to GitHub + Argo CD integration without rewriting core |
| G5 | AI later as **advisory** path, not inside reconcile |

## 4. Non-goals (v1)

- Multi-cloud cluster provisioning (not a Cluster API / ksctl replacement)
- Full multi-tenant SaaS control plane
- Replacing Argo CD / Flux as general GitOps
- Running LLMs inside the reconcile loop

## 5. Users & jobs

| Persona | Job to be done |
|---------|----------------|
| Platform engineer | Offer safe PR previews with quotas and cleanup |
| App developer | Get a URL on every PR without YAML archaeology |
| Reviewer | Click a link, try the change, trust it matches the PR |

## 6. User stories

1. As a developer, when I open a PR, I want a preview URL commented (or visible in status) so reviewers can try the change.
2. As a platform engineer, when a PR closes, I want the namespace and dependent resources gone without manual `kubectl delete`.
3. As an operator author, I want clear `Ready` / `Failed` conditions so I can debug without reading logs first.
4. As a future user of AI assist, I want failure summaries on the PR without changing controller determinism.

## 7. Functional requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F1 | CRD `PreviewEnvironment` with source, image/ref, TTL, optional overrides | P0 |
| F2 | Create Namespace (or use specified ns) owned by the CR | P0 |
| F3 | Deploy a minimal app (Deployment + Service; Ingress later) | P0 |
| F4 | Status: phase/conditions, message, optional URL | P0 |
| F5 | Finalizer deletes owned resources on CR delete | P0 |
| F6 | TTL expiry triggers cleanup | P1 |
| F7 | GitHub webhook / Action creates/updates/deletes CRs | P1 |
| F8 | Optional: create Argo CD `Application` instead of direct apply | P2 |
| F9 | ResourceQuota / LimitRange defaults per preview | P2 |
| F10 | AI failure summary posted to PR (side service) | P3 |

## 8. Non-functional requirements

| ID | Requirement |
|----|-------------|
| N1 | Idempotent reconcile |
| N2 | Works on kind for local demos |
| N3 | Structured logs + Kubernetes events |
| N4 | Unit tests via `envtest`; smoke e2e on kind |
| N5 | Least-privilege RBAC for the manager |

## 9. Success metrics

- Create → Ready path works with only `kubectl apply -f sample.yaml`
- Delete CR → namespace gone within reconcile SLA (e.g. &lt; 2 minutes on kind)
- At least one end-to-end GitHub PR demo recorded in docs
- Decision log stays current (no silent architecture drift)

## 10. Milestones (summary)

See [plan.md](./plan.md) for detailed phases. High level:

1. Manual CR → namespace + deploy + status + finalizer  
2. TTL + Ingress URL  
3. GitHub integration  
4. Argo CD path  
5. AI advisory path  

## 11. Open questions

Record answers in [decisions.md](./decisions.md) when resolved.

- Namespace-per-PR vs shared namespace with name prefix?
- Image build in-cluster vs external CI pushes image digest into CR?
- Default ingress controller assumption (nginx, Traefik, none)?
