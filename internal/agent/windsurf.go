package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// Windsurf implements the Agent interface for Windsurf IDE.
type Windsurf struct{}

func (w *Windsurf) Name() string { return "windsurf" }

func (w *Windsurf) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".windsurf"))
	return err == nil
}

func (w *Windsurf) ProjectSkillDir() string { return ".windsurf/skills" }

func (w *Windsurf) InstallSkill(srcDir, skillName string) error {
	dest := filepath.Join(w.ProjectSkillDir(), skillName)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	os.RemoveAll(dest)
	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		return err
	}
	if err := os.Symlink(absSrc, dest); err != nil {
		return copyDir(absSrc, dest)
	}
	fmt.Printf("  [windsurf] Installed skill %s -> %s\n", skillName, dest)
	return nil
}
