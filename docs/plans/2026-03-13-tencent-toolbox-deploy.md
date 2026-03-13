# Tencent Cloud AI Agent 工具箱 — agent-backend Deployment (v0)

**Scope:** Deploy `HIT-A/agent-backend` on the Tencent Cloud “AI Agent 工具箱 1.0.0” image.

This guide focuses on:
- environment variables
- Qdrant collection creation
- running the server
- verifying the SSOT `/v1` contracts

> Privacy reminder: **No EAS cookies/tokens/passwords** should ever be sent to this server. Client runs private skills.

---

## 0) Assumptions (based on your server probe)
- OS: Ubuntu 24.04
- Qdrant running:
  - REST: `:6333`
  - gRPC: `:6334`
- Langfuse stack running: `:3000` (optional for v1+)

---

## 1) Get the code

```bash
git clone git@github.com:HIT-A/agent-backend.git
cd agent-backend
```

Optional sanity check:
```bash
go test ./...
```

---

## 2) Environment variables (v0)

### 2.1 Required

| env | example | meaning |
|---|---|---|
| `DB_PATH` | `./data/jobs.db` | SQLite job store path |
| `QDRANT_URL` | `http://127.0.0.1:6333` | Qdrant base URL |
| `QDRANT_COLLECTION` | `hita_knowledge` | Qdrant collection name |

### 2.2 Optional

| env | example | meaning |
|---|---|---|
| `QDRANT_API_KEY` | `...` | if Qdrant auth enabled |
| `WORKER_COUNT` | `4` | async worker count |
| `EMBEDDING_PROVIDER` | `deterministic` | v0 default fallback |

> **Important:** v0 embedding fallback is a **3-dim deterministic vector**.
> Therefore **Qdrant collection vector size must be 3** unless you implement a real embedding provider.

---

## 3) Create / verify Qdrant collection

### 3.1 Check if collection exists

If Qdrant auth is **disabled**:
```bash
curl -sS "http://127.0.0.1:6333/collections/hita_knowledge" | jq
```

If Qdrant auth is **enabled** (you will see HTTP 401 without a key):
```bash
curl -sS "http://127.0.0.1:6333/collections/hita_knowledge" \
  -H "api-key: $QDRANT_API_KEY" | jq
```

### 3.2 Create collection (vector size = 3)

If Qdrant auth is **disabled**:
```bash
curl -sS -X PUT "http://127.0.0.1:6333/collections/hita_knowledge" \
  -H "Content-Type: application/json" \
  -d '{"vectors":{"size":3,"distance":"Cosine"}}' | jq
```

If Qdrant auth is **enabled**:
```bash
curl -sS -X PUT "http://127.0.0.1:6333/collections/hita_knowledge" \
  -H "Content-Type: application/json" \
  -H "api-key: $QDRANT_API_KEY" \
  -d '{"vectors":{"size":3,"distance":"Cosine"}}' | jq
```

---

## 4) Build & run

### 4.1 Build

```bash
go build -o bin/agent-backend ./cmd/server
```

### 4.2 Run

```bash
mkdir -p ./data
DB_PATH=./data/jobs.db \
QDRANT_URL=http://127.0.0.1:6333 \
QDRANT_COLLECTION=hita_knowledge \
WORKER_COUNT=4 \
./bin/agent-backend
```

Default listen: `:8080`.

---

## 5) Verify API (SSOT /v1)

### 5.1 Health

```bash
curl -sS http://127.0.0.1:8080/health
```
Expected:
```json
{"status":"ok"}
```

### 5.2 Skills registry

```bash
curl -sS http://127.0.0.1:8080/v1/skills | jq
```
Expected:
- includes `rag.query`
- includes `is_async` field

### 5.3 Invoke semantics (must be HTTP 200 even on failures)

Unknown skill must be `ok=false` (HTTP 200):
```bash
curl -i -sS -X POST http://127.0.0.1:8080/v1/skills/does-not-exist:invoke \
  -H 'Content-Type: application/json' \
  -d '{"input":{}}'
```

### 5.4 RAG query (query-only)

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/skills/rag.query:invoke \
  -H 'Content-Type: application/json' \
  -d '{"input":{"query":"data structures","top_k":5}}' | jq
```

Notes:
- If you have not ingested any vectors/points, `hits` may be empty — this is fine for v0.
- If you see a Qdrant error about vector size mismatch, your collection `vectors.size` does not match the embedding provider output size (v0 deterministic = 3).

---

## 6) Ingestion (v1+) placeholder

v0 only provides **query**. To get meaningful results, you will need an ingestion pipeline:
- parse documents → chunk → embedding → upsert points into Qdrant
- payload should include at least:
  - `doc_id` (required)
  - `snippet` (required)
  - recommended: `chunk_id`, `title`, `url`, `source`

---

## 7) Production hardening checklist (recommended)

- Rotate default plaintext secrets (Langfuse/Qdrant/etc)
- Restrict ports (prefer exposing only 80/443 + internal network for Qdrant)
- Configure reverse proxy (Caddy/Nginx) with TLS
- Ensure server logs do not include request bodies
