package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeCode implements the Agent interface for Claude Code.
type ClaudeCode struct{}

func (c *ClaudeCode) Name() string { return "claude-code" }

func (c *ClaudeCode) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".claude"))
	return err == nil
}

func (c *ClaudeCode) ProjectSkillDir() string { return ".claude/skills" }

func (c *ClaudeCode) InstallSkill(srcDir, skillName string) error {
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
	fmt.Printf("  [claude-code] Installed skill %s -> %s\n", skillName, dest)
	return nil
}
