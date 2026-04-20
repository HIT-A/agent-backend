#!/bin/bash

# Agent Backend v2 测试脚本

BASE_URL="http://localhost:8080"

echo "=== Agent Backend v2 Test Script ==="
echo ""

# 1. Health Check
echo "1. Testing /health"
curl -s ${BASE_URL}/health | jq .
echo ""

# 2. List Skills
echo "2. Testing GET /v1/skills"
curl -s ${BASE_URL}/v1/skills | jq '.skills | length'
echo " skills available"
echo ""

# 3. Courses Search
echo "3. Testing POST /api/courses/search"
curl -s -X POST ${BASE_URL}/api/courses/search \
  -H "Content-Type: application/json" \
  -d '{"keyword": "数学", "campus": "shenzhen", "limit": 5}' | jq .
echo ""

# 4. Course Read
echo "4. Testing POST /api/courses/read"
curl -s -X POST ${BASE_URL}/api/courses/read \
  -H "Content-Type: application/json" \
  -d '{"course_code": "MA21003", "campus": "shenzhen"}' | jq .
echo ""

# 5. HITSZ Login (需要 hitsz-info-fetcher)
echo "5. Testing POST /api/hitsz/login"
echo "Note: This requires hitsz-info-fetcher binary to be available"
curl -s -X POST ${BASE_URL}/api/hitsz/login \
  -H "Content-Type: application/json" \
  -d '{"username": "2024312121", "password": "Anny770415"}' | jq .
echo ""

# 6. RAG Query (需要 Qdrant 和 Embedding)
echo "6. Testing POST /api/rag/query"
echo "Note: This requires Qdrant and Embedding API to be configured"
curl -s -X POST ${BASE_URL}/api/rag/query \
  -H "Content-Type: application/json" \
  -d '{"query": "高等数学", "top_k": 3}' | jq .
echo ""

echo "=== Test Complete ==="
