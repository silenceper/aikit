package agent

type Agent interface {
	Name() string
	GlobalSkillDir(home string) string
	ProjectSkillDir(project string) string
}

func All() []Agent {
	return []Agent{
		Cursor{},
		ClaudeCode{},
		Codex{},
		Copilot{},
		Windsurf{},
	}
}

func Names() []string {
	agents := All()
	names := make([]string, len(agents))
	for i, item := range agents {
		names[i] = item.Name()
	}
	return names
}

func NormalizeLegacyName(name string) string {
	if name == "github-copilot" {
		return "copilot"
	}
	return name
}

func ByName(name string) (Agent, bool) {
	name = NormalizeLegacyName(name)
	for _, item := range All() {
		if item.Name() == name {
			return item, true
		}
	}
	return nil, false
}
