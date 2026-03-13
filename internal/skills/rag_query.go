package skills

import (
	"context"
	"strings"
)

// NewRAGQuerySkill registers the rag.query skill.
//
// v0: query-only, sync, backed by Qdrant vector search.
func NewRAGQuerySkill() Skill {
	return Skill{
		Name:    "rag.query",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			query, _ := input["query"].(string)
			query = strings.TrimSpace(query)
			if query == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "query is required", Retryable: false}
			}

			topK := 5
			if v, ok := input["top_k"]; ok {
				if f, ok := v.(float64); ok {
					topK = int(f)
				}
			}

			provider, err := NewEmbeddingProviderFromEnv()
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "embedding provider config error: " + err.Error(), Retryable: false}
			}
			vec, err := provider.Embed(ctx, query)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "embedding failed: " + err.Error(), Retryable: true}
			}

			qdrant, err := NewQdrantClientFromEnv()
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "qdrant config error: " + err.Error(), Retryable: false}
			}

			hits, err := qdrant.Search(ctx, vec, topK)
			if err != nil {
				// Treat transport/search failures as retryable.
				return nil, &InvokeError{Code: "INTERNAL", Message: "qdrant search failed: " + err.Error(), Retryable: true}
			}

			// SSOT output shape.
			return map[string]any{"hits": hits}, nil
		},
	}
}
