# CRD design — `PreviewEnvironment`

**API group:** `mirage.dev`  
**Kind:** `PreviewEnvironment`  
**Scope:** **Namespaced**  
**Typical CR namespace:** `mirage-system` (management); workloads run in `spec.targetNamespace`.

Short names: `pe`, `preview`.

## Spec (v1alpha1)

```yaml
apiVersion: mirage.dev/v1alpha1
kind: PreviewEnvironment
metadata:
  name: pr-42-myapp
  namespace: mirage-system
spec:
  source:
    provider: github          # github | gitlab | bitbucket
    repo: https://github.com/org/app
    pullRequest: 42
    commitSHA: abcdef0123
    branch: feature/x
    # projectID: group/app    # GitLab
    # workspace: my-ws        # Bitbucket
    # repoSlug: app           # Bitbucket

  image: ghcr.io/org/app@sha256:...   # prefer digest
  requireDigest: true                 # also MIRAGE_REQUIRE_DIGEST
  imagePullPolicy: IfNotPresent
  command: []                         # optional entrypoint override
  args: []
  env: []
  envFrom: []
  resources: {}
  containerPort: 8080
  replicas: 1
  ttlSeconds: 86400                   # status.expiresAt computed; MIRAGE_DEFAULT_TTL_SECONDS / MIRAGE_MAX_TTL_SECONDS
  targetNamespace: preview-pr-42      # immutable after create
  backend: direct                     # direct | argocd
  suspend: false
  networkPolicy: baseline             # baseline | permissive | disabled
  serviceAccountName: ""
  imagePullSecrets: []
  nodeSelector: {}
  tolerations: []
  affinity: {}
  priorityClassName: ""
  terminationGracePeriodSeconds: 30
  workloadLabels: {}
  workloadAnnotations: {}
  readinessProbe:
    path: /healthz
    port: 8080
    initialDelaySeconds: 3
    periodSeconds: 5
  livenessProbe:
    path: /healthz
    port: 8080
  ingress:
    enabled: true
    host: pr-42.preview.example.com
    path: /
    ingressClassName: nginx
    annotations: {}
    tls:
      enabled: true
      secretName: pr-42-tls
  argoCD:                             # required when backend=argocd
    repoURL: https://github.com/org/app
    path: deploy/preview
    targetRevision: HEAD
    project: default
    argoNamespace: argocd
    destinationNamespace: preview-pr-42
```

## Status

```yaml
status:
  phase: Ready                 # Pending | Ready | Failed | Expiring | Paused
  url: https://pr-42.preview.example.com
  expiresAt: "2026-08-26T12:00:00Z"
  observedGeneration: 1
  message: Deployment available
  replicaStatus: "1/1"
  argoApplication: argocd/pr-42-myapp   # when backend=argocd
  conditions:
    - type: Ready
      status: "True"
      reason: WorkloadReady
    - type: Progressing
      status: "False"
```

### Condition reasons

| Reason | When |
|--------|------|
| `WorkloadReady` | Deployment available |
| `NamespaceConflict` | `targetNamespace` exists but is not owned by this CR |
| `ImageInvalid` | Missing/empty image |
| `InvalidSpec` | Backend/Argo/validation failure |
| `RolloutFailed` | Progress deadline / ImagePullBackOff |
| `Expiring` | TTL passed; cleanup in progress |
| `Paused` | `spec.suspend=true` |
| `ArgoSyncing` / `ArgoHealthy` | Argo CD backend |

## Ownership model

- CR is Namespaced; it **cannot** owner-reference the Namespace (cluster-scoped) or cross-namespace children.
- Controller **finalizer** creates/deletes `spec.targetNamespace`.
- Children are labeled (`mirage.dev/owner-uid`, `mirage.dev/owner-name`, `mirage.dev/owner-namespace`, `app.kubernetes.io/managed-by=mirage`).
- Controller watches Deployments/Services/Ingresses by those labels and requeues the owning CR.
- Prefer **one namespace per preview** for blast-radius isolation.

## Validation

- CRD markers + CEL: required `image`/`targetNamespace`, immutable `targetNamespace`, enum backends/networkPolicy.
- Validating webhook: digest policy, registry allowlist, ingress host suffix, Argo fields, max TTL.

## Samples

- [`config/samples/mirage_v1alpha1_previewenvironment.yaml`](../config/samples/mirage_v1alpha1_previewenvironment.yaml)
- [`config/samples/previewenvironment_production.yaml`](../config/samples/previewenvironment_production.yaml)
- [`config/samples/previewenvironment_argocd.yaml`](../config/samples/previewenvironment_argocd.yaml)
