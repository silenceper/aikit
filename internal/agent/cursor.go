package agent

import "path/filepath"

type Cursor struct{}

func (Cursor) Name() string { return "cursor" }
func (Cursor) GlobalSkillDir(home string) string {
	return filepath.Join(home, ".cursor", "skills")
}
func (Cursor) ProjectSkillDir(project string) string {
	return filepath.Join(project, ".cursor", "skills")
}
