package agent

// Agent defines the interface that each IDE adapter must implement.
type Agent interface {
	Name() string
	Detect(projectDir string) bool
	ProjectSkillDir() string
	InstallSkill(srcDir, skillName string) error
}

// All returns all known agent adapters.
func All() []Agent {
	return []Agent{
		&Cursor{},
		&ClaudeCode{},
		&Copilot{},
		&Windsurf{},
	}
}

// ByName returns agents matching the given names.
func ByName(names []string) []Agent {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	var out []Agent
	for _, a := range All() {
		if set[a.Name()] {
			out = append(out, a)
		}
	}
	return out
}
