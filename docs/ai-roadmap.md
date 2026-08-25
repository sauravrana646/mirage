# AI roadmap (Phase 7+)

**Principle:** The Mirage controller stays **deterministic**. AI is an advisory side path that speaks the same CRD/status language.

## Why not LLM-in-reconcile

- Reconcile must be fast, retry-safe, and testable offline.
- LLM latency/outages must not block create/delete.
- Auditors and CNCF-style projects expect controllers to be predictable.

## Integration pattern

```text
PreviewEnvironment status.conditions Ready=False
        │
        ▼
  AI worker / GitHub Action
   - read Events + pod logs (redacted)
   - call LLM
   - comment on PR OR patch annotation mirage.dev/ai-summary
        │
        ▼
  Humans / CI act; controller ignores AI except optional annotation display
```

## Use cases (priority)

| Priority | Use case | Input | Output |
|----------|----------|-------|--------|
| P0 | Failure RCA | Events, logs, status | PR comment |
| P1 | Manifest hints | Dockerfile, repo meta | suggested `spec.resources` |
| P2 | Risk gate | diff + heuristics (+ LLM) | annotation allow/deny preview |
| P3 | NL ops | chat text | creates the same CR YAML |

## Contracts

- Annotation keys (proposed): `mirage.dev/ai-summary`, `mirage.dev/ai-risk`
- Never store raw secrets in annotations
- Controller may copy summary into `status` later — still produced externally

## Stack (when you get here)

- GitHub Action or small Go service
- Any LLM API
- Prompt + redaction templates in-repo under `ai/` (future)

## Done definition

A failed preview produces a useful explanation on the PR **without** importing an LLM client into `controllers/`.
