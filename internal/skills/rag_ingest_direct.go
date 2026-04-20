package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func IngestMarkdownDirect(ctx context.Context, markdownContent []byte, sourceName, sourceTag string, qdrant *QdrantClient, embedder EmbeddingProvider) (int, error) {
	if len(markdownContent) == 0 {
		return 0, fmt.Errorf("empty content")
	}

	contentSHA := sha256.Sum256(markdownContent)
	sha256Hex := hex.EncodeToString(contentSHA[:])

	doc, err := ParseDocument(sourceName, markdownContent)
	if err != nil {
		return 0, fmt.Errorf("parse document: %w", err)
	}

	normalized := buildNormalizedMarkdown(doc)
	_ = normalized
	chunks := ChunkText(doc, 1400)
	if len(chunks) == 0 {
		return 0, fmt.Errorf("no chunks generated")
	}

	docID := fmt.Sprintf("direct:%s:%s", sourceTag, sha256Hex[:12])

	_ = qdrant.DeleteByDocID(ctx, qdrant.Collection, docID)

	points := make([]QdrantPoint, 0, len(chunks))
	for _, ch := range chunks {
		vec, err := embedder.Embed(ctx, ch.Text)
		if err != nil {
			return 0, fmt.Errorf("embed chunk: %w", err)
		}
		payload := map[string]any{
			"doc_id":   docID,
			"chunk_id": ch.ChunkID,
			"title":    doc.Title,
			"snippet":  trimSnippet(ch.Text),
			"source":   sourceTag,
			"category": sourceTag,
			"path":     sha256Hex,
		}
		points = append(points, QdrantPoint{ID: stablePointID(docID, ch.ChunkID), Vector: vec, Payload: payload})
	}

	if err := qdrant.Upsert(ctx, qdrant.Collection, points); err != nil {
		return 0, fmt.Errorf("qdrant upsert: %w", err)
	}

	return len(chunks), nil
}
