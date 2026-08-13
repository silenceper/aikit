package agent

import "path/filepath"

type ClaudeCode struct{}

func (ClaudeCode) Name() string { return "claude-code" }
func (ClaudeCode) GlobalSkillDir(home string) string {
	return filepath.Join(home, ".claude", "skills")
}
func (ClaudeCode) ProjectSkillDir(project string) string {
	return filepath.Join(project, ".claude", "skills")
}
