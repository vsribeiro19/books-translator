#!/usr/bin/env bash
# End-to-end smoke test for the PDF book translator.
#
# Starts a stub OpenAI-compatible LLM, the pdf-service and the orchestrator,
# then drives the full flow: upload -> extract -> translate (stub) -> rebuild,
# verifying the preview PDF and the final translated PDF.
#
# REAL=1 runs against the real configured LLM provider (requires a key in
# config/secrets.env) instead of the stub.
set -euo pipefail

ROOT="$(CDPATH= cd "$(dirname "$0")/.." && pwd)"
ORCH_PORT="${ORCH_PORT:-8080}"
PDF_PORT="${PDF_PORT:-8081}"
LLM_PORT="${LLM_PORT:-9876}"
REAL="${REAL:-0}"

TMP="$(mktemp -d)"
PIDS=()
JOBID=""

cleanup() {
  for p in "${PIDS[@]:-}"; do
    kill "$p" 2>/dev/null || true
  done
  rm -rf "$TMP"
}
trap cleanup EXIT

die() { echo "smoke FAIL: $*" >&2; exit 1; }

port_open() { ( : >/dev/tcp/127.0.0.1/"$1" ) 2>/dev/null; }

wait_port() {
  local n=0
  until port_open "$1"; do
    n=$((n + 1))
    [ "$n" -gt 120 ] && die "timeout waiting for port $1"
    sleep 0.5
  done
}

json_field() { printf '%s' "$1" | grep -o "\"$2\":[0-9]*" | head -1 | grep -o '[0-9]*'; }

[ -d "$ROOT/pdf-service/node_modules" ] || die "pdf-service dependencies missing; run make install"

for p in "$ORCH_PORT" "$PDF_PORT"; do
  port_open "$p" && die "port $p already in use (stop the existing service or set *_PORT)"
done

# ---- 1. optional real provider check ----
if [ "$REAL" = "1" ]; then
  port_open "$LLM_PORT" && LLM_PORT_TAG="(port $LLM_PORT ignored in REAL mode)"
  if ! grep -q '^OPENCODE_API_KEY=..*' "$ROOT/config/secrets.env" 2>/dev/null && [ -z "${OPENCODE_API_KEY:-}" ]; then
    die "REAL mode requires OPENCODE_API_KEY in config/secrets.env (copy from secrets.env.example)"
  fi
  echo "mode: REAL (external LLM provider)"
else
  echo "mode: MOCK (stub LLM)"
fi

# ---- 2. start stub LLM (mock mode only) ----
if [ "$REAL" != "1" ]; then
  PORT="$LLM_PORT" node "$ROOT/scripts/stub-llm.cjs" &
  PIDS+=($!)
  wait_port "$LLM_PORT"
  echo "stub-llm up  :$LLM_PORT"
fi

# ---- 3. start pdf-service + generate sample PDF ----
(
  cd "$ROOT/pdf-service"
  mkdir -p data
  rm -f data/sample.pdf
  node scripts/mk-sample.cjs
  exec env PORT="$PDF_PORT" npx tsx src/index.ts
) &
PIDS+=($!)
wait_port "$PDF_PORT"
echo "pdf-service up :$PDF_PORT"

# ---- 4. start orchestrator ----
(
  cd "$ROOT/orchestrator"
  if [ "$REAL" = "1" ]; then
    exec env PORT="$ORCH_PORT" DATA_DIR="$TMP/data" CONFIG_FILE="$ROOT/config/orchestrator.yaml" go run ./cmd/server
  else
    exec env PORT="$ORCH_PORT" DATA_DIR="$TMP/data" CONFIG_FILE="$ROOT/config/orchestrator.yaml" \
      LLM_BASE_URL="http://127.0.0.1:$LLM_PORT/v1" LLM_MODEL=stub go run ./cmd/server
  fi
) &
PIDS+=($!)
wait_port "$ORCH_PORT"
echo "orchestrator up :$ORCH_PORT"

# ---- 5. create job ----
RESP="$(curl -sf -X POST "http://127.0.0.1:$ORCH_PORT/jobs" \
  -F pdf=@"$ROOT/pdf-service/data/sample.pdf" \
  -F preview_first_chapter=true)" || die "POST /jobs failed"
JOBID="$(printf '%s' "$RESP" | grep -o '"jobId":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -n "$JOBID" ] || die "no jobId in response: $RESP"
echo "job created: $JOBID"

# ---- 6. poll until preview_ready and download preview ----
preview_ok=0
for _ in $(seq 1 120); do
  st="$(curl -sf "http://127.0.0.1:$ORCH_PORT/jobs/$JOBID")" || die "GET /jobs failed"
  case "$st" in
    *'"status":"preview_ready"'*)
      curl -sf -o "$TMP/preview.pdf" "http://127.0.0.1:$ORCH_PORT/jobs/$JOBID/preview"
      [ "$(head -c4 "$TMP/preview.pdf")" = "%PDF" ] || die "preview is not a PDF"
      preview_ok=1
      break
      ;;
    *'"status":"failed"'*) die "job failed: $st" ;;
  esac
  sleep 0.5
done
[ "$preview_ok" = "1" ] || die "preview_ready timeout"
echo "preview OK ($(wc -c <"$TMP/preview.pdf") bytes)"

# ---- 7. poll until completed and download result ----
completed_ok=0
for _ in $(seq 1 120); do
  st="$(curl -sf "http://127.0.0.1:$ORCH_PORT/jobs/$JOBID")" || die "GET /jobs failed"
  case "$st" in
    *'"status":"completed"'*)
      curl -sf -o "$TMP/result.pdf" "http://127.0.0.1:$ORCH_PORT/jobs/$JOBID/result"
      [ "$(head -c4 "$TMP/result.pdf")" = "%PDF" ] || die "result is not a PDF"
      echo "final status: $st"
      completed_ok=1
      break
      ;;
    *'"status":"failed"'*) die "job failed: $st" ;;
  esac
  sleep 0.5
done
[ "$completed_ok" = "1" ] || die "completed timeout"
echo "result OK ($(wc -c <"$TMP/result.pdf") bytes)"

# ---- 8. assert progress (2/2 chapters) ----
done_="$(json_field "$st" chaptersDone)"
total_="$(json_field "$st" chaptersTotal)"
[ "$done_" = "$total_" ] && [ -n "$done_" ] || die "progress mismatch: $done_/$total_"
echo "progress OK: $done_/$total_ chapters"

# ---- 9. validate result content ----
if [ "$REAL" = "1" ]; then
  node "$ROOT/scripts/smoke-verify.cjs" "$TMP/result.pdf"
else
  node "$ROOT/scripts/smoke-verify.cjs" "$TMP/result.pdf" "PT:"
fi

echo "SMOKE OK"