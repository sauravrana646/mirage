# CRD design — PreviewEnvironment & PreviewTemplate

**API group:** `mirage.dev`

**Kinds:** `PreviewEnvironment` (short names `pe`, `preview`), `PreviewTemplate` (`pt`, `previewtemplate`)

**Scope:** Namespaced

**Typical CR namespace:** `mirage-system` (management); workloads run in `spec.targetNamespace`.

See also [preview-platform.md](./preview-platform.md) and [cli.md](./cli.md).

## PreviewTemplate

```yaml
apiVersion: mirage.dev/v1alpha1
kind: PreviewTemplate
metadata:
  name: standard-web-app
spec:
  resources:
    preset: small                 # small | medium | large
  networkPolicy: baseline
  ingress:
    enabled: true
    domain: preview.example.com   # host = <pe-name>.<domain> when unset
  ttlSeconds: 86400
  security:
    profile: restricted           # restricted | baseline
    # runtimeClassName: gvisor
  replicas: 1
  containerPort: 8080
  backend: direct
```

## PreviewEnvironment Spec (v1alpha1)

```yaml
apiVersion: mirage.dev/v1alpha1
kind: PreviewEnvironment
metadata:
  name: pr-42-myapp
  namespace: mirage-system
spec:
  templateRef:
    name: standard-web-app        # optional platform defaults

  source:
    provider: github              # github | gitlab | bitbucket
    repo: https://github.com/org/app
    pullRequest: 42
    commitSHA: abcdef0123
    branch: feature/x

  # Single-service (legacy / simple):
  image: ghcr.io/org/app@sha256:...
  # OR multi-service:
  # services:
  #   - name: api
  #     image: ghcr.io/org/api@sha256:...
  #     port: 8080
  #   - name: frontend
  #     image: ghcr.io/org/web@sha256:...
  #     port: 3000
  #     ingress:
  #       enabled: true
  #       host: pr-42.preview.example.com

  requireDigest: true
  ttlSeconds: 86400
  targetNamespace: preview-pr-42  # immutable after create
  backend: direct                 # direct | argocd
  suspend: false
  networkPolicy: baseline
  ingress:
    enabled: true
    host: pr-42.preview.example.com
  runtimeClassName: ""            # optional override of template security.runtimeClassName
```

`spec.image` **or** `spec.services` is required (CEL). Explicit PE fields override the template.

## Status

```yaml
status:
  phase: Provisioning | Ready | Failed | Paused | Expiring
  url: https://pr-42.preview.example.com
  expiresAt: "2026-08-26T12:00:00Z"
  observedGeneration: 1
  message: All preview services available
  template: standard-web-app
  replicaStatus: "api=1/1,frontend=1/1"
  argoApplication: argocd/pr-42-myapp   # when backend=argocd
  services:
    - name: api
      ready: true
      replicas: 1/1
    - name: frontend
      ready: true
      url: https://...
  conditions:
    - type: NamespaceReady
      status: "True"
    - type: WorkloadReady
      status: "True"
      reason: WorkloadReady
    - type: NetworkReady
      status: "True"
    - type: RouteReady
      status: "True"
    - type: Ready
      status: "True"
    - type: Progressing
      status: "False"
```

### Condition reasons

| Reason | When |
|--------|------|
| `WorkloadReady` | Deployments available |
| `NamespaceConflict` | `targetNamespace` exists but is not owned by this CR |
| `ImageInvalid` | Missing image/services |
| `TemplateNotFound` | `templateRef` could not be resolved |
| `InvalidSpec` | Backend/Argo/validation failure |
| `RolloutFailed` | Progress deadline / ImagePullBackOff |
| `Expiring` | TTL passed; cleanup in progress |
| `Paused` | `spec.suspend=true` |
| `ArgoSyncing` / `ArgoHealthy` | Argo CD backend |

## Ownership model

- CR is Namespaced; it **cannot** owner-reference the Namespace (cluster-scoped) or cross-namespace children.
- Controller **finalizer** creates/deletes `spec.targetNamespace`.
- Children are labeled (`mirage.dev/owner-uid`, `mirage.dev/owner-name`, `mirage.dev/owner-namespace`, `app.kubernetes.io/managed-by=mirage`, `mirage.dev/service`).
- Controller watches Deployments/Services/Ingresses by those labels and requeues the owning CR.
- Prefer **one namespace per preview** for blast-radius isolation.

## Validation

- CRD markers + CEL: `image` or `services`, immutable `targetNamespace`, enum backends/networkPolicy.
- Validating webhook: digest policy, registry allowlist, ingress host suffix, Argo fields, max TTL, per-service images.

## Samples

- [`config/samples/mirage_v1alpha1_previewenvironment.yaml`](../config/samples/mirage_v1alpha1_previewenvironment.yaml)
- [`config/samples/previewtemplate_standard.yaml`](../config/samples/previewtemplate_standard.yaml)
- [`config/samples/previewenvironment_multiservice.yaml`](../config/samples/previewenvironment_multiservice.yaml)
- [`config/samples/previewenvironment_production.yaml`](../config/samples/previewenvironment_production.yaml)
- [`config/samples/previewenvironment_argocd.yaml`](../config/samples/previewenvironment_argocd.yaml)
