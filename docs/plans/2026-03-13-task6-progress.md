# Task 6 Progress Notes (RAG query)

Date: 2026-03-13

## SSOT alignment
- SSOT requires `invoke` to always return HTTP 200 with `{ok:false,error}` on failures.
- Implemented SSOT invoke error envelope (SKILL_NOT_FOUND, INVALID_JSON) earlier and pushed.

## Task 6 status
- Added failing tests for `rag.query` (commit `c2cef42`).
- Implemented `rag.query` skill backed by Qdrant (query-only) and made tests pass (commit `44f13bf`).

## Contracts implemented (v0)
- Skill name: `rag.query`
- Input: `{ query: string, top_k?: number, filters?: object }`
- Output: `{ hits: [ { doc_id, chunk_id?, title?, url?, snippet, score, source? } ] }`

## Runtime config (env)
- `QDRANT_URL` (required for real calls)
- `QDRANT_COLLECTION` (required for real calls)
- `QDRANT_API_KEY` (optional)

## Notes / Known limitations
- v0 uses a minimal deterministic embedding fallback if no embedding provider configured.
- Next step: decide embedding provider strategy (toolbox sandbox? local model? external API) and implement ingestion (v1).
