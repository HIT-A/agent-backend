package skills

// Skill is a minimal representation of a callable capability exposed by the API.
// Keep this small for now; it can be extended later as requirements evolve.
type Skill struct {
	Name string `json:"name"`
}
