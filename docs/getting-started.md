# Getting started

Quick path from zero to a working preview environment. For platform design, see [preview-platform.md](./preview-platform.md). For production hardening, see [production.md](./production.md).

## Prerequisites

- Kubernetes 1.25+ (kind is fine for local demos)
- `kubectl`, Helm 3.8+
- Optional: [cert-manager](https://cert-manager.io/) if you enable the validating webhook (default in the Helm chart)

## 1. Install CRDs

Helm installs CRDs from `charts/mirage/crds/` on first install only. Prefer an explicit apply so upgrades stay predictable (see [upgrade.md](./upgrade.md)):

```bash
kubectl apply -f config/crd/bases/
# or from a checkout:
make install
```

## 2. Install the operator (Helm)

```bash
helm upgrade --install mirage charts/mirage \
  --namespace mirage-system --create-namespace \
  --set image.repository=ghcr.io/sauravrana646/mirage \
  --set image.tag=0.1.0
```

Local develop without Helm:

```bash
kind create cluster --name mirage
make install
make run   # manager against your kubeconfig
```

Verify:

```bash
kubectl get pods -n mirage-system
kubectl get crd previewenvironments.mirage.dev previewtemplates.mirage.dev
```

## 3. Apply a sample

**Option A — template + PreviewEnvironment (recommended):**

```bash
kubectl apply -f config/samples/previewtemplate_standard.yaml
# or: kubectl apply -f examples/simple/previewtemplate_standard.yaml
```

That sample creates `PreviewTemplate/standard-web-app` and `PreviewEnvironment/pr-142`. Explicit PE fields always override the template.

**Option B — single CR (no template):**

```bash
kubectl apply -f config/samples/mirage_v1alpha1_previewenvironment.yaml
```

Multi-service and Argo CD variants live under [`examples/`](../examples/) and `config/samples/`.

Watch status:

```bash
kubectl get previewenvironments -A
kubectl describe previewenvironment pr-142
```

Conditions of interest: `NamespaceReady`, `WorkloadReady`, `NetworkReady`, `RouteReady`.

## 4. mirage CLI basics

```bash
make build-cli
./bin/mirage list
./bin/mirage describe pr-142
./bin/mirage url pr-142
./bin/mirage logs pr-142 [-f] [--service frontend]
./bin/mirage diagnose pr-142
```

Create without hand-writing YAML:

```bash
./bin/mirage create --pr 142 \
  --image ghcr.io/org/app@sha256:... \
  --template standard-web-app
```

Suspend / resume / delete:

```bash
./bin/mirage suspend pr-142
./bin/mirage resume pr-142
./bin/mirage delete pr-142
```

Full command reference: [cli.md](./cli.md).

## 5. Optional: Ingress on kind

For a reachable URL on kind (nginx + nip.io), follow [kind-ingress.md](./kind-ingress.md). Label the ingress controller namespace so baseline NetworkPolicy allows traffic:

```bash
kubectl label ns ingress-nginx mirage.dev/ingress-access=true
```

## Next steps

| Goal | Doc / path |
|------|------------|
| Templates & multi-service | [preview-platform.md](./preview-platform.md) |
| Shared-cluster hardening | [production.md](./production.md) |
| CI → PreviewEnvironment | [examples/github](../examples/github/), [scm-integration.md](./scm-integration.md) |
| Failures & `diagnose` | [troubleshooting.md](./troubleshooting.md) |
| Chart upgrades | [upgrade.md](./upgrade.md) |
