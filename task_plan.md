# Task Plan — rag.ingest (v1 ingestion MVP)

## Goal
Implement a minimal ingestion pipeline as a **server-side public skill** `rag.ingest` (async) that:
- pulls documents from `HIT-A/HITA_RagData`
- parses + chunks
- embeds via external embedding provider (BigModel)
- upserts vectors + payload into Qdrant

## Constraints
- No student P0 secrets ever processed.
- BigModel API key via env only; never logged; never committed.
- Job output must not contain raw document text; only counters + ids.
- Maintain /v1 SSOT invoke semantics.

## Phases
1) Design doc (SSOT addendum): contracts + pipeline + security
2) Implementation plan (TDD, bite-sized tasks)
3) Implement `rag.ingest` async skill
4) Verify on Tencent toolbox Qdrant

## Decisions
- Collection migration strategy: **B** — rebuild `hita_knowledge` in-place when switching to BigModel embeddings.
- Embedding model/dimensions: TBD (set via env; default recommended in design).
