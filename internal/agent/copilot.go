package agent

import "path/filepath"

type Copilot struct{}

func (Copilot) Name() string { return "copilot" }
func (Copilot) GlobalSkillDir(home string) string {
	return filepath.Join(home, ".copilot", "skills")
}
func (Copilot) ProjectSkillDir(project string) string {
	return filepath.Join(project, ".agents", "skills")
}
