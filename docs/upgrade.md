# Upgrading Mirage

## CRDs before Helm

Always apply CRDs **before** (or as a deliberate step ahead of) upgrading the chart. Helm 3 copies files from a chart’s `crds/` directory on **first install only**. It does **not** upgrade, patch, or delete those CRDs on later `helm upgrade` / `helm uninstall`.

```bash
# From a Mirage checkout — preferred
kubectl apply -f config/crd/bases/
# equivalent:
make install

# Or apply the copies bundled in the chart
kubectl apply -f charts/mirage/crds/

# Then upgrade the operator
helm upgrade mirage charts/mirage \
  --namespace mirage-system \
  -f charts/mirage/values.yaml \
  -f charts/mirage/values-production.yaml \
  --set image.digest=sha256:<new-digest>
```

Skipping the CRD apply leaves the apiserver on old schemas while the new manager expects new fields — admission and reconcile can fail in confusing ways.

## Recommended order

1. Read release notes / changelog for CRD or webhook changes.
2. `kubectl apply` CRDs from the target version.
3. `helm upgrade` with the matching chart and image digest.
4. Verify:

```bash
kubectl get crd previewenvironments.mirage.dev previewtemplates.mirage.dev -o yaml | head
kubectl get pods -n mirage-system
kubectl get validatingwebhookconfiguration
kubectl get previewenvironments -A
```

## Webhook and cert-manager

If the chart enables the validating webhook, ensure cert-manager remains healthy across the upgrade so the Certificate/secret serving the webhook stays valid. A Fail-closed webhook with a broken cert blocks PreviewEnvironment create/update until fixed (see [troubleshooting.md](./troubleshooting.md)).

## Rollback

```bash
helm rollback mirage -n mirage-system
```

Rolling back the release does **not** roll back CRDs. If you must revert API fields, re-apply the older CRD manifests intentionally and accept that Kubernetes CRD downgrades can be lossy — prefer forward fixes when possible.

## Uninstall notes

```bash
helm uninstall mirage -n mirage-system
# CRDs are retained by Helm on purpose
kubectl delete crd previewenvironments.mirage.dev previewtemplates.mirage.dev
```

Delete CRDs only when you intend to remove all PreviewEnvironment / PreviewTemplate objects cluster-wide.
