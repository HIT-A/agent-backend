package httpserver

import "context"

// StartWorkerPool starts n workers consuming job IDs from queue.
//
// NOTE: Stub for TDD; implementation added in subsequent commit.
func StartWorkerPool(ctx context.Context, n int, queue <-chan string, store JobStore) {
	_ = ctx
	_ = n
	_ = queue
	_ = store
}
