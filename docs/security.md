# Security notes

**Status:** Production baselines for shared clusters — use `charts/mirage/values-production.yaml`.

## Threats

| Threat | Mitigation |
|--------|------------|
| Preview runs untrusted PR code | Isolate namespace; NetworkPolicy; PSA restricted; no hostPath; SA token not automounted |
| CI identity too powerful | Least-privilege Role (`config/rbac/ci_role.yaml`); prefer OIDC; never cluster-admin kubeconfig |
| Leftover envs expose old builds | Finalizers + TTL (CR deleted on expiry); max TTL webhook policy |
| Ingress exposes internal apps | Host suffix allowlist; deny dangerous nginx snippet annotations; TLS optional |
| Namespace hijack | Refuse reconcile if `targetNamespace` exists without Mirage ownership labels; immutable field |
| Argo destination escape | `destinationNamespace` must equal `targetNamespace`; controller forces dest to target |
| Supply chain (operator image) | Pin digests; release workflow; distroless nonroot |
| AI side path leaks secrets | Default-branch scripts only; fork `workflow_run` skipped; redact env/tokens before LLM |

## Trust boundaries

1. **Manager** — creates namespaces and workloads for previews; keep ClusterRole as tight as practical.
2. **CI bot** — mutates Mirage CRs only (not arbitrary Deployments cluster-wide).
3. **PR workload** — untrusted; treat as hostile.
4. **AI service** — untrusted with data; scripts always from default branch when secrets are present.

## Baselines

| Area | Baseline |
|------|----------|
| Workloads | PSA `restricted`, ResourceQuota/LimitRange, NetworkPolicy (`baseline` ingress only from `mirage.dev/ingress-access=true` namespaces) |
| Admission | Webhook Fail; digest/registry/host/maxTTL policies via Helm `policy.*` |
| Packaging | HA replicas, PDB, topology spread; `values-production.yaml` |
| GitOps | Argo destination locked to `targetNamespace` |
| AI | No PR-head script execution with `KUBE_CONFIG` |

## Manager RBAC (known trade-off)

The manager ClusterRole can create Namespaces and Deployments/Services/Ingresses cluster-wide because preview namespaces are dynamic. On a shared cluster, pair Mirage with admission policy (Kyverno/OPA) that only allows Mirage writes into namespaces labeled `app.kubernetes.io/managed-by=mirage`.

## Explicit non-goals

- Multi-tenant SaaS isolation guarantees
- Replacing Argo CD project RBAC (still required when using `backend: argocd`)
