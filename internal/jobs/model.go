package jobs

import (
	"encoding/json"
	"errors"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

var ErrNotFound = errors.New("job not found")

type Job struct {
	ID         string          `json:"id"`
	Status     Status          `json:"status"`
	SkillName  string          `json:"skill_name"`
	InputJSON  json.RawMessage `json:"input_json"`
	OutputJSON json.RawMessage `json:"output_json"`
	Error      string          `json:"error"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
