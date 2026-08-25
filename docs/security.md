# Security notes

**Status:** Draft baseline — harden before any shared cluster use.

## Threats (initial)

| Threat | Mitigation direction |
|--------|----------------------|
| Preview runs untrusted PR code | Isolate namespace; no cluster-admin to workloads; NetworkPolicy; no hostPath |
| CI identity too powerful | RBAC limited to PreviewEnvironment CRs + necessary reads |
| Leftover envs expose old builds | Finalizers + TTL; metrics for orphan namespaces |
| Ingress exposes internal apps | Auth optional later; private DNS / allowlists; no prod secrets in preview |
| Supply chain (operator image) | Pin digests; minimal base image; CI builds signed (later) |
| AI side path leaks logs/secrets | Redact before LLM; never send Secret data |

## Trust boundaries

1. **Manager** — privileged relative to preview namespaces; keep Role tight.
2. **CI bot** — can mutate Mirage CRs only.
3. **PR workload** — untrusted; treat as hostile.
4. **AI service** — untrusted with data; read-only to cluster where possible.

## Baselines by phase

| Phase | Baseline |
|-------|----------|
| 2 | OwnerRefs + finalizer cleanup |
| 5 | ResourceQuota, LimitRange, optional NetworkPolicy |
| 4+ | Document least privilege for GitHub token / kube credentials |
| 7 | Redaction policy for AI |

## Explicit non-goals early

- Full Pod Security Admission policy pack (add when demos leave a personal kind cluster)
- Multi-tenant SaaS isolation guarantees
