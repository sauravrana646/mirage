# Security notes

**Status:** Draft baseline — harden before any shared cluster use.

## Threats (initial)

| Threat | Mitigation direction |
|--------|----------------------|
| Preview runs untrusted PR code | Isolate namespace; no cluster-admin to workloads; NetworkPolicy; no hostPath |
| CI identity too powerful | RBAC limited to PreviewEnvironment CRs in `mirage-system` (+ necessary reads); ship sample Role in Phase 4 |
| Leftover envs expose old builds | Finalizers + TTL (CR deleted on expiry); metrics for orphan namespaces |
| Ingress exposes internal apps | Auth optional later; private DNS / allowlists; no prod secrets in preview |
| Namespace hijack | Refuse reconcile if `targetNamespace` exists without Mirage ownership labels |
| Supply chain (operator image) | Pin digests; minimal base image; CI builds signed (later) |
| AI side path leaks logs/secrets | Redact before LLM; never send Secret data |

## Trust boundaries

1. **Manager** — can create namespaces and workloads for previews; keep ClusterRole as tight as practical.
2. **CI bot** — can mutate Mirage CRs only (not arbitrary Deployments cluster-wide).
3. **PR workload** — untrusted; treat as hostile.
4. **AI service** — untrusted with data; read-only to cluster where possible.

## Baselines by phase

| Phase | Baseline |
|-------|----------|
| 2 | Finalizer cleanup; ownership labels; NamespaceConflict; immutable `targetNamespace`; default ResourceQuota + LimitRange; baseline NetworkPolicy (DNS egress + app ingress); PSA `restricted` labels; restricted pod securityContext; SA token not automounted |
| 3 | Ingress docs (nginx + nip.io); prefer unprivileged images; ingress off by default in samples |
| 4 | Least-privilege CI Role sample; document shared-SA risks + prefer OIDC |
| 5 | Helm chart; optional Kyverno/OPA to constrain manager writes; image digest/registry allowlists |
| 7 | Redaction policy for AI |

## Manager RBAC (known trade-off)

The manager ClusterRole can create Namespaces and Deployments/Services/Ingresses cluster-wide because preview namespaces are dynamic. On a shared cluster, pair Mirage with admission policy (Kyverno/OPA) that only allows Mirage writes into namespaces labeled `app.kubernetes.io/managed-by=mirage`. Do not treat the manager SA as a break-glass admin credential.

## Explicit non-goals early

- Full Pod Security Admission policy pack (add when demos leave a personal kind cluster)
- Multi-tenant SaaS isolation guarantees
