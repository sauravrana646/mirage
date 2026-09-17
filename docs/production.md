# Production guide

Hardening Mirage for shared clusters. Pair this with [security.md](./security.md) and `charts/mirage/values-production.yaml`.

## Install with production values

```bash
# CRDs first (and on every upgrade — Helm does not upgrade crds/)
kubectl apply -f config/crd/bases/

helm upgrade --install mirage charts/mirage \
  --namespace mirage-system --create-namespace \
  -f charts/mirage/values.yaml \
  -f charts/mirage/values-production.yaml \
  --set image.repository=ghcr.io/sauravrana646/mirage \
  --set image.digest=sha256:<digest> \
  --set policy.allowedRegistries=ghcr.io/your-org/ \
  --set policy.ingressHostSuffix=.preview.example.com
```

`values-production.yaml` turns on HA defaults, Fail-closed webhook + cert-manager, digest/TTL policies, PDB, ServiceMonitor, and metrics NetworkPolicy. Override `policy.*` per environment — empty allowlists disable those checks.

## High availability

| Setting | Production intent |
|---------|-------------------|
| `replicaCount: 2` | Multiple managers with leader election |
| `podDisruptionBudget` | Keep at least one manager during drains |
| `topologySpreadConstraints` | Spread replicas across nodes |

Do not run a single replica behind a Fail webhook: a down manager blocks PreviewEnvironment admission.

## Validating webhook

Keep `webhook.enabled=true` and `webhook.failurePolicy: Fail` in production. Requires cert-manager (`webhook.certManager.enabled=true`).

Policies commonly set via Helm `policy.*` / manager env:

| Policy | Purpose |
|--------|---------|
| `requireDigest` | Reject mutable tags (`:latest`, etc.) |
| `allowedRegistries` | Comma-separated image prefix allowlist |
| `ingressHostSuffix` | Require Ingress hosts end with this suffix (e.g. `.preview.example.com`) |
| `defaultTTLSeconds` / `maxTTLSeconds` | Default and cap for `spec.ttlSeconds` |

## RBAC

- **Manager** — ClusterRole creates namespaces and workloads in dynamic preview namespaces. On shared clusters, add admission policy (Kyverno/OPA) so only Mirage writes into namespaces labeled `app.kubernetes.io/managed-by=mirage`.
- **CI** — use `config/rbac/ci_role.yaml` (`mirage-ci` SA): mutate `PreviewEnvironment` only, never cluster-admin. Prefer short-lived OIDC over long-lived kubeconfig secrets. See [scm-integration.md](./scm-integration.md).
- **Argo CD** — enable `argoCD.rbac` only when using `backend: argocd`; lock destinations with Argo projects. Controller forces `destinationNamespace` to `targetNamespace`.

## Preview isolation (quotas, PSA, NetworkPolicy)

Platform defaults belong in `PreviewTemplate`; per-PR variance in `PreviewEnvironment`.

| Control | Baseline |
|---------|----------|
| PSA | `restricted` on preview namespaces (`spec.security.profile` / template) |
| Quotas | ResourceQuota + LimitRange per preview namespace |
| NetworkPolicy | `baseline` (app-port ingress only from namespaces labeled `mirage.dev/ingress-access=true`; DNS egress). Use `permissive` / `disabled` only when you accept the trade-off |
| Workload | Restricted securityContext, no hostPath, `automountServiceAccountToken: false` |
| Replicas | Capped (operator enforces an upper bound) |

Label ingress controller / mesh namespaces that must reach previews:

```bash
kubectl label ns ingress-nginx mirage.dev/ingress-access=true
```

## Ingress suffix and TLS

Set `policy.ingressHostSuffix` so CI cannot publish arbitrary hosts. Terminate TLS via Ingress annotations / `spec.ingress.tls` (cert-manager ClusterIssuer is a common pattern — see `config/samples/previewenvironment_production.yaml`).

## Observability

Production values enable metrics and (when Prometheus Operator is present) ServiceMonitor. Metrics NetworkPolicy restricts scrape to namespaces labeled `metrics=enabled`.

## Checklist

- [ ] CRDs applied explicitly before/with Helm install
- [ ] Image pinned by digest
- [ ] Webhook Fail + cert-manager healthy
- [ ] `allowedRegistries` and `ingressHostSuffix` set for your org
- [ ] TTL default/max configured
- [ ] CI uses least-privilege `mirage-ci` Role
- [ ] Ingress namespaces labeled for baseline NetworkPolicy
- [ ] PDB / replicaCount ≥ 2
