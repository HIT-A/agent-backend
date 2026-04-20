#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://127.0.0.1:8080}"
REPO="${1:-HIT-A/HITA_RagData}"
BRANCH="${2:-main}"

echo "=== RAG Manual Sync ==="
echo "API: $API_URL"
echo "Repo: $REPO"
echo "Branch: $BRANCH"
echo ""
payload=$(cat <<JSON
{
  "repo": "${REPO}",
  "ref": "${BRANCH}",
  "path_prefix": "",
  "source": "manual-sync",
  "collection": "hit-courses",
  "dry_run": false,
  "max_files": 100,
  "max_chunks": 5000,
  "workers": 4,
  "store_in_cos": true,
  "cos_prefix": "rag-source"
}
JSON
)

echo "Sending request..."
resp=$(curl -sS -m 300 -X POST "${API_URL}/api/rag/ingest" \
  -H 'Content-Type: application/json' \
  -d "$payload" 2>&1) || {
    echo "❌ Request failed: $resp"
    exit 1
  }

echo "Response:"
echo "$resp" | python3 -m json.tool 2>/dev/null || echo "$resp"

echo ""
echo "=== Done ==="
