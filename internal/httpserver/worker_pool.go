package httpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"hoa-agent-backend/internal/jobs"
	"hoa-agent-backend/internal/skills"
)

// StartWorkerPool starts n workers consuming job IDs from queue.
//
// Workers will:
// - mark job running
// - execute the skill handler
// - mark succeeded/failed with output_json/error
func StartWorkerPool(ctx context.Context, n int, queue <-chan string, store JobStore) {
	if n <= 0 {
		n = 1
	}
	if store == nil {
		panic("httpserver: nil JobStore")
	}

	for i := 0; i < n; i++ {
		go func() {
			reg := skills.NewRegistry()
			for {
				select {
				case <-ctx.Done():
					return
				case id, ok := <-queue:
					if !ok {
						return
					}

					job, err := store.Get(ctx, id)
					if err != nil {
						continue
					}
					if job.Status != jobs.StatusQueued {
						continue
					}

					_, _ = store.UpdateStatus(ctx, id, jobs.StatusRunning, nil, "")

					var payload struct {
						Input map[string]any `json:"input"`
						Trace map[string]any `json:"trace"`
					}
					if err := json.Unmarshal(job.InputJSON, &payload); err != nil {
						_, _ = store.UpdateStatus(ctx, id, jobs.StatusFailed, nil, fmt.Sprintf("invalid input_json: %v", err))
						continue
					}

					skill, ok := reg.Get(job.SkillName)
					if !ok {
						_, _ = store.UpdateStatus(ctx, id, jobs.StatusFailed, nil, fmt.Sprintf("unknown skill: %s", job.SkillName))
						continue
					}

					out, err := skill.Invoke(ctx, payload.Input, payload.Trace)
					if err != nil {
						_, _ = store.UpdateStatus(ctx, id, jobs.StatusFailed, nil, err.Error())
						continue
					}

					outB, err := json.Marshal(out)
					if err != nil {
						_, _ = store.UpdateStatus(ctx, id, jobs.StatusFailed, nil, fmt.Sprintf("marshal output: %v", err))
						continue
					}

					_, _ = store.UpdateStatus(ctx, id, jobs.StatusSucceeded, json.RawMessage(outB), "")
				}
			}
		}()
	}
}
