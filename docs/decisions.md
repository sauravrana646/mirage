# Design decisions

Append-only log. Newest entries at the **top**.  
Format inspired by ADRs — keep each entry short.

Template:

```markdown
## YYYY-MM-DD — Title

**Status:** Accepted | Proposed | Superseded by YYYY-MM-DD

**Context:** Why we needed a choice.

**Decision:** What we chose.

**Consequences:** Trade-offs, follow-ups.
```

---

## 2026-08-25 — Project name: Mirage

**Status:** Accepted

**Context:** Need a memorable name for an ephemeral PR-preview operator.

**Decision:** Name the project **Mirage** (appears for a PR, vanishes when done).

**Consequences:** Repo `sauravrana646/mirage`; API group tentatively `mirage.dev`.

---

## 2026-08-25 — Problem focus: PR preview environments

**Status:** Accepted

**Context:** Choose a learning project that teaches K8s controllers + Go and maps to CNCF contribution (especially Argo).

**Decision:** Build a PreviewEnvironment operator (not cluster provisioning, not a general app platform).

**Consequences:** Clear MVP; Argo CD integration is optional Phase 6, not a blocker for Phase 2.

---

## 2026-08-25 — Primary stack: Go + Kubebuilder

**Status:** Accepted

**Context:** CNCF controllers are overwhelmingly Go + controller-runtime.

**Decision:** Implement Mirage in Go using Kubebuilder / controller-runtime.

**Consequences:** Aligns with upstream contribution skills; envtest/kind as default test path.

---

## 2026-08-25 — Direct apply first, Argo later

**Status:** Accepted

**Context:** Argo CD ApplicationSet already covers some preview flows; learning controllers needs owning reconcile logic.

**Decision:** Phase 2 uses direct create/update of Namespace/Deployment/Service. Optional `backend: argocd` in Phase 6.

**Consequences:** Faster MVP; later dual-backend complexity must be behind a clear spec field.

---

## 2026-08-25 — GitHub Action before in-cluster webhook

**Status:** Accepted (for Phase 4 default)

**Context:** Need PR → CR wiring without operating a public webhook endpoint on day one.

**Decision:** Prefer GitHub Actions that apply/delete CRs; reconsider webhook/GitHub App if Action friction is high.

**Consequences:** Simpler security early; cluster credentials live in CI secrets (document least privilege).

---

## 2026-08-25 — AI stays outside reconcile

**Status:** Accepted

**Context:** Desire to add AI later for RCA and assist.

**Decision:** Controller never calls an LLM. AI reads status/events and writes PR comments or annotations.

**Consequences:** Deterministic tests; AI outages cannot block cleanup; see [ai-roadmap.md](./ai-roadmap.md).

---

## 2026-08-25 — License: Apache 2.0

**Status:** Accepted

**Context:** CNCF-friendly open source default.

**Decision:** Apache License 2.0.

**Consequences:** Standard contribution expectations if the project grows.

---

## 2026-08-25 — One namespace per preview (lean default)

**Status:** Proposed

**Context:** Isolation vs density.

**Decision (lean):** Default to one namespace per preview env.

**Consequences:** Easier quotas/NetworkPolicies; more namespaces. Revisit if control-plane load becomes an issue.

---

## OPEN — CR scope: Namespaced vs Cluster

**Status:** Proposed

**Context:** Namespaced CRs are easier to RBAC; Cluster scope can watch all previews centrally.

**Decision:** TBD in Phase 1 scaffold — document choice here when set.

**Consequences:** Affects Kubebuilder API markers and sample paths.
