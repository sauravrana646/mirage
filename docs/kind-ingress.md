# kind + Ingress (Phase 3 demos)

Mirage’s documented Ingress path on kind is **nginx ingress controller** + **nip.io** hosts.

## Install nginx ingress on kind

```bash
kind create cluster --name mirage
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.12.0/deploy/static/provider/kind/deploy.yaml
kubectl -n ingress-nginx wait --for=condition=Available deploy/ingress-nginx-controller --timeout=180s
```

## Sample with Ingress

Edit `config/samples/mirage_v1alpha1_previewenvironment.yaml`:

```yaml
spec:
  ingress:
    enabled: true
    host: preview-sample.127.0.0.1.nip.io
    ingressClassName: nginx
```

Then:

```bash
make install
make run   # or make deploy IMG=...
kubectl apply -f config/samples/mirage_v1alpha1_previewenvironment.yaml
kubectl get previewenvironments -o wide
# status.url should be http://preview-sample.127.0.0.1.nip.io
```

## Notes

- Other ingress controllers / Gateway API are out of scope until this path works.
- Preview workloads use restricted container securityContext; prefer unprivileged images (sample uses `nginxinc/nginx-unprivileged`).
