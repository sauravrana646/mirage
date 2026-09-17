# Mirage

**Ephemeral PR preview environments for Kubernetes — production-ready operator.**

Mirage creates short-lived preview environments for pull requests and tears them down when the PR closes or a TTL expires.

## Features

| Area | Capability |
|------|------------|
| Core | Namespaced `PreviewEnvironment` CRD, finalizer cleanup, immutable `targetNamespace` |
| Workloads | Deployment + Service, probes, imagePullSecrets, workload labels/annotations |
| Isolation | PSA `restricted`, ResourceQuota/LimitRange, NetworkPolicy (`baseline`/`permissive`/`disabled`) |
| Lifecycle | TTL expiry, suspend/resume, Kubernetes Events, Prometheus metrics |
| Access | Optional Ingress (path, annotations, TLS), nginx+nip.io docs |
| Scheduling | nodeSelector, tolerations, affinity, priorityClass, grace period |
| GitOps | `backend: argocd` creates Argo CD Applications |
| CI | GitHub / GitLab / Bitbucket: build→digest→upsert CR→**commit status + PR/MR comment** via `mirage-notify`; OIDC/SSO token auth supported |
| Admission | Validating webhook (digest, registry allowlist, ingress host suffix, Argo fields, max TTL) |
| AI | Side-path failure summaries (`ai/`) — never in reconcile |
| Package | Helm chart (`charts/mirage`), HA replicas, PDB, topology spread, ServiceMonitor, webhook TLS, `values-production.yaml` |

## Install (Helm)

```bash
# CRDs (also shipped under charts/mirage/crds/)
kubectl apply -f config/crd/bases/

helm upgrade --install mirage charts/mirage \
  --namespace mirage-system --create-namespace \
  --set image.repository=ghcr.io/sauravrana646/mirage \
  --set image.tag=0.1.0 \
  --set replicaCount=2 \
  --set webhook.enabled=true
```

See [`charts/mirage/README.md`](./charts/mirage/README.md). For shared clusters, also apply `charts/mirage/values-production.yaml` and set `policy.allowedRegistries` / `policy.ingressHostSuffix`.

## Local dev loop

```bash
kind create cluster --name mirage   # or k3s
make install
make run

kubectl apply -f config/samples/mirage_v1alpha1_previewenvironment.yaml
kubectl get previewenvironments
# short names: kubectl get pe
```

| Command | Purpose |
|---------|---------|
| `make generate manifests` | Codegen + CRD/RBAC/webhook |
| `make test` | envtest + webhook unit tests |
| `make lint` | golangci-lint |
| `make docker-build IMG=...` | Manager image |
| `make deploy IMG=...` | Kustomize deploy |

## Docs

| Doc | Purpose |
|-----|---------|
| [PRD](./docs/prd.md) | Product requirements |
| [Plan](./docs/plan.md) | Build plan |
| [Architecture](./docs/architecture.md) | System design |
| [CRD](./docs/crd.md) | API reference |
| [Security](./docs/security.md) | Threat model & baselines |
| [Decisions](./docs/decisions.md) | ADR log |
| [kind Ingress](./docs/kind-ingress.md) | nginx + nip.io |
| [SCM integration](./docs/scm-integration.md) | GitHub / GitLab / Bitbucket status + comments + OIDC |
| [Prior art](./docs/prior-art.md) | Alternatives |
| [AI path](./ai/README.md) | Advisory side path |

## Production sample

See [`config/samples/previewenvironment_production.yaml`](./config/samples/previewenvironment_production.yaml).

## License

[Apache License 2.0](./LICENSE)
