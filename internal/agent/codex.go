package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/asset"
)

type Codex struct{}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) Detect(projectDir string) bool {
	for _, name := range []string{".codex", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
			return true
		}
	}
	return false
}

func (c *Codex) ProjectSkillDir() string { return ".codex/skills" }

func (c *Codex) SupportsCommand() bool { return false }

func (c *Codex) InstallSkill(projectDir, srcDir, skillName string) error {
	dest := filepath.Join(projectDir, c.ProjectSkillDir(), skillName)
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
	fmt.Printf("  [codex] Installed skill %s\n", skillName)
	return nil
}

// InstallRule merges rules into AGENTS.md using managed section markers.
func (c *Codex) InstallRule(projectDir string, rule asset.RuleData) error {
	agentsFile := filepath.Join(projectDir, "AGENTS.md")
	content := buildManagedContent([]ruleEntry{{
		Name: rule.Name, Content: rule.Content, Globs: rule.Globs, AlwaysApply: rule.AlwaysApply,
	}})
	if err := writeManagedSection(agentsFile, content); err != nil {
		return err
	}
	fmt.Printf("  [codex] Installed rule %s -> AGENTS.md\n", rule.Name)
	return nil
}

// InstallMCP appends MCP server config to .codex/config.toml.
func (c *Codex) InstallMCP(projectDir string, mcp asset.MCPData) error {
	codexDir := filepath.Join(projectDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		return err
	}
	configFile := filepath.Join(codexDir, "config.toml")

	existing, _ := os.ReadFile(configFile)
	body := string(existing)

	sectionHeader := fmt.Sprintf("[mcp_servers.%s]", mcp.Name)
	if strings.Contains(body, sectionHeader) {
		fmt.Printf("  [codex] MCP %s already in config.toml, skipped\n", mcp.Name)
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n", sectionHeader))
	if mcp.Command != "" {
		sb.WriteString(fmt.Sprintf("command = %q\n", mcp.Command))
	}
	if len(mcp.Args) > 0 {
		quoted := make([]string, len(mcp.Args))
		for i, a := range mcp.Args {
			quoted[i] = fmt.Sprintf("%q", a)
		}
		sb.WriteString(fmt.Sprintf("args = [%s]\n", strings.Join(quoted, ", ")))
	}
	if len(mcp.Env) > 0 {
		sb.WriteString("[mcp_servers." + mcp.Name + ".env]\n")
		for k, v := range mcp.Env {
			sb.WriteString(fmt.Sprintf("%s = %q\n", k, v))
		}
	}

	body += sb.String()
	if err := os.WriteFile(configFile, []byte(body), 0644); err != nil {
		return err
	}
	fmt.Printf("  [codex] Installed MCP %s -> .codex/config.toml\n", mcp.Name)
	return nil
}

func (c *Codex) InstallCommand(projectDir string, cmd asset.CommandData) error {
	return nil
}
