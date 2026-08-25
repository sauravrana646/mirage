# Mirage

**Ephemeral PR preview environments for Kubernetes.**

Mirage is a Kubernetes operator that spins up short-lived preview environments for pull requests and tears them down when the PR closes or a TTL expires — like a mirage: visible while you need it, gone when you don’t.

> Status: design / early planning. See [`docs/`](./docs/).

## Why

Teams want per-PR previews without hand-maintained ApplicationSets, leftover namespaces, or tribal cleanup scripts. Mirage makes that a declared Kubernetes resource with a clear lifecycle.

## Docs

| Doc | Purpose |
|-----|---------|
| [PRD](./docs/prd.md) | Problem, goals, users, requirements |
| [Plan](./docs/plan.md) | Phase-wise build plan you can follow |
| [Architecture](./docs/architecture.md) | System design and components |
| [CRD design](./docs/crd.md) | API sketch for `PreviewEnvironment` |
| [Security](./docs/security.md) | Threats, trust boundaries, baselines |
| [Decisions](./docs/decisions.md) | Design decision log (append-only) |
| [AI roadmap](./docs/ai-roadmap.md) | How AI plugs in later (not in reconcile) |

## Stack (target)

- **Go** + **Kubebuilder** / `controller-runtime`
- **kind** for local clusters
- **GitHub** Actions for image build + CR apply (phase 4)
- **nginx ingress** + nip.io for preview URLs on kind (phase 3)
- **Argo CD** optional backend (phase 6)
- **AI** as a side path (phase 7+) — never in the hot reconcile loop

## Locked design defaults

See [`docs/decisions.md`](./docs/decisions.md). Short version: namespaced `PreviewEnvironment`, finalizer-managed `targetNamespace`, `ttlSeconds` only (expiry deletes the CR).

## Quick links

- Follow the build order in [`docs/plan.md`](./docs/plan.md)
- Record every non-trivial choice in [`docs/decisions.md`](./docs/decisions.md)

## License

[Apache License 2.0](./LICENSE)
