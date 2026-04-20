package syncknowledge

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"hoa-agent-backend/internal/skills"
)

func StartAutoSync(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}

	repo := strings.TrimSpace(os.Getenv("RAGDATA_REPO"))
	if repo == "" {
		repo = "HIT-A/HITA_RagData"
	}
	branch := strings.TrimSpace(os.Getenv("RAGDATA_BRANCH"))
	if branch == "" {
		branch = "main"
	}
	localPath := strings.TrimSpace(os.Getenv("RAGDATA_SYNC_STATE_PATH"))
	if localPath == "" {
		localPath = "./data/sync_state"
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("[RAG AutoSync] Starting background sync for %s/%s every %v", repo, branch, interval)

		for {
			select {
			case <-ctx.Done():
				log.Printf("[RAG AutoSync] Shutting down")
				return
			case <-ticker.C:
				if err := doSync(ctx, repo, branch, localPath); err != nil {
					log.Printf("[RAG AutoSync] Sync failed: %v", err)
				}
			}
		}
	}()
}

func doSync(ctx context.Context, repo, branch, localPath string) error {
	start := time.Now()

	qdrant, err := skills.NewQdrantClientFromEnv()
	if err != nil {
		return fmt.Errorf("qdrant init: %w", err)
	}

	embedder, err := skills.NewEmbeddingProviderFromEnv()
	if err != nil {
		return fmt.Errorf("embedder init: %w", err)
	}

	qdrantWrapper := &qdrantAdapter{client: qdrant}
	embedderWrapper := &embedderAdapter{provider: embedder}

	syncer := NewGitHubSync(repo, branch, localPath, qdrantWrapper, embedderWrapper)
	if err := syncer.LoadState(); err != nil {
		log.Printf("[RAG AutoSync] No previous state, starting fresh: %v", err)
	}

	result, err := syncer.Sync(ctx)
	if err != nil {
		return fmt.Errorf("sync execution: %w", err)
	}

	if result.NoChange {
		log.Printf("[RAG AutoSync] No changes detected (commit=%s)", result.LatestCommit[:8])
		return nil
	}

	log.Printf("[RAG AutoSync] Sync completed in %v: commit=%s upserted=%d deleted=%d errors=%d",
		time.Since(start),
		result.LatestCommit[:8],
		len(result.Upserted),
		len(result.Deleted),
		len(result.Errors),
	)

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			log.Printf("[RAG AutoSync] Error: %s", e)
		}
	}

	return nil
}

type qdrantAdapter struct {
	client *skills.QdrantClient
}

func (a *qdrantAdapter) Upsert(ctx context.Context, point *Point) error {
	vec := make([]float64, len(point.Vector))
	for i, v := range point.Vector {
		vec[i] = float64(v)
	}
	return a.client.Upsert(ctx, a.client.Collection, []skills.QdrantPoint{
		{ID: point.ID, Vector: vec, Payload: point.Payload},
	})
}

func (a *qdrantAdapter) DeleteByFilter(ctx context.Context, filter map[string]interface{}) error {
	return a.client.DeleteByDocID(ctx, a.client.Collection, filter["match"].(map[string]interface{})["value"].(string))
}

type embedderAdapter struct {
	provider skills.EmbeddingProvider
}

func (a *embedderAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	vec, err := a.provider.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out, nil
}
