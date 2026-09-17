# Troubleshooting

Start with the CLI when a preview is stuck or unhealthy:

```bash
make build-cli
./bin/mirage diagnose <name> [-n <namespace>]
./bin/mirage describe <name>
./bin/mirage logs <name> [--service <svc>]
```

`diagnose` prints condition checks (`NamespaceReady`, `WorkloadReady`, `NetworkReady`, `RouteReady`), recent warning events, and hints for ImagePull / CrashLoop. Optional `--ai` points at the advisory side path under `ai/` (does not call an LLM from the CLI). See [cli.md](./cli.md).

## ImagePullBackOff / ErrImagePull

**Symptoms:** `WorkloadReady=False`; pods show `ImagePullBackOff`.

**Checks:**

1. Image reference — prefer digests (`@sha256:...`). With `policy.requireDigest` or `spec.requireDigest`, tags are rejected at admission.
2. Registry allowlist — image must match `policy.allowedRegistries` prefixes.
3. Pull secrets — preview namespaces do not inherit cluster pull credentials unless you add them (imagePullSecrets / projected secrets via your CI workflow).
4. Private GHCR/GCR — ensure the node/kubelet identity or a Secret in `targetNamespace` can pull.

```bash
kubectl -n <targetNamespace> get pods
kubectl -n <targetNamespace> describe pod <pod>
./bin/mirage diagnose <name>
```

## Namespace conflict

**Symptoms:** `Ready=False`, reason `NamespaceConflict` (or similar); reconcile refuses to proceed.

**Cause:** `spec.targetNamespace` already exists and is **not** owned by this PreviewEnvironment (missing Mirage ownership labels). The operator will not hijack foreign namespaces. `targetNamespace` is immutable after create.

**Fix:** Pick a free namespace name, or delete the leftover namespace only if you are sure it is safe. Do not reuse namespaces owned by another CR.

## Webhook / certificate failures

**Symptoms:** `kubectl apply` of PreviewEnvironment fails with webhook timeout, x509 errors, or connection refused to the webhook service.

**Checks:**

```bash
kubectl get validatingwebhookconfiguration
kubectl get pods -n mirage-system
kubectl get certificate,secret -n mirage-system   # cert-manager path
kubectl describe certificate -n mirage-system
```

Common causes:

| Cause | Mitigation |
|-------|------------|
| cert-manager missing | Install cert-manager before enabling webhook |
| Certificate not Ready | Inspect Certificate/Challenge; fix DNS/issuer |
| Manager down + `failurePolicy: Fail` | Restore replicas; temporary disable only in non-prod (`webhook.enabled=false`) |
| NetworkPolicy blocking webhook | Allow API server → webhook port (see chart / `config/network-policy`) |

Production should keep Fail-closed webhooks; do not leave `failurePolicy: Ignore` as a permanent workaround.

## TTL expiry

**Symptoms:** Preview disappears; phase/condition `Expired`; CR deleted after TTL.

**Cause:** `spec.ttlSeconds` (or template / `policy.defaultTTLSeconds`) elapsed. Webhook `maxTTLSeconds` rejects overly long TTLs at admit time.

**Fix:** Recreate the PreviewEnvironment; raise TTL within policy max if platform allows. TTL deletion is intentional lifecycle, not a bug.

## Suspend / resume

**Symptoms:** Workloads scaled down or not progressing; `spec.suspend: true`.

```bash
./bin/mirage describe <name>    # look for suspend
./bin/mirage resume <name>
# or
./bin/mirage suspend <name>
```

Suspended previews are paused by design — resume when you need the environment again. TTL may still apply depending on controller behavior; check `describe` / events if the CR vanishes while suspended.

## Ingress / RouteReady

**Symptoms:** `RouteReady=False` or URL unreachable.

- Host must satisfy `policy.ingressHostSuffix` when set.
- Ingress class installed and matching `ingressClassName`.
- Baseline NetworkPolicy: label ingress controller namespace `mirage.dev/ingress-access=true` (see [kind-ingress.md](./kind-ingress.md)).
- DNS / nip.io / corporate DNS must resolve the host.

## NetworkPolicy blocking traffic

**Symptoms:** Pods Ready but HTTP from ingress or other namespaces fails.

- Default `baseline` only allows ingress from namespaces with `mirage.dev/ingress-access=true`.
- For local demos only, `networkPolicy: permissive` or `disabled` relaxes this (not for shared production).

## Useful kubectl

```bash
kubectl get previewenvironments,previewtemplates -A
kubectl describe previewenvironment <name> -n <ns>
kubectl get events -n <targetNamespace> --sort-by='.lastTimestamp'
kubectl get deploy,svc,ingress,networkpolicy -n <targetNamespace>
```
