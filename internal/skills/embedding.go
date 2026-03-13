package skills

import (
	"context"
	"errors"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
)

// EmbeddingProvider converts an input string into an embedding vector.
//
// For v0 we keep it small and allow environment-based configuration.
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
	default:
		return nil, errors.New("unknown EMBEDDING_PROVIDER")
	}
}

// DeterministicEmbeddingProvider is a tiny fallback used when no provider is configured.
// It is NOT meant to be high quality; it is only meant to be stable.
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
