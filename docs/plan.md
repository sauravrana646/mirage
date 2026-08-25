# Mirage — Phase-wise build plan

Follow phases in order. Do not skip the “done when” checks. After each phase, append any design choices to [decisions.md](./decisions.md).

Locked defaults (see decisions): **Namespaced** CR, **finalizer-managed** `targetNamespace`, **`ttlSeconds` only**, nginx + nip.io for Phase 3 URLs.

---

## Phase 0 — Foundations (tools & mental model)

**Goal:** Be able to run a cluster and explain a reconcile loop.

### Steps
1. Install: Go 1.22+, Docker, kubectl, kind, kubebuilder, Helm, `gh`.
2. Create a kind cluster; deploy a sample Deployment/Service by hand.
3. Read (skim): Kubebuilder book — *CronJob* tutorial concepts (CRD, reconcile, status).
4. Clone this repo; keep docs as source of truth.

### Learn
- Pods, Deployments, Services, Namespaces, RBAC basics  
- Go modules, `context`, errors  

### Done when
- [ ] `kubectl get nodes` works on kind  
- [ ] You can explain: desired state → reconcile → status in one paragraph  

---

## Phase 1 — Operator skeleton (P0)

**Goal:** Empty Mirage operator that installs on kind.

### Steps
1. `kubebuilder init` / `create api` for group `mirage.dev`, kind `PreviewEnvironment`, **namespaced**.
2. Generate manifests; install CRD + manager on kind (manager in `mirage-system`).
3. Add Makefile targets: `make generate`, `manifests`, `install`, `run` / `deploy`.
4. Commit scaffold; update README with “dev loop”.

### Learn
- Kubebuilder layout, code generation, manager main  

### Done when
- [ ] CRD installed; `kubectl get previewenvironments -n mirage-system` works  
- [ ] Manager pod Running (or `make run` against kind)  

---

## Phase 2 — Core reconcile (P0)

**Goal:** Manual CR creates a working preview app and cleans up on delete.

Split into **2a** (create path) and **2b** (delete + failure) if velocity drops — do not start Phase 3 until both are green.

### Ownership model (do this first)

- CR lives in a management namespace (e.g. `mirage-system`).
- `spec.targetNamespace` is the preview workload namespace (one per CR).
- **Finalizer** on the CR: on delete, remove children then delete `targetNamespace`, then remove finalizer.
- **ownerReferences** only on namespaced objects *inside* `targetNamespace` (Deployment, Service, later Ingress/Quota). Never ownerRef the Namespace from a namespaced CR.
- If `targetNamespace` already exists and is **not** labeled/annotated as owned by this CR → `Ready=False`, Reason `NamespaceConflict`; do not hijack it.

### Steps — 2a Create path
1. Finalize CRD fields (see [crd.md](./crd.md)); regenerate.
2. Reconcile ensure:
   - Namespace `spec.targetNamespace` (create if missing; label with CR uid/name)
   - Deployment + Service (`spec.image`, replicas/env/resources as supported)
3. Status conditions: `Ready` with Reason/Message; set `phase` Pending → Ready.
4. Sample YAML under `config/samples/`.
5. Unit tests for create/idempotent update (`envtest`).

### Steps — 2b Delete + failure path
1. Finalizer cleanup: delete owned children and `targetNamespace` on CR delete.
2. Failure cases → `Ready=False` + requeue backoff:
   - invalid/missing image
   - Deployment rollout / ImagePullBackOff (surface message from pods/events)
   - `NamespaceConflict` as above
3. Unit tests for delete and at least one failure branch.

### Learn
- Idempotent create/update, finalizers, conditions, ownerRefs, requeue  

### Done when
- [ ] `kubectl apply -f config/samples/...` → pods Ready; status Ready  
- [ ] `kubectl delete` CR → target namespace and children gone  
- [ ] Conflict / pull failure sets Failed (or Ready=False) meaningfully  
- [ ] envtest covers create + delete (and one failure if practical)  

---

## Phase 3 — URL + TTL (P1)

**Goal:** Previews are reachable and time-bounded.

### TTL semantics (locked)
- Spec field: **`ttlSeconds` only** (no `spec.expiresAt` in v1alpha1).
- On first reconcile (or when ttl is set), compute and write `status.expiresAt`.
- When `now >= status.expiresAt`: set `phase=Expiring`, tear down children + target namespace, then **delete the CR** (same cleanup path as user delete).
- Emit a Kubernetes Event on expiry.

### Ingress recipe (locked for demos)
- Install **nginx ingress controller** on kind; document exact commands in README or `docs/kind-ingress.md`.
- Host pattern: nip.io (or equivalent) from node IP; write URL into `status.url` when Ingress is Ready.

### Steps
1. Add optional Ingress (`spec.ingress.enabled` / host) + `status.url`.
2. Implement TTL as above; `RequeueAfter` until expiry.
3. Kubernetes Events for create / ready / expire / fail.
4. Document kind + nginx + nip.io setup; sample with ingress enabled.

### Learn
- Ingress, time-based reconcile (`RequeueAfter`), Events API  

### Done when
- [ ] Status includes a URL reachable via the documented kind ingress setup  
- [ ] Expired preview is fully cleaned up (namespace + CR gone) without manual delete  

---

## Phase 4 — GitHub integration (P1)

**Goal:** PR open/sync/close drives CRs.

### Prerequisite: image pipeline
Phase 4 cannot succeed without a digests-on-PR build. Before wiring CR apply:

1. Add a minimal sample app (or document using an existing public image for a dry run).
2. GitHub Actions workflow: on PR → build image → push to GHCR (or similar) tagged by PR/SHA.
3. Later steps consume that **image digest** in the `PreviewEnvironment` spec.

### Steps
1. Choose path A: GitHub Action applies CR YAML; or path B: small webhook receiver.
   - Prefer **Action first** (already decided).
2. Ship a **least-privilege RBAC** manifest for the CI identity: create/update/delete `PreviewEnvironment` in `mirage-system` (+ get/list/watch as needed). No cluster-admin kubeconfig in CI.
3. On `opened`/`synchronize`: upsert `PreviewEnvironment` with image digest from the build job.
4. On `closed`: delete CR.
5. Comment preview URL on PR when Ready (Action polls status, or annotation hook).
6. Document required secrets/permissions (GHCR push, kube credentials, GitHub `pull-requests: write` for comments).

### Learn
- CI → cluster auth (kubeconfig / OIDC later), GitHub Checks/comments  

### Done when
- [ ] Open PR → image built → preview appears  
- [ ] Close PR → preview gone  
- [ ] Push to PR → preview updates (new image digest)  
- [ ] CI uses documented non-admin RBAC  

---

## Phase 5 — Hardening (P1/P2)

**Goal:** Safer defaults for shared clusters.

> **Note:** Minimal ResourceQuota + LimitRange per preview namespace should land as soon as demos leave a single-user kind cluster — ideally by end of Phase 3 if sharing early. Phase 5 makes them default and adds NetworkPolicy + packaging.

### Steps
1. Default ResourceQuota + LimitRange per preview namespace (aligns with PRD F9; treat as **P1** once shared).
2. Optional NetworkPolicy (deny egress except DNS/needed).
3. Metrics (controller-runtime metrics) + basic Grafana/Prometheus notes.
4. Helm chart for installing Mirage.
5. kind e2e smoke in CI (GitHub Actions).

### Learn
- Multi-tenant baselines, operator packaging, CI for controllers  

### Done when
- [ ] Chart installs Mirage cleanly  
- [ ] New previews get quota/limit defaults  
- [ ] CI runs unit tests; optional kind job  

---

## Phase 6 — Argo CD path (P2)

**Goal:** Optional GitOps backend instead of direct apply.

### Steps
1. Feature flag / `spec.backend: direct|argocd`.
2. When `argocd`: create/update Argo `Application`; still own lifecycle via Mirage CR.
3. Document overlap vs ApplicationSet PR generator (when to use which).
4. Map learnings → possible Argo CD contribution notes (good-first-issues list).

### Learn
- Argo Application API, sync status vs Mirage conditions  

### Done when
- [ ] Same sample app deployable via Argo backend  
- [ ] Cleanup still works  

---

## Phase 7 — AI advisory path (P3)

**Goal:** AI helps humans; controller stays deterministic.

### Steps
1. Follow [ai-roadmap.md](./ai-roadmap.md).
2. On Failed condition: side job gathers events/logs → LLM summary → PR comment.
3. Optional: suggest resource requests as CR annotations; human/CI applies.
4. Never block reconcile on LLM availability.

### Done when
- [ ] Failed preview gets a useful PR comment without controller changes that call an LLM  

---

## Phase 8 — Share & contribute

**Goal:** Portfolio + optional CNCF contribution pipeline.

### Steps
1. Public demo GIF/docs; pin architecture diagram.
2. Write “what I learned” comparing Mirage to Argo ApplicationSet.
3. Optionally pick CNCF issues (Argo CD / controller-runtime examples / Cluster API addons) that match skills from Phases 2–6.
4. Keep decisions.md honest — include mistakes.

### Done when
- [ ] Repo is demoable by a stranger from README alone  
- [ ] (Aspirational, not a blocker) At least one upstream PR opened (even docs/tests)  

---

## Suggested weekly cadence

| Week | Focus |
|------|--------|
| 1 | Phase 0–1 |
| 2 | Phase 2a–2b |
| 3 | Phase 3 (+ early quota/limits if sharing cluster) |
| 4 | Phase 4 (incl. image build pipeline) |
| 5 | Phase 5 |
| 6+ | Phase 6–8 as energy allows |

Adjust freely; prefer a **working Phase 2** over a half-built Phase 6.
