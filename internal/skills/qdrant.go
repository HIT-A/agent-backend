package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type QdrantClient struct {
	BaseURL    string
	Collection string
	APIKey     string
	HTTP       *http.Client
}

func NewQdrantClientFromEnv() (*QdrantClient, error) {
	base := strings.TrimSpace(os.Getenv("QDRANT_URL"))
	base = strings.TrimRight(base, "/")
	if base == "" {
		return nil, errors.New("QDRANT_URL is required")
	}
	col := strings.TrimSpace(os.Getenv("QDRANT_COLLECTION"))
	if col == "" {
		return nil, errors.New("QDRANT_COLLECTION is required")
	}

	c := &QdrantClient{
		BaseURL:    base,
		Collection: col,
		APIKey:     strings.TrimSpace(os.Getenv("QDRANT_API_KEY")),
		HTTP:       http.DefaultClient,
	}
	return c, nil
}

// Hit is the SSOT RAG hit shape returned by rag.query.
type Hit struct {
	DocID   string  `json:"doc_id"`
	ChunkID string  `json:"chunk_id"`
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"`
}

func (c *QdrantClient) Search(ctx context.Context, vector []float64, topK int) ([]Hit, error) {
	if c == nil {
		return nil, errors.New("nil qdrant client")
	}
	if c.HTTP == nil {
		c.HTTP = http.DefaultClient
	}
	if topK <= 0 {
		topK = 5
	}

	endpoint := fmt.Sprintf("%s/collections/%s/points/search", c.BaseURL, c.Collection)

	body, err := json.Marshal(struct {
		Vector      []float64 `json:"vector"`
		Limit       int       `json:"limit"`
		WithPayload bool      `json:"with_payload"`
	}{
		Vector:      vector,
		Limit:       topK,
		WithPayload: true,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		// Qdrant supports api-key header.
		req.Header.Set("api-key", c.APIKey)
	}

	// Keep a finite timeout even if caller forgets.
	ctx2, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx2)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return nil, fmt.Errorf("qdrant search failed: status=%d body=%q", res.StatusCode, string(b))
	}

	var decoded struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, err
	}

	out := make([]Hit, 0, len(decoded.Result))
	for _, r := range decoded.Result {
		p := r.Payload
		out = append(out, Hit{
			DocID:   getString(p, "doc_id"),
			ChunkID: getString(p, "chunk_id"),
			Title:   getString(p, "title"),
			URL:     getString(p, "url"),
			Snippet: getString(p, "snippet"),
			Score:   r.Score,
			Source:  getString(p, "source"),
		})
	}
	return out, nil
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// QdrantPoint is the minimal payload for points upsert.
type QdrantPoint struct {
	ID      any            `json:"id"`
	Vector  []float64      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

func (c *QdrantClient) Upsert(ctx context.Context, collection string, points []QdrantPoint) error {
	if c == nil {
		return errors.New("nil qdrant client")
	}
	if c.HTTP == nil {
		c.HTTP = http.DefaultClient
	}
	col := collection
	if strings.TrimSpace(col) == "" {
		col = c.Collection
	}
	if strings.TrimSpace(col) == "" {
		return errors.New("qdrant collection is required")
	}

	endpoint := fmt.Sprintf("%s/collections/%s/points?wait=true", c.BaseURL, col)

	body, err := json.Marshal(struct {
		Points []QdrantPoint `json:"points"`
	}{
		Points: points,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("api-key", c.APIKey)
	}

	ctx2, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx2)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return fmt.Errorf("qdrant upsert failed: status=%d body=%q", res.StatusCode, string(b))
	}

	return nil
}

func (c *QdrantClient) DeleteByDocID(ctx context.Context, collection, docID string) error {
	if c == nil {
		return errors.New("nil qdrant client")
	}
	if c.HTTP == nil {
		c.HTTP = http.DefaultClient
	}
	col := collection
	if strings.TrimSpace(col) == "" {
		col = c.Collection
	}
	if strings.TrimSpace(col) == "" {
		return errors.New("qdrant collection is required")
	}
	if strings.TrimSpace(docID) == "" {
		return errors.New("docID is required")
	}

	endpoint := fmt.Sprintf("%s/collections/%s/points/delete?wait=true", c.BaseURL, col)
	body, err := json.Marshal(map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{
					"key": "doc_id",
					"match": map[string]any{
						"value": docID,
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("api-key", c.APIKey)
	}

	ctx2, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx2)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return fmt.Errorf("qdrant delete failed: status=%d body=%q", res.StatusCode, string(b))
	}

	return nil
}
