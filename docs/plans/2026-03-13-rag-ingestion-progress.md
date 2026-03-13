# rag.ingest (v1) — Progress

## 2026-03-13
- Decided ingestion will be a **server public async skill**: `rag.ingest`.
- Data source repo: `HIT-A/HITA_RagData` (folders by source; types include txt/pdf/html/md).
- MVP scope: txt/md/html first; PDF deferred.
- Collection strategy chosen: rebuild `hita_knowledge` in-place when switching to BigModel embeddings.

## Next
- Produce implementation plan (TDD) and start implementing:
  - GitHub fetcher
  - parser + chunker
  - BigModel embedding provider
  - Qdrant upsert
  - rag.ingest async skill + job output counters
