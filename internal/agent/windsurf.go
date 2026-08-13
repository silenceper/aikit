package agent

import "path/filepath"

type Windsurf struct{}

func (Windsurf) Name() string { return "windsurf" }
func (Windsurf) GlobalSkillDir(home string) string {
	return filepath.Join(home, ".codeium", "windsurf", "skills")
}
func (Windsurf) ProjectSkillDir(project string) string {
	return filepath.Join(project, ".windsurf", "skills")
}
