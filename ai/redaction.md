# Redaction rules for AI advisory

Evidence sent to an LLM (or posted on a PR) must be scrubbed first. `ai/summarize.sh` applies these rules; keep the script and this doc aligned.

## Never send

- Raw kubeconfig contents, bearer tokens, or SA tokens
- Values of Secrets (`kubectl get secret -o yaml` is forbidden in the gather path)
- Environment variable values that look like credentials (see patterns below)
- Private keys, certificates with private material, cloud access keys
- Full `.dockerconfigjson` / registry password fields

## Always redact (patterns)

| Pattern | Replacement |
|---------|-------------|
| `Bearer <token>` | `Bearer ***REDACTED***` |
| `token=…` / `password=…` / `api_key=…` / `secret=…` | value → `***REDACTED***` |
| GitHub PATs (`ghp_`, `github_pat_`, …) | prefix + `***REDACTED***` |
| JWT-shaped strings (`eyJ…eyJ…`) | `***JWT_REDACTED***` |
| Env-like keys containing `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `CREDENTIAL`, `PRIVATE` | value → `***REDACTED***` |
| AWS access key IDs (`AKIA…`) | `***AWS_KEY_REDACTED***` |

## Safe to include (after redaction)

- `PreviewEnvironment` status (phase, conditions, URL)
- Event reasons/messages (already redacted)
- Pod phase, restart counts, container wait reasons
- Tail of container logs with credential patterns stripped
- Image references by digest (not registry passwords)

## Operational rules

1. Prefer `kubectl get` / `describe` / `logs --tail` over dumping full object trees with secret refs expanded.
2. Truncate payloads before calling an external API (see `summarize.sh` clip limit).
3. If unsure whether a string is sensitive, redact it.
4. Never ask the model to “echo back” secrets or reconstruct redacted values.
