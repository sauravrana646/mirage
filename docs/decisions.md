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

## 2026-09-17 — API group served as mirage.dev

**Status:** Accepted

**Context:** Kubebuilder defaulted to `mirage.mirage.dev` (group + domain). Docs and PRD use `mirage.dev/v1alpha1`.

**Decision:** Override `+groupName` / GroupVersion to **`mirage.dev`**.

**Consequences:** CRD file is `mirage.dev_previewenvironments.yaml`; samples use `apiVersion: mirage.dev/v1alpha1`.

---

## 2026-09-17 — No cross-namespace ownerReferences

**Status:** Accepted

**Context:** Namespaced CRs cannot owner-reference objects in another namespace (or Namespaces).

**Decision:** Label workloads/namespaces with `mirage.dev/owner-uid`; cleanup deletes the owned `targetNamespace` via finalizer. Do not call `SetControllerReference` across namespaces.

**Consequences:** GC will not cascade from CR deletion alone — finalizer is mandatory. Matches earlier ownership ADR.

---

## 2026-09-17 — Production feature set

**Status:** Accepted

**Context:** Move beyond MVP to shared-cluster usable operator.

**Decision:** Ship validating webhook policies, Helm chart (HA + PDB + topology spread), GitHub preview Action, release workflow, Argo CD backend via unstructured Applications, AI advisory side path, suspend, probes, TLS ingress, NetworkPolicy modes, Prometheus metrics, Events, scheduling fields (nodeSelector/tolerations/affinity), default/max TTL env policy, Progressing/Expired conditions.

**Consequences:** Larger surface area; webhook requires cert-manager (or Helm-generated certs) for cluster installs; Argo backend needs Argo CD CRDs present.

---

## 2026-09-17 — Cross-namespace ownership via labels

**Status:** Accepted

**Context:** Preview workloads live in `targetNamespace`; the CR lives in a management namespace. Kubernetes forbids cross-namespace ownerReferences.

**Decision:** Label children with `mirage.dev/owner-*` and watch Deployments/Services/Ingresses; finalizer owns Namespace lifecycle. Do not attempt ownerRef from CR to children.

**Consequences:** GC is reconcile-driven, not kube-controller-manager ownerRef GC.

---

## 2026-09-17 — Security hardening after review

**Status:** Accepted

**Context:** Security/bugbot reviews found AI `workflow_run` checkout of PR SHA with secrets, unconstrained Argo destinations, and open NetworkPolicy ingress.

**Decision:** AI advisory always checks out the default branch and skips fork `workflow_run`; Argo destination forced to `targetNamespace`; baseline NP requires `mirage.dev/ingress-access=true`; deny nginx snippet annotations; ship `values-production.yaml`.

**Consequences:** Operators must label ingress namespaces; demos may use `networkPolicy: permissive`.

---

## 2026-09-17 — Immutable targetNamespace

**Status:** Accepted

**Context:** Changing `spec.targetNamespace` after create provisioned a new env but left the old namespace running (orphan leak).

**Decision:** Mark `targetNamespace` **immutable** via CRD CEL (`XValidation`).

**Consequences:** Callers must delete/recreate the CR to move namespaces; cleanup always matches the single owned namespace.

---

## 2026-09-17 — Preview isolation defaults

**Status:** Accepted

**Context:** Security review: untrusted PR images need PSA, network, and identity baselines before shared-cluster demos.

**Decision:** Preview namespaces get PSA `restricted` labels, ResourceQuota/LimitRange, baseline NetworkPolicy (DNS egress + app-port ingress), restricted container securityContext, and `automountServiceAccountToken: false`. Cap `replicas` at 5.

**Consequences:** Images must run as non-root (sample uses nginx-unprivileged). Apps needing broader egress need a future escape hatch.

---

## 2026-08-25 — Default ingress: nginx + nip.io on kind

**Status:** Accepted (Phase 3)

**Context:** Need one reachable URL recipe for demos without operating real DNS.

**Decision:** Phase 3 supports **nginx ingress controller** on kind and **nip.io-style** hosts (e.g. `pr-42.<node-ip>.nip.io`). Other controllers (Traefik, Gateway API) are out of scope until after the first demo works.

**Consequences:** Docs and samples assume nginx; Gateway API remains a later option noted in the plan.

---

## 2026-08-25 — TTL: ttlSeconds only; expiry deletes the CR

**Status:** Accepted (v1alpha1)

**Context:** CRD sketch had both `ttlSeconds` and `expiresAt`; cleanup semantics were ambiguous (delete workload vs mark expired vs delete CR).

**Decision:** v1alpha1 exposes **`spec.ttlSeconds` only**. On create (or first observe), controller sets `status.expiresAt = now + ttl`. When past expiry: set `phase=Expiring`, delete children / target namespace, then **delete the PreviewEnvironment CR** (or remove finalizer so the CR goes away). No long-lived `Expired` tombstone in v1.

**Consequences:** Simpler API; audit of past previews is via Events/logs, not retained CRs. Absolute `expiresAt` in spec can be added later if needed.

---

## 2026-08-25 — Ownership: Namespaced CR + finalizer-managed targetNamespace

**Status:** Accepted

**Context:** A Namespaced CR cannot owner-reference a cluster-scoped Namespace. Need isolation without forcing Cluster scope.

**Decision:**
- `PreviewEnvironment` is **Namespaced** (default install ns: `mirage-system`).
- Each preview gets its own **`spec.targetNamespace`**.
- Controller uses a **finalizer** on the CR to create/delete that namespace; **ownerReferences** apply only to namespaced children *inside* `targetNamespace` (Deployment, Service, Ingress, Quota, …).
- Do **not** set ownerRef from the CR onto the Namespace object.

**Consequences:** Clear blast radius; CI can be RBAC’d to Mirage CRs in `mirage-system` without cluster-admin. Cluster-scoped CR remains possible later if we want Namespace ownerRefs.

---

## 2026-08-25 — CR scope: Namespaced

**Status:** Accepted (supersedes OPEN — CR scope)

**Context:** Namespaced CRs are easier to RBAC; Cluster scope can watch all previews centrally.

**Decision:** **Namespaced** API for `PreviewEnvironment`.

**Consequences:** Kubebuilder scaffold uses namespaced markers; samples live under a management namespace; see ownership decision above.

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

**Status:** Accepted (was Proposed)

**Context:** Isolation vs density.

**Decision:** Default to one namespace per preview env (`spec.targetNamespace`).

**Consequences:** Easier quotas/NetworkPolicies; more namespaces. Revisit if control-plane load becomes an issue.
