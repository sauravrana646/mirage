# Mirage

**Ephemeral PR preview environments for Kubernetes.**

Mirage is a Kubernetes operator that spins up short-lived preview environments for pull requests and tears them down when the PR closes or a TTL expires — like a mirage: visible while you need it, gone when you don’t.

> Status: **MVP implemented** (Phases 1–3). See [`docs/`](./docs/).

## Quick start (kind)

```bash
# Prerequisites: Go 1.23+, Docker, kubectl, kind, make

kind create cluster --name mirage

# Install CRDs and run the manager locally against the cluster
make install
make run

# In another terminal — create a sample preview
kubectl apply -f config/samples/mirage_v1alpha1_previewenvironment.yaml
kubectl get previewenvironments
kubectl get all -n preview-sample

# Cleanup
kubectl delete -f config/samples/mirage_v1alpha1_previewenvironment.yaml
```

### Dev loop

| Command | Purpose |
|---------|---------|
| `make generate manifests` | Codegen + CRD/RBAC |
| `make test` | envtest unit tests |
| `make install` | Install CRDs into current kube-context |
| `make run` | Run controller locally |
| `make docker-build IMG=mirage:dev` | Build manager image |
| `make deploy IMG=mirage:dev` | Deploy into cluster |

CI least-privilege RBAC sample: [`config/rbac/ci_role.yaml`](./config/rbac/ci_role.yaml)  
Example GitHub Action sketch: [`config/samples/github-actions-preview.yaml`](./config/samples/github-actions-preview.yaml)

## Why

Teams want per-PR previews without hand-maintained ApplicationSets, leftover namespaces, or tribal cleanup scripts. Mirage makes that a declared Kubernetes resource with a clear lifecycle.

## Docs

| Doc | Purpose |
|-----|---------|
| [PRD](./docs/prd.md) | Problem, goals, users, requirements |
| [Plan](./docs/plan.md) | Phase-wise build plan |
| [Architecture](./docs/architecture.md) | System design and components |
| [CRD design](./docs/crd.md) | API for `PreviewEnvironment` |
| [Security](./docs/security.md) | Threats, trust boundaries, baselines |
| [Decisions](./docs/decisions.md) | Design decision log (append-only) |
| [AI roadmap](./docs/ai-roadmap.md) | How AI plugs in later (not in reconcile) |
| [Prior art](./docs/prior-art.md) | Related tools and how Mirage differs |
| [kind Ingress](./docs/kind-ingress.md) | nginx + nip.io demo setup |

## Stack

- **Go** + **Kubebuilder** / `controller-runtime`
- **kind** for local clusters
- **GitHub** Actions for image build + CR apply (phase 4)
- **nginx ingress** + nip.io for preview URLs on kind (phase 3)
- **Argo CD** optional backend (phase 6)
- **AI** as a side path (phase 7+) — never in the hot reconcile loop

## Locked design defaults

See [`docs/decisions.md`](./docs/decisions.md). Short version: namespaced `PreviewEnvironment` (`mirage.dev/v1alpha1`), finalizer-managed `targetNamespace`, `ttlSeconds` only (expiry deletes the CR).

## License

[Apache License 2.0](./LICENSE)
