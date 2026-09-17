# Argo CD backend

Mirage owns preview lifecycle (`targetNamespace`, TTL, finalizers). Argo CD syncs a Git path into that namespace when `spec.backend: argocd`.

## Apply

Prerequisites: Argo CD installed; Helm `argoCD.rbac.enabled` (or equivalent ClusterRole) so the manager can create Applications. Destination namespace must equal `targetNamespace` (enforced by the controller).

```bash
kubectl apply -f previewenvironment_argocd.yaml
```

Edit `spec.argoCD` (`repoURL`, `path`, `targetRevision`, `argoNamespace`, `project`) for your app.

Canonical sample: [`config/samples/previewenvironment_argocd.yaml`](../../config/samples/previewenvironment_argocd.yaml). Security notes: [security.md](../../docs/security.md).
