package agent

import "path/filepath"

type Codex struct{}

func (Codex) Name() string { return "codex" }
func (Codex) GlobalSkillDir(home string) string {
	return filepath.Join(home, ".codex", "skills")
}
func (Codex) ProjectSkillDir(project string) string {
	return filepath.Join(project, ".codex", "skills")
}
