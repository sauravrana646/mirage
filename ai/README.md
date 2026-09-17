# AI advisory (outside reconcile)

Mirage keeps the controller **deterministic**. AI never runs inside the reconcile loop.

## Why

- Reconcile must be fast, retry-safe, and testable offline.
- LLM latency or outages must not block create/update/delete of preview environments.
- Auditors expect operators to be predictable; advisory text is a side effect, not desired state.

## Side-path

```text
PreviewEnvironment Ready=False / phase=Failed
        │
        ▼
  GitHub Action (ai-advisory.yml)
   - kubectl: status, events, pod logs
   - redact per ai/redaction.md
   - ./ai/summarize.sh  →  OpenAI if OPENAI_API_KEY set, else heuristic
   - upsert PR comment (<!-- mirage-ai-advisory -->)
        │
        ▼
  Humans / CI act; controller ignores AI output
```

Optional later: patch annotation `mirage.dev/ai-summary` on the CR. The controller may display it but must not call an LLM.

## Scripts

| Path | Role |
|------|------|
| `summarize.sh` | Collect redacted evidence; emit markdown summary |
| `redaction.md` | Rules for what must never leave the cluster toward an LLM |

## Secrets / vars

| Name | Where | Purpose |
|------|-------|---------|
| `KUBE_CONFIG` | Actions secret | Base64 kubeconfig (mirage-ci SA) |
| `OPENAI_API_KEY` | Actions secret (optional) | Enable LLM summary |
| `OPENAI_MODEL` | Actions variable (optional) | Default `gpt-4o-mini` |

Without `OPENAI_API_KEY`, `summarize.sh` posts a heuristic summary from events/logs only.
