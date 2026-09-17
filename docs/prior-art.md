# Prior art / alternatives

Mirage sits in a crowded space. This doc records what already exists and why Mirage still exists as a **teachable, ownable** controller rather than a net-new product claim.

## Closest Kubernetes-native options

| Tool | Notes |
|------|--------|
| [Argo CD ApplicationSet Pull Request Generator](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Pull-Request/) | Default for GitOps shops. Discovers PRs and creates/deletes Applications. Mirage Phase 6 is an optional Argo backend; early phases learn direct reconcile. |
| [review-app-operator](https://github.com/wille/review-app-operator) | Go operator + GitHub Action; idle downscale and traffic forwarder. More features than Mirage MVP. |
| [eph](https://github.com/ephlabs/eph) | Ephemeral env controller with multi-provider ambitions. |
| mirrord preview environments | Request-level isolation on shared staging — different model (not Namespace-per-PR). |
| Platform SaaS (Vercel, Netlify, GitLab Review Apps, Heroku Review Apps) | Same UX for many apps; not a CRD you own in-cluster. |

## How Mirage differentiates (intentionally small)

- Small **`PreviewEnvironment` CRD** with clear status/conditions
- **Direct apply first**, Argo optional later
- **TTL + finalizer** cleanup as first-class
- **AI never inside reconcile** (advisory side path only)
- Built to learn **Go + controller-runtime** and map skills to CNCF contributions

If you already run Argo CD and only need PR previews, prefer ApplicationSet. Use Mirage when you want a minimal operator you understand end-to-end.
