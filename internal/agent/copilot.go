package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
)

type Copilot struct{}

func (c *Copilot) Name() string { return "github-copilot" }

func (c *Copilot) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".github"))
	return err == nil
}

func (c *Copilot) ProjectSkillDir() string { return ".agents/skills" }

func (c *Copilot) SupportsCommand() bool { return false }

func (c *Copilot) InstallSkill(projectDir, srcDir, skillName string) error {
	dest := filepath.Join(projectDir, c.ProjectSkillDir(), skillName)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	os.RemoveAll(dest)
	if err := copyDir(srcDir, dest); err != nil {
		return err
	}
	fmt.Printf("  [github-copilot] Installed skill %s\n", skillName)
	return nil
}

// InstallRule merges rules into .github/copilot-instructions.md.
func (c *Copilot) InstallRule(projectDir string, rule asset.RuleData) error {
	dir := filepath.Join(projectDir, ".github")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(dir, "copilot-instructions.md")
	content := buildManagedContent([]ruleEntry{{
		Name: rule.Name, Content: rule.Content, Globs: rule.Globs, AlwaysApply: rule.AlwaysApply,
	}})
	if err := writeManagedSection(dest, content); err != nil {
		return err
	}
	fmt.Printf("  [github-copilot] Installed rule %s\n", rule.Name)
	return nil
}

// InstallMCP is a no-op for Copilot (MCP not supported).
func (c *Copilot) InstallMCP(projectDir string, mcp asset.MCPData) error {
	return nil
}

// InstallCommand is a no-op for Copilot (commands not supported).
func (c *Copilot) InstallCommand(projectDir string, cmd asset.CommandData) error {
	return nil
}
