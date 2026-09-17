# Mirage

**Kubernetes-native preview environment platform.**

Mirage creates short-lived preview environments for pull requests and tears them down when the PR closes or a TTL expires. Platform defaults live in `PreviewTemplate`; per-PR variance lives in `PreviewEnvironment` (single- or multi-service).

## Features

| Area | Capability |
|------|------------|
| Platform | `PreviewTemplate` defaults (resources, network, ingress domain, TTL, PSA) |
| Core | Namespaced `PreviewEnvironment` CRD, finalizer cleanup, immutable `targetNamespace` |
| Workloads | Single-image or multi-service (`services[]`) Deployment + Service |
| Isolation | PSA `restricted`, ResourceQuota/LimitRange, NetworkPolicy (`baseline`/`permissive`/`disabled`) |
| Lifecycle | TTL expiry, suspend/resume, Kubernetes Events, Prometheus metrics |
| Status | `NamespaceReady` / `WorkloadReady` / `NetworkReady` / `RouteReady` conditions |
| Access | Optional Ingress (path, annotations, TLS), nginx+nip.io docs |
| Scheduling | nodeSelector, tolerations, affinity, priorityClass, RuntimeClass, grace period |
| GitOps | `backend: argocd` creates Argo CD Applications |
| CI | GitHub / GitLab / Bitbucket via `mirage-notify` (outside reconcile) |
| CLI | `mirage` list/describe/logs/url/suspend/resume/delete/diagnose/create |
| Admission | Validating webhook (digest, registry allowlist, ingress host suffix, Argo fields, max TTL) |
| AI | Side-path failure summaries (`ai/`) — never in reconcile |
| Package | Helm chart (`charts/mirage`), HA replicas, PDB, topology spread, ServiceMonitor |

## Install (Helm)

```bash
kubectl apply -f config/crd/bases/

helm upgrade --install mirage charts/mirage \
  --namespace mirage-system --create-namespace \
  --set image.repository=ghcr.io/sauravrana646/mirage \
  --set image.tag=0.1.0 \
  --set replicaCount=2 \
  --set webhook.enabled=true
```

See [`charts/mirage/README.md`](./charts/mirage/README.md).

## Local dev loop

```bash
kind create cluster --name mirage
make install
make run

kubectl apply -f config/samples/previewtemplate_standard.yaml
# multi-service:
kubectl apply -f config/samples/previewenvironment_multiservice.yaml

make build-cli
./bin/mirage list
./bin/mirage diagnose pr-142
```

| Command | Purpose |
|---------|---------|
| `make generate manifests` | Codegen + CRD/RBAC/webhook |
| `make test` | envtest + webhook unit tests |
| `make lint` | golangci-lint |
| `make build-cli` | Developer CLI (`bin/mirage`) |
| `make docker-build IMG=...` | Manager image |

## Docs

| Doc | Purpose |
|-----|---------|
| [Getting started](./docs/getting-started.md) | Install, sample apply, CLI basics |
| [Production](./docs/production.md) | HA, webhook, policies, RBAC, isolation |
| [Troubleshooting](./docs/troubleshooting.md) | Common failures + `mirage diagnose` |
| [Upgrade](./docs/upgrade.md) | CRD apply before Helm; CRD upgrade notes |
| [Preview platform](./docs/preview-platform.md) | Templates, multi-service, status model |
| [CLI](./docs/cli.md) | `mirage` command reference |
| [Examples](./examples/) | simple / github / multi-service / argocd |
| [PRD](./docs/prd.md) | Product requirements |
| [Plan](./docs/plan.md) | Build plan |
| [Architecture](./docs/architecture.md) | System design |
| [CRD](./docs/crd.md) | API reference |
| [Security](./docs/security.md) | Threat model & baselines |
| [Decisions](./docs/decisions.md) | ADR log |
| [kind Ingress](./docs/kind-ingress.md) | nginx + nip.io |
| [SCM integration](./docs/scm-integration.md) | GitHub / GitLab / Bitbucket + OIDC |
| [Prior art](./docs/prior-art.md) | Alternatives |
| [AI path](./ai/README.md) | Advisory side path |

## License

[Apache License 2.0](./LICENSE)
