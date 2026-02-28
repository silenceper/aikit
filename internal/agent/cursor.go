package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// Cursor implements the Agent interface for Cursor IDE.
type Cursor struct{}

func (c *Cursor) Name() string { return "cursor" }

func (c *Cursor) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".cursor"))
	return err == nil
}

func (c *Cursor) ProjectSkillDir() string { return ".cursor/skills" }

// InstallSkill copies or symlinks a skill directory into the project's .cursor/skills/.
func (c *Cursor) InstallSkill(srcDir, skillName string) error {
	dest := filepath.Join(c.ProjectSkillDir(), skillName)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	// Remove existing
	os.RemoveAll(dest)
	// Symlink
	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		return err
	}
	if err := os.Symlink(absSrc, dest); err != nil {
		// Fallback to copy
		return copyDir(absSrc, dest)
	}
	fmt.Printf("  [cursor] Installed skill %s -> %s\n", skillName, dest)
	return nil
}
