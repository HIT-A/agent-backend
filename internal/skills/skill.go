package skills

import "context"

// Skill is a minimal representation of a callable capability exposed by the API.
// Keep this small for now; it can be extended later as requirements evolve.
//
// Invoke is excluded from JSON serialization so the skills list endpoint can
// safely encode skills.
type Skill struct {
	Name   string                                                                                        `json:"name"`
	Invoke func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) `json:"-"`
}

func NewEchoSkill() Skill {
	return Skill{
		Name: "echo",
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = ctx
			out := map[string]any{"input": input}
			if trace != nil {
				out["trace"] = trace
			}
			return out, nil
		},
	}
}
