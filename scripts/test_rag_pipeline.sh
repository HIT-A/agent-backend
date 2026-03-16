#!/bin/bash
# RAG Pipeline Test Script
# Tests the complete flow: GitHub Download → Convert → COS → RAG Ingest

set -e

echo "=== RAG Pipeline Test ==="
echo "Testing with HIT-A/HITA_RagData repository"
echo ""

# Configuration
API_URL="${API_URL:-http://localhost:8080}"
REPO="HIT-A/HITA_RagData"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Step 1: Check server health${NC}"
curl -s "${API_URL}/health" | jq . 2>/dev/null || echo "Server not responding at ${API_URL}"
echo ""

echo -e "${BLUE}Step 2: List available skills${NC}"
SKILLS=$(curl -s "${API_URL}/v1/skills")
echo "$SKILLS" | jq -r '.skills[].name' 2>/dev/null | head -30
echo ""

echo -e "${BLUE}Step 3: Test RAG Query (before ingestion)${NC}"
curl -s -X POST "${API_URL}/v1/skills/rag.query:invoke" \
  -H "Content-Type: application/json" \
  -d '{"input": {"query": "哈工大选课", "top_k": 3}}' | jq . 2>/dev/null || echo "RAG query failed"
echo ""

echo -e "${BLUE}Step 4: Test document conversion (local file)${NC}"
# Read a sample file
SAMPLE_FILE="/tmp/HITA_RagData/新生手册/选课相关.txt"
if [ -f "$SAMPLE_FILE" ]; then
    echo "Converting: $SAMPLE_FILE"
    CONTENT_BASE64=$(base64 -i "$SAMPLE_FILE")
    
    # Test via MCP if server is registered, otherwise show file content
    echo "File content preview:"
    head -20 "$SAMPLE_FILE"
    echo "..."
    echo ""
fi

echo -e "${BLUE}Step 5: Test GitHub file fetch${NC}"
curl -s -X POST "${API_URL}/v1/skills/rag.ingest:invoke" \
  -H "Content-Type: application/json" \
  -d "{
    \"input\": {
      \"repo\": \"${REPO}\",
      \"ref\": \"main\",
      \"path_prefix\": \"新生手册\",
      \"source\": \"hit-freshman-guide\",
      \"workers\": 2,
      \"store_in_cos\": false,
      \"max_files\": 5
    }
  }" | jq . 2>/dev/null || echo "RAG ingest test failed"
echo ""

echo -e "${BLUE}Step 6: Test aggregator search${NC}"
curl -s -X POST "${API_URL}/v1/skills/aggregator.search:invoke" \
  -H "Content-Type: application/json" \
  -d '{"input": {"query": "哈工大选课", "sources": ["rag"], "top_k": 5}}' | jq . 2>/dev/null || echo "Search failed"
echo ""

echo -e "${GREEN}=== Test Complete ===${NC}"
echo ""
echo "To run full pipeline with all repos:"
echo "  curl -X POST ${API_URL}/v1/skills/rag.ingest_from_github:invoke \\"
echo "    -d '{\"input\": {\"repos\": [\"HIT-A/HITA_RagData\"], \"auto_ingest_rag\": true}}'"