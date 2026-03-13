package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

					job, err := store.ClaimRunning(ctx, id)
					if err != nil {
						// Another worker likely claimed it first.
						if errors.Is(err, jobs.ErrNotClaimable) {
							continue
						}
						continue
					}

					var payload struct {
						Input map[string]any `json:"input"`
						Trace map[string]any `json:"trace"`
					}
					if err := json.Unmarshal(job.InputJSON, &payload); err != nil {
						updateFinalStatus(id, store, jobs.StatusFailed, nil, fmt.Sprintf("invalid input_json: %v", err))
						continue
					}

					skill, ok := reg.Get(job.SkillName)
					if !ok {
						updateFinalStatus(id, store, jobs.StatusFailed, nil, fmt.Sprintf("unknown skill: %s", job.SkillName))
						continue
					}

					var (
						out      map[string]any
						invokeOK bool
					)
					func() {
						defer func() {
							if r := recover(); r != nil {
								updateFinalStatus(id, store, jobs.StatusFailed, nil, fmt.Sprintf("panic: %v", r))
								invokeOK = false
							}
						}()

						result, err := skill.Invoke(ctx, payload.Input, payload.Trace)
						if err != nil {
							updateFinalStatus(id, store, jobs.StatusFailed, nil, err.Error())
							invokeOK = false
							return
						}
						out = result
						invokeOK = true
					}()

					if !invokeOK {
						// status was already updated in error/panic path
						continue
					}

					outB, err := json.Marshal(out)
					if err != nil {
						updateFinalStatus(id, store, jobs.StatusFailed, nil, fmt.Sprintf("marshal output: %v", err))
						continue
					}

					updateFinalStatus(id, store, jobs.StatusSucceeded, json.RawMessage(outB), "")
				}
			}
		}()
	}
}

func updateFinalStatus(id string, store JobStore, status jobs.Status, output json.RawMessage, errMsg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = store.UpdateStatus(ctx, id, status, output, errMsg)
}
