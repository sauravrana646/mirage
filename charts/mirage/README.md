# Mirage Helm Chart

Production Helm chart for the [Mirage](https://github.com/sauravrana646/mirage) Kubernetes operator — ephemeral PR preview environments (`PreviewEnvironment`, `mirage.dev/v1alpha1`).

## Prerequisites

- Kubernetes 1.25+
- Helm 3.8+
- [cert-manager](https://cert-manager.io/) (required when `webhook.enabled=true`, the default)
- Optional: Prometheus Operator (for `metrics.serviceMonitor.enabled`)

## CRDs

CRDs live in **`crds/`** (`crds/previewenvironment.yaml`) and are installed automatically on `helm install`.

Helm 3 installs CRDs from `crds/` once and does **not** upgrade or delete them on chart upgrade/uninstall. To update CRDs after a release:

```bash
# From a Mirage checkout
make install
# or
kubectl apply -f config/crd/bases/
# or apply the chart copy
kubectl apply -f charts/mirage/crds/
```

If you prefer to manage CRDs out-of-band, install them first, then install the chart as usual.

## Install

```bash
# Create namespace and install (default release name: mirage)
helm upgrade --install mirage ./charts/mirage \
  --namespace mirage-system \
  --create-namespace \
  --set image.repository=ghcr.io/sauravrana646/mirage \
  --set image.tag=0.1.0
```

Pin by digest in production:

```bash
helm upgrade --install mirage ./charts/mirage \
  --namespace mirage-system \
  --create-namespace \
  --set image.repository=ghcr.io/sauravrana646/mirage \
  --set image.digest=sha256:<digest>
```

## Verify

```bash
kubectl get pods -n mirage-system
kubectl get crd previewenvironments.mirage.dev
kubectl get validatingwebhookconfiguration
```

## Configuration

| Key | Default | Description |
|-----|---------|-------------|
| `replicaCount` | `2` | Manager replicas (HA with leader election) |
| `image.repository` | `ghcr.io/sauravrana646/mirage` | Controller image |
| `image.tag` | `""` (chart `appVersion`) | Image tag |
| `image.digest` | `""` | Image digest (overrides tag when set) |
| `image.pullPolicy` | `IfNotPresent` | Pull policy |
| `controller.leaderElect` | `true` | `--leader-elect` |
| `webhook.enabled` | `true` | ValidatingWebhookConfiguration + webhook service |
| `webhook.certManager.enabled` | `true` | cert-manager Issuer + Certificate |
| `metrics.enabled` | `true` | Metrics Service on `:8443` |
| `metrics.serviceMonitor.enabled` | `false` | Prometheus Operator ServiceMonitor |
| `metrics.networkPolicy.enabled` | `false` | NetworkPolicy restricting metrics to `metrics=enabled` namespaces |
| `argoCD.rbac.enabled` | `true` | ClusterRole rules for `argoproj.io/applications` |
| `policy.requireDigest` | `false` | Reject images without `@sha256:` |
| `policy.allowedRegistries` | `""` | Comma-separated image prefix allowlist |
| `policy.ingressHostSuffix` | `""` | Required Ingress host suffix |
| `policy.defaultTTLSeconds` | `0` | Default TTL when spec omits `ttlSeconds` |
| `policy.maxTTLSeconds` | `0` | Max allowed `ttlSeconds` (0 = unlimited) |
| `podDisruptionBudget.enabled` | `true` | PDB for manager pods |
| `topologySpreadConstraints` | hostname skew | Spread HA replicas across nodes |
| `resources` | see `values.yaml` | Container resources |
| `nodeSelector` / `tolerations` / `affinity` | `{}` / `[]` / `{}` | Scheduling |
| `podSecurityContext` | restricted PSS | Pod security context |

See [`values.yaml`](./values.yaml) for the full set.

### Disable the webhook

```bash
helm upgrade --install mirage ./charts/mirage \
  --namespace mirage-system \
  --create-namespace \
  --set webhook.enabled=false
```

### Enable ServiceMonitor

```bash
helm upgrade --install mirage ./charts/mirage \
  --namespace mirage-system \
  --create-namespace \
  --set metrics.serviceMonitor.enabled=true
```

### Metrics NetworkPolicy

When enabled, only pods in namespaces labeled `metrics: enabled` can scrape port `8443`:

```bash
kubectl label namespace monitoring metrics=enabled
helm upgrade --install mirage ./charts/mirage \
  --namespace mirage-system \
  --create-namespace \
  --set metrics.networkPolicy.enabled=true
```

## What gets installed

- **Deployment** `*-controller-manager` — leader election, health probes on `:8081`, metrics on `:8443`
- **ServiceAccount** `controller-manager`
- **ClusterRole / Role** — `previewenvironments` (all), namespaces, deployments, services, ingresses, networkpolicies, resourcequotas, limitranges, events; optional Argo CD Applications
- **ValidatingWebhookConfiguration** (optional) — PreviewEnvironment `CREATE`/`UPDATE`
- **cert-manager Certificate** (optional, with webhook)
- **ServiceMonitor** / **NetworkPolicy** (optional, metrics)

## Uninstall

```bash
helm uninstall mirage -n mirage-system
# CRDs are retained by Helm; remove explicitly if desired:
kubectl delete crd previewenvironments.mirage.dev
```

## License

Apache License 2.0 — see the repository root.
