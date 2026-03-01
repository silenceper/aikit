package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
)

type Windsurf struct{}

func (w *Windsurf) Name() string { return "windsurf" }

func (w *Windsurf) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".windsurf"))
	return err == nil
}

func (w *Windsurf) ProjectSkillDir() string { return ".windsurf/skills" }

func (w *Windsurf) SupportsCommand() bool { return false }

func (w *Windsurf) InstallSkill(projectDir, srcDir, skillName string) error {
	dest := filepath.Join(projectDir, w.ProjectSkillDir(), skillName)
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
	fmt.Printf("  [windsurf] Installed skill %s\n", skillName)
	return nil
}

// InstallRule merges rules into .windsurfrules or .windsurf/rules/.
func (w *Windsurf) InstallRule(projectDir string, rule asset.RuleData) error {
	dir := filepath.Join(projectDir, ".windsurf", "rules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(dir, rule.Name+".md")
	if err := os.WriteFile(dest, []byte(rule.Content), 0644); err != nil {
		return err
	}
	fmt.Printf("  [windsurf] Installed rule %s\n", rule.Name)
	return nil
}

// InstallMCP is a no-op for Windsurf (no standard MCP config).
func (w *Windsurf) InstallMCP(projectDir string, mcp asset.MCPData) error {
	return nil
}

// InstallCommand is a no-op for Windsurf (commands not supported).
func (w *Windsurf) InstallCommand(projectDir string, cmd asset.CommandData) error {
	return nil
}
