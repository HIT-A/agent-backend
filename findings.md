# Findings — rag.ingest (v1 ingestion MVP)

- Qdrant on toolbox requires API key (401 without it); agent-backend supports `QDRANT_API_KEY` header `api-key`.
- Current `rag.query` exists and expects Qdrant payload keys: `doc_id, chunk_id, title, url, snippet, source`.
- Current embedding providers in code: deterministic + stub only. Need to add BigModel embedding provider.
- Data repo planned: https://github.com/HIT-A/HITA_RagData (currently empty) with folders by source; file types include txt/pdf/html/md.
- v1 ingestion MVP should support: txt/md/html first; pdf as v1.1.
