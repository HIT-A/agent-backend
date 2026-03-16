package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// EmbeddingProvider converts an input string into an embedding vector.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

func NewEmbeddingProviderFromEnv() (EmbeddingProvider, error) {
	switch strings.TrimSpace(os.Getenv("EMBEDDING_PROVIDER")) {
	case "", "deterministic":
		return DeterministicEmbeddingProvider{}, nil
	case "stub":
		v := strings.TrimSpace(os.Getenv("EMBEDDING_STUB_VECTOR"))
		if v == "" {
			return nil, errors.New("EMBEDDING_STUB_VECTOR is required when EMBEDDING_PROVIDER=stub")
		}
		vec, err := parseCSVFloat64(v)
		if err != nil {
			return nil, err
		}
		return StubEmbeddingProvider{Vector: vec}, nil
	case "bigmodel":
		return NewBigModelEmbeddingProviderFromEnv()
	default:
		return nil, errors.New("unknown EMBEDDING_PROVIDER")
	}
}

// DeterministicEmbeddingProvider is a tiny fallback used when no provider is configured.
type DeterministicEmbeddingProvider struct{}

func (DeterministicEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	_ = ctx

	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	sum := h.Sum64()

	// 3-dim stable embedding in [0,1]. Keep minimal for v0.
	b0 := float64((sum >> 0) & 0xff)
	b1 := float64((sum >> 8) & 0xff)
	b2 := float64((sum >> 16) & 0xff)
	return []float64{b0 / 255.0, b1 / 255.0, b2 / 255.0}, nil
}

type StubEmbeddingProvider struct {
	Vector []float64
}

func (s StubEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	_ = ctx
	_ = text
	out := make([]float64, len(s.Vector))
	copy(out, s.Vector)
	return out, nil
}

func parseCSVFloat64(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, errors.New("empty embedding vector")
	}
	return out, nil
}

// BigModelEmbeddingProvider uses ZHIPU BigModel API for embeddings.
type BigModelEmbeddingProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int
	HTTP       *http.Client
}

func NewBigModelEmbeddingProviderFromEnv() (*BigModelEmbeddingProvider, error) {
	apiKey := strings.TrimSpace(os.Getenv("BIGMODEL_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("BIGMODEL_API_KEY is required")
	}

	base := strings.TrimSpace(os.Getenv("BIGMODEL_BASE_URL"))
	if base == "" {
		base = "https://open.bigmodel.cn"
	}
	base = strings.TrimRight(base, "/")

	model := strings.TrimSpace(os.Getenv("BIGMODEL_EMBEDDING_MODEL"))
	if model == "" {
		model = "embedding-3"
	}

	dims := 2048
	if v := strings.TrimSpace(os.Getenv("BIGMODEL_DIMENSIONS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid BIGMODEL_DIMENSIONS: %w", err)
		}
		dims = n
	}

	return &BigModelEmbeddingProvider{
		BaseURL:    base,
		APIKey:     apiKey,
		Model:      model,
		Dimensions: dims,
		HTTP:       http.DefaultClient,
	}, nil
}

func (p *BigModelEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	if p == nil {
		return nil, errors.New("nil bigmodel embedding provider")
	}
	if p.HTTP == nil {
		p.HTTP = http.DefaultClient
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("empty text")
	}

	endpoint := p.BaseURL + "/api/paas/v4/embeddings"

	body, err := json.Marshal(map[string]any{
		"model":      p.Model,
		"input":      text,
		"dimensions": p.Dimensions,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	ctx2, cancel := context.WithTimeout(req.Context(), 15*time.Second)
	defer cancel()
	req = req.WithContext(ctx2)

	res, err := p.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return nil, fmt.Errorf("bigmodel embeddings failed: status=%d body=%q", res.StatusCode, string(b))
	}

	var decoded struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return nil, errors.New("empty embedding")
	}
	out := make([]float64, len(decoded.Data[0].Embedding))
	copy(out, decoded.Data[0].Embedding)
	return out, nil
}
