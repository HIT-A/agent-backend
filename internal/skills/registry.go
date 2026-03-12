package skills

// Registry stores available skills.
// Minimal implementation: in-memory list.
type Registry struct {
	skills []Skill
}

func NewRegistry() *Registry {
	// Register at least one built-in skill.
	return &Registry{skills: []Skill{{Name: "placeholder"}}}
}

func (r *Registry) List() []Skill {
	// Return a copy to keep registry immutable from callers.
	out := make([]Skill, len(r.skills))
	copy(out, r.skills)
	return out
}
