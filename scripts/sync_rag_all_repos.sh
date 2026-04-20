#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://127.0.0.1:8080}"
BRANCH="main"
COLLECTION="hit-courses"

REPOS=(
  "HIT-A/HITA_RagData"
  "hitlug/hit-network-resource"
  "hithesis/hithesis"
  "rccoder/HIT-Computer-Network"
  "HITSZ-OpenCS/HITSZ-OpenCS"
  "HITLittleZheng/HITCS"
  "LiYing0/CS_Gra-HITsz"
  "hoverwinter/HIT-OSLab"
  "sherlockqwq/HITWH_learningResource_share"
  "gzn00417/HIT-CS-Labs"
  "DWaveletT/HIT-COA-Lab"
  "hitszosa/universal-hit-thesis"
  "gcentqs/hit-854"
  "szxSpark/hit-master-course-note"
  "Mor-Li/HITSZ-OpenDS"
  "guoJohnny/-837-"
  "hitcslj/HIT-CS-Master"
  "PKUanonym/REKCARC-TSC-UHT"
  "QSCTech/zju-icicles"
)

echo "=== RAG Full Sync: ${#REPOS[@]} repositories ==="
echo "API: $API_URL"
echo "Collection: $COLLECTION"
echo ""

success=0
failed=0

for repo in "${REPOS[@]}"; do
  echo "--- Syncing: $repo ---"
  
  payload=$(cat <<JSON
{
  "repo": "${repo}",
  "ref": "${BRANCH}",
  "path_prefix": "",
  "source": "batch-sync",
  "collection": "${COLLECTION}",
  "dry_run": false,
  "max_files": 100,
  "max_chunks": 5000,
  "workers": 4,
  "store_in_cos": true,
  "cos_prefix": "rag-source"
}
JSON
)

  resp=$(curl -sS -m 300 -X POST "${API_URL}/api/rag/ingest" \
    -H 'Content-Type: application/json' \
    -d "$payload" 2>&1) || {
      echo "  ❌ Failed: $resp"
      ((failed++))
      continue
    }
  
  # Check if response contains ok:true
  if echo "$resp" | grep -q '"ok":true'; then
    echo "  ✅ Success"
    ((success++))
  else
    echo "  ⚠️  Response: $resp"
    ((failed++))
  fi

  sleep 2
done

echo ""
echo "=== Summary ==="
echo "Success: $success"
echo "Failed: $failed"
echo "Total: ${#REPOS[@]}"

if [ $failed -eq 0 ]; then
  echo "✅ All repositories synced!"
else
  echo "⚠️  Some repositories failed. Check logs above."
  exit 1
fi
