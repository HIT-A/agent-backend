package skills

// Registry stores available skills.
// Minimal implementation: in-memory list.
type Registry struct {
	skills []Skill
	index  map[string]Skill
}

func NewRegistry() *Registry {
	r := &Registry{}
	r.Register(NewEchoSkill())
	r.Register(NewSleepEchoSkill())
	r.Register(NewRAGQuerySkill())
	return r
}

func (r *Registry) Register(s Skill) {
	if r.index == nil {
		r.index = make(map[string]Skill)
	}
	// Keep deterministic list order: append in registration order.
	r.skills = append(r.skills, s)
	r.index[s.Name] = s
}

func (r *Registry) List() []Skill {
	// Return a copy to keep registry immutable from callers.
	//
	// NOTE: We intentionally exclude Invoke from the returned slice so callers can
	// safely compare and/or serialize the list.
	out := make([]Skill, len(r.skills))
	for i := range r.skills {
		out[i] = Skill{Name: r.skills[i].Name, IsAsync: r.skills[i].IsAsync}
	}
	return out
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r.index == nil {
		return Skill{}, false
	}
	s, ok := r.index[name]
	return s, ok
}
