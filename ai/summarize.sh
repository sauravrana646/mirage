#!/usr/bin/env bash
# Gather redacted cluster evidence for a PreviewEnvironment and emit a summary.
# If OPENAI_API_KEY is set, call the OpenAI API; otherwise use a heuristic summary.
#
# Usage:
#   ./ai/summarize.sh --name pr-42 --namespace mirage-system [--out summary.md]
#
# See ai/README.md and ai/redaction.md.
set -euo pipefail

PE_NAME=""
PE_NAMESPACE="mirage-system"
OUT_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) PE_NAME="$2"; shift 2 ;;
    --namespace) PE_NAMESPACE="$2"; shift 2 ;;
    --out) OUT_FILE="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,10p' "$0"
      exit 0
      ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$PE_NAME" ]]; then
  echo "--name is required" >&2
  exit 2
fi

# --- redaction helpers (keep in sync with ai/redaction.md) ---
redact() {
  # shellcheck disable=SC2001
  sed -E \
    -e 's/(Bearer[[:space:]]+)[A-Za-z0-9._~\-\/+%=]+/\1***REDACTED***/g' \
    -e 's/(token[=:][[:space:]]*)[A-Za-z0-9._~\-\/+%=]+/\1***REDACTED***/gi' \
    -e 's/(password[=:][[:space:]]*)[^[:space:]]+/\1***REDACTED***/gi' \
    -e 's/(api[_-]?key[=:][[:space:]]*)[A-Za-z0-9._~\-\/+%=]+/\1***REDACTED***/gi' \
    -e 's/(secret[=:][[:space:]]*)[^[:space:]]+/\1***REDACTED***/gi' \
    -e 's/ghp_[A-Za-z0-9]+/ghp_***REDACTED***/g' \
    -e 's/gho_[A-Za-z0-9]+/gho_***REDACTED***/g' \
    -e 's/ghu_[A-Za-z0-9]+/ghu_***REDACTED***/g' \
    -e 's/ghs_[A-Za-z0-9]+/ghs_***REDACTED***/g' \
    -e 's/ghr_[A-Za-z0-9]+/ghr_***REDACTED***/g' \
    -e 's/github_pat_[A-Za-z0-9_]+/github_pat_***REDACTED***/g' \
    -e 's/eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/***JWT_REDACTED***/g' \
    -e 's/([A-Za-z0-9_]*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|PRIVATE)[A-Za-z0-9_]*[=:][[:space:]]*)[^[:space:]]+/\1***REDACTED***/gi' \
    -e 's/AKIA[0-9A-Z]{16}/***AWS_KEY_REDACTED***/g'
}

gather_evidence() {
  local tmp
  tmp="$(mktemp)"
  {
    echo "=== PreviewEnvironment ==="
    # Drop env / envFrom before any LLM or log export.
    kubectl -n "$PE_NAMESPACE" get previewenvironment "$PE_NAME" -o json 2>/dev/null \
      | jq 'del(.spec.env, .spec.envFrom, .metadata.managedFields)' 2>/dev/null \
      | redact \
      || kubectl -n "$PE_NAMESPACE" get previewenvironment "$PE_NAME" -o yaml 2>&1 \
         | sed '/^[[:space:]]*env:/,/^[[:space:]]*[a-zA-Z]/d' \
         | redact \
      || echo "(not found)"
    echo
    echo "=== Events (PE namespace) ==="
    kubectl -n "$PE_NAMESPACE" get events --field-selector "involvedObject.name=${PE_NAME}" \
      --sort-by=.lastTimestamp 2>&1 || true
    local target_ns
    target_ns="$(kubectl -n "$PE_NAMESPACE" get previewenvironment "$PE_NAME" \
      -o jsonpath='{.spec.targetNamespace}' 2>/dev/null || true)"
    if [[ -n "$target_ns" ]]; then
      echo
      echo "=== Events (targetNamespace=${target_ns}) ==="
      kubectl -n "$target_ns" get events --sort-by=.lastTimestamp 2>&1 | tail -n 80 || true
      echo
      echo "=== Pods ==="
      kubectl -n "$target_ns" get pods -o wide 2>&1 || true
      echo
      echo "=== Pod logs (last 100 lines each, redacted) ==="
      local pods
      pods="$(kubectl -n "$target_ns" get pods -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)"
      for pod in $pods; do
        echo "--- ${pod} ---"
        kubectl -n "$target_ns" logs "$pod" --all-containers --tail=100 2>&1 || true
      done
      echo
      echo "=== Deployment describe ==="
      kubectl -n "$target_ns" describe deploy 2>&1 | head -n 120 || true
    fi
  } > "$tmp"
  redact < "$tmp"
  rm -f "$tmp"
}

heuristic_summary() {
  local evidence="$1"
  local phase reason message
  phase="$(kubectl -n "$PE_NAMESPACE" get previewenvironment "$PE_NAME" \
    -o jsonpath='{.status.phase}' 2>/dev/null || echo unknown)"
  reason="$(kubectl -n "$PE_NAMESPACE" get previewenvironment "$PE_NAME" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}' 2>/dev/null || true)"
  message="$(kubectl -n "$PE_NAMESPACE" get previewenvironment "$PE_NAME" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}' 2>/dev/null || true)"

  echo "_Heuristic summary (OPENAI_API_KEY not set). Evidence was redacted per ai/redaction.md._"
  echo
  echo "**PreviewEnvironment:** \`${PE_NAMESPACE}/${PE_NAME}\`"
  echo
  echo "| Field | Value |"
  echo "|---|---|"
  echo "| Phase | \`${phase:-unknown}\` |"
  echo "| Ready reason | \`${reason:-n/a}\` |"
  echo "| Message | ${message:-n/a} |"
  echo
  echo "**Likely signals from events/logs:**"
  echo
  if echo "$evidence" | grep -qiE 'ImagePullBackOff|ErrImagePull'; then
    echo "- Image pull failure — check digest, registry auth, and image existence."
  fi
  if echo "$evidence" | grep -qiE 'CrashLoopBackOff|OOMKilled'; then
    echo "- Container crash / OOM — inspect command, probes, and resource limits."
  fi
  if echo "$evidence" | grep -qiE 'NamespaceConflict'; then
    echo "- Namespace conflict — \`targetNamespace\` may be owned by another CR."
  fi
  if echo "$evidence" | grep -qiE 'CreateContainerConfigError|secret|configmap'; then
    echo "- Config/secret mount issue — verify referenced Secrets/ConfigMaps exist."
  fi
  if echo "$evidence" | grep -qiE 'FailedScheduling|Insufficient'; then
    echo "- Scheduling pressure — cluster may lack CPU/memory for the preview."
  fi
  if ! echo "$evidence" | grep -qiE 'ImagePullBackOff|CrashLoopBackOff|NamespaceConflict|FailedScheduling|OOMKilled'; then
    echo "- No high-confidence pattern matched; review the redacted evidence excerpt below."
  fi
  echo
  echo "<details><summary>Redacted evidence (truncated)</summary>"
  echo
  echo '```text'
  echo "$evidence" | tail -n 80
  echo '```'
  echo
  echo "</details>"
}

openai_summary() {
  local evidence="$1"
  local model="${OPENAI_MODEL:-gpt-4o-mini}"
  local prompt_file payload_file response_file
  prompt_file="$(mktemp)"
  payload_file="$(mktemp)"
  response_file="$(mktemp)"

  cat > "$prompt_file" <<'PROMPT'
You are an advisory assistant for Mirage PreviewEnvironments.
Given redacted Kubernetes status, events, and logs, write a short PR comment:
- 2-5 bullet root-cause hypotheses
- 2-4 concrete next steps for a human
- Do not invent secrets or ask for them
- Do not suggest changing the Mirage controller reconcile loop to call an LLM
Keep the tone factual and concise. Use markdown.
PROMPT

  # Truncate evidence to keep request small
  local clipped
  clipped="$(echo "$evidence" | tail -c 12000)"

  if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required for OpenAI path; falling back to heuristic" >&2
    heuristic_summary "$evidence"
    rm -f "$prompt_file" "$payload_file" "$response_file"
    return
  fi

  jq -n \
    --arg model "$model" \
    --rawfile sys "$prompt_file" \
    --arg evidence "$clipped" \
    '{
      model: $model,
      temperature: 0.2,
      messages: [
        {role: "system", content: $sys},
        {role: "user", content: ("PreviewEnvironment evidence (redacted):\n\n" + $evidence)}
      ]
    }' > "$payload_file"

  if curl -fsS https://api.openai.com/v1/chat/completions \
      -H "Authorization: Bearer ${OPENAI_API_KEY}" \
      -H "Content-Type: application/json" \
      -d @"$payload_file" > "$response_file"; then
    echo "_AI advisory summary (external to reconcile). Evidence redacted per ai/redaction.md._"
    echo
    jq -r '.choices[0].message.content // empty' "$response_file"
  else
    echo "OpenAI request failed; falling back to heuristic summary." >&2
    heuristic_summary "$evidence"
  fi

  rm -f "$prompt_file" "$payload_file" "$response_file"
}

main() {
  local evidence summary
  evidence="$(gather_evidence)"
  if [[ -n "${OPENAI_API_KEY:-}" ]]; then
    summary="$(openai_summary "$evidence")"
  else
    summary="$(heuristic_summary "$evidence")"
  fi

  if [[ -n "$OUT_FILE" ]]; then
    printf '%s\n' "$summary" > "$OUT_FILE"
  else
    printf '%s\n' "$summary"
  fi
}

main
