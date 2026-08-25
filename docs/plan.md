# Mirage — Phase-wise build plan

Follow phases in order. Do not skip the “done when” checks. After each phase, append any design choices to [decisions.md](./decisions.md).

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
1. `kubebuilder init` / `create api` for group `mirage.dev`, kind `PreviewEnvironment`.
2. Generate manifests; install CRD + manager on kind.
3. Add Makefile targets: `make generate`, `manifests`, `install`, `run` / `deploy`.
4. Commit scaffold; update README with “dev loop”.

### Learn
- Kubebuilder layout, code generation, manager main  

### Done when
- [ ] CRD installed; `kubectl get previewenvironments` works  
- [ ] Manager pod Running (or `make run` against kind)  

---

## Phase 2 — Core reconcile (P0)

**Goal:** Manual CR creates a working preview app and cleans up on delete.

### Steps
1. Finalize CRD fields (see [crd.md](./crd.md)); regenerate.
2. Reconcile:
   - Ensure Namespace (name from spec or derived from PR metadata)
   - Ensure Deployment + Service (image from spec)
   - Set ownerReferences from CR → child objects
3. Status conditions: `Ready`, `Failed` with Reason/Message.
4. Finalizer: on delete, remove owned namespace (or owned children).
5. Sample YAML under `config/samples/`.
6. Unit tests for key reconcile branches (`envtest`).

### Learn
- Idempotent create/update, finalizers, conditions, ownerRefs, requeue  

### Done when
- [ ] `kubectl apply -f config/samples/...` → pods Ready  
- [ ] `kubectl delete` CR → namespace/resources gone  
- [ ] Status shows Ready/Failed meaningfully  

---

## Phase 3 — URL + TTL (P1)

**Goal:** Previews are reachable and time-bounded.

### Steps
1. Add optional Ingress (or Gateway API later) + status URL.
2. Implement TTL: `spec.ttl` / `spec.expiresAt`; reconcile deletes or marks expired.
3. Kubernetes Events for create/ready/expire/fail.
4. Document kind ingress setup (e.g. nginx ingress controller).

### Learn
- Ingress, time-based reconcile (`RequeueAfter`), Events API  

### Done when
- [ ] Status includes a URL (even if local/nip.io style)  
- [ ] Expired preview is cleaned up without manual delete  

---

## Phase 4 — GitHub integration (P1)

**Goal:** PR open/sync/close drives CRs.

### Steps
1. Choose path A: GitHub Action applies CR YAML; or path B: small webhook receiver.
   - Prefer **Action first** (simpler; record decision).
2. On `opened`/`synchronize`: upsert `PreviewEnvironment` with image digest from CI build.
3. On `closed`: delete CR.
4. Comment preview URL on PR when Ready (Action or controller annotation hook).
5. Document required secrets/permissions.

### Learn
- CI → cluster auth (kubeconfig / OIDC later), GitHub Checks/comments  

### Done when
- [ ] Open PR → preview appears  
- [ ] Close PR → preview gone  
- [ ] Push to PR → preview updates (new image)  

---

## Phase 5 — Hardening (P1/P2)

**Goal:** Safer defaults for shared clusters.

### Steps
1. Default ResourceQuota + LimitRange per preview namespace.
2. Optional NetworkPolicy (deny egress except DNS/needed).
3. Metrics (controller-runtime metrics) + basic Grafana/Prometheus notes.
4. Helm chart for installing Mirage.
5. kind e2e smoke in CI (GitHub Actions).

### Learn
- Multi-tenant baselines, operator packaging, CI for controllers  

### Done when
- [ ] Chart installs Mirage cleanly  
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

**Goal:** Portfolio + CNCF contribution pipeline.

### Steps
1. Public demo GIF/docs; pin architecture diagram.
2. Write “what I learned” comparing Mirage to Argo ApplicationSet.
3. Pick CNCF issues (Argo CD / controller-runtime examples / Cluster API addons) that match skills from Phases 2–6.
4. Keep decisions.md honest — include mistakes.

### Done when
- [ ] Repo is demoable by a stranger from README alone  
- [ ] At least one upstream PR opened (even docs/tests)  

---

## Suggested weekly cadence

| Week | Focus |
|------|--------|
| 1 | Phase 0–1 |
| 2 | Phase 2 |
| 3 | Phase 3 |
| 4 | Phase 4 |
| 5 | Phase 5 |
| 6+ | Phase 6–8 as energy allows |

Adjust freely; prefer a **working Phase 2** over a half-built Phase 6.
