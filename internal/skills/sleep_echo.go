package skills

import (
	"context"
	"time"
)

// SleepEcho is a demo async skill used to exercise the async path.
// It sleeps briefly (configurable via input.sleep_ms) then echoes input/trace.
func NewSleepEchoSkill() Skill {
	return Skill{
		Name:    "sleep_echo",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			sleepMS := 10
			if v, ok := input["sleep_ms"]; ok {
				if f, ok := v.(float64); ok {
					sleepMS = int(f)
				}
			}
			t := time.NewTimer(time.Duration(sleepMS) * time.Millisecond)
			select {
			case <-ctx.Done():
				if !t.Stop() {
					<-t.C
				}
				return nil, ctx.Err()
			case <-t.C:
			}

			out := map[string]any{"input": input}
			if trace != nil {
				out["trace"] = trace
			}
			return out, nil
		},
	}
}
