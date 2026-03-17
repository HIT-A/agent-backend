#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://127.0.0.1:8080}"
REPO="${RAGDATA_REPO:-HIT-A/HITA_RagData}"
BRANCH="${RAGDATA_BRANCH:-main}"
INTAKE_DIR="${RAGDATA_INTAKE_DIR:-incoming/manual_raw}"
COLLECTION="${QDRANT_COLLECTION:-hit_courses_2048}"

payload=$(cat <<JSON
{
  "input": {
    "repo": "${REPO}",
    "branch": "${BRANCH}",
    "intake_dir": "${INTAKE_DIR}",
    "max_file_size_mb": 25,
    "collection": "${COLLECTION}",
    "store_in_cos": true,
    "cos_prefix": "rag-intake/raw",
    "delete_invalid": true,
    "delete_on_succeeded": true,
    "max_files": 200
  },
  "trace": {
    "trigger": "systemd-daily"
  }
}
JSON
)

curl -sS -m 30 -X POST "${API_URL}/v1/skills/rag.intake_manual_folder:invoke" \
  -H 'Content-Type: application/json' \
  -d "$payload"
