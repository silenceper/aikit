package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// Copilot implements the Agent interface for GitHub Copilot.
type Copilot struct{}

func (c *Copilot) Name() string { return "github-copilot" }

func (c *Copilot) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".github"))
	return err == nil
}

func (c *Copilot) ProjectSkillDir() string { return ".agents/skills" }

func (c *Copilot) InstallSkill(srcDir, skillName string) error {
	dest := filepath.Join(c.ProjectSkillDir(), skillName)
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
	fmt.Printf("  [github-copilot] Installed skill %s -> %s\n", skillName, dest)
	return nil
}
