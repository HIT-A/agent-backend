# rag.ingest (v1) — Ingestion Pipeline Design

**Goal:** Add a minimal server-side public skill `rag.ingest` (async) that ingests documents from `HIT-A/HITA_RagData`, embeds them using an external embedding provider (BigModel), and upserts them into Qdrant so `rag.query` returns non-empty hits.

**Scope (v1 MVP):**
- Supported formats: `txt`, `md`, `html`
- Unsupported/placeholder: `pdf` (planned v1.1)
- One collection: `hita_knowledge` (rebuild in-place; see decision)
- Minimal chunking + payload contract aligned to SSOT

**Non-goals:**
- No private/EAS data ingestion
- No full workflow UI
- No complex layout PDF parsing in v1

---

## 1) Key Decisions

### 1.1 Qdrant collection migration
- **Decision:** Rebuild `hita_knowledge` in-place when switching to BigModel embeddings.
- Implication: `vectors.size` must match BigModel output dimensions.

### 1.2 Embedding provider
- Use **BigModel Embeddings API** (`/api/paas/v4/embeddings`).
- Key is provided via env `BIGMODEL_API_KEY` only (never logged).
- Model/dimensions are env-configurable.

---

## 2) End-to-end ingestion flow

1) Client calls `POST /v1/skills/rag.ingest:invoke` with an input selecting repo/ref/path.
2) Server returns `{ ok:true, job_id }`.
3) Worker executes ingestion pipeline:
   1. Fetch file list from GitHub repo `HIT-A/HITA_RagData` (by path_prefix).
   2. Download file contents.
   3. Parse to plain text.
   4. Chunk into passages.
   5. Embed via BigModel in batches.
   6. Upsert into Qdrant with payload.
4) Job completes with output counters (no raw text), e.g. `{ processed_files, processed_chunks, upserted_points, skipped_files, errors_count }`.

---

## 3) Skill contract (SSOT-aligned)

### 3.1 Skill name
- `rag.ingest`

### 3.2 Invoke
- `POST /v1/skills/rag.ingest:invoke`
- **Async**: returns `{ ok:true, job_id }`
- Failure: HTTP 200 `{ ok:false, error:{code,message,retryable} }` (same as SSOT)

### 3.3 Input (v1 MVP)
```json
{
  "repo": "HIT-A/HITA_RagData",
  "ref": "main",
  "path_prefix": "optional/subdir",
  "source": "string",
  "collection": "hita_knowledge",
  "dry_run": false,
  "max_files": 100,
  "max_chunks": 5000
}
```

Notes:
- `path_prefix` allows ingesting one subfolder at a time.
- `source` is a label written into payload (e.g., `handbook`, `notes`, `official`).
- `dry_run=true` runs fetch/parse/chunk only and reports counters; no Qdrant writes.

### 3.4 Output (job.output_json)
```json
{
  "processed_files": 10,
  "processed_chunks": 240,
  "upserted_points": 240,
  "skipped_files": 0,
  "errors_count": 0
}
```

---

## 4) Payload contract (must match rag.query expectations)

Each Qdrant point MUST have payload keys:
- **required:** `doc_id`, `snippet`
- **recommended:** `chunk_id`, `title`, `url`, `source`

Point ID:
- `id = sha256(doc_id + ":" + chunk_id)` (stable, idempotent upserts)

---

## 5) Parsing + chunking (v1 MVP)

### 5.1 Parsing
- `txt`: raw text
- `md`: keep text; remove front-matter if present; minimal cleanup
- `html`: strip `script/style`, then extract body text (best-effort)

### 5.2 Chunking
- Split by paragraphs/newlines, then pack into chunks with max ~1200–1500 chars.
- Preserve minimal metadata:
  - `doc_id`
  - `chunk_id` (index within doc)
  - `title` best-effort (file name or first heading)

---

## 6) BigModel Embeddings integration

Env:
- `BIGMODEL_API_KEY` (required)
- `BIGMODEL_EMBEDDING_MODEL` (default `embedding-3`)
- `BIGMODEL_DIMENSIONS` (default: choose later; must match Qdrant vectors.size)

Batching:
- embedding-3 supports arrays up to 64 items. Use batch size 32–64.

---

## 7) Qdrant upsert

Env:
- `QDRANT_URL`, `QDRANT_COLLECTION` (or per-request `collection`)
- `QDRANT_API_KEY` if enabled

Upsert endpoint:
- `PUT /collections/{collection}/points?wait=true`

---

## 8) Security / logging rules

- Never log document raw text.
- Never log `BIGMODEL_API_KEY` / `QDRANT_API_KEY`.
- Jobs store may persist `input_json` but should not persist raw docs.
- Job output_json contains only counters + ids.

---

## 9) Open questions (v1.1+)
- PDF parsing approach (text extraction quality)
- Incremental ingestion (hash-based skip)
- Collection rebuild automation
