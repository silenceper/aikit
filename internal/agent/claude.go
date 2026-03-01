package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
)

type ClaudeCode struct{}

func (c *ClaudeCode) Name() string { return "claude-code" }

func (c *ClaudeCode) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".claude"))
	return err == nil
}

func (c *ClaudeCode) ProjectSkillDir() string { return ".claude/skills" }

func (c *ClaudeCode) SupportsCommand() bool { return true }

func (c *ClaudeCode) InstallSkill(projectDir, srcDir, skillName string) error {
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
	fmt.Printf("  [claude-code] Installed skill %s\n", skillName)
	return nil
}

// InstallRule merges rules into CLAUDE.md using managed section markers.
func (c *ClaudeCode) InstallRule(projectDir string, rule asset.RuleData) error {
	// Claude Code uses a single CLAUDE.md; we accumulate via managed section.
	// For single-rule install, we write the managed section.
	claudeFile := filepath.Join(projectDir, "CLAUDE.md")
	content := buildManagedContent([]ruleEntry{{
		Name: rule.Name, Content: rule.Content, Globs: rule.Globs, AlwaysApply: rule.AlwaysApply,
	}})
	if err := writeManagedSection(claudeFile, content); err != nil {
		return err
	}
	fmt.Printf("  [claude-code] Installed rule %s -> CLAUDE.md\n", rule.Name)
	return nil
}

// InstallMCP merges MCP config into .mcp.json at project root.
func (c *ClaudeCode) InstallMCP(projectDir string, mcp asset.MCPData) error {
	mcpFile := filepath.Join(projectDir, ".mcp.json")
	var mcpConfig map[string]any
	if data, err := os.ReadFile(mcpFile); err == nil {
		_ = json.Unmarshal(data, &mcpConfig)
	}
	if mcpConfig == nil {
		mcpConfig = map[string]any{}
	}
	servers, ok := mcpConfig["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	entry := map[string]any{
		"command": mcp.Command,
		"args":    mcp.Args,
	}
	if mcp.Transport != "" && mcp.Transport != "stdio" {
		entry["transport"] = mcp.Transport
	}
	if len(mcp.Env) > 0 {
		entry["env"] = mcp.Env
	}
	servers[mcp.Name] = entry
	mcpConfig["mcpServers"] = servers

	data, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(mcpFile, data, 0644); err != nil {
		return err
	}
	fmt.Printf("  [claude-code] Installed MCP %s\n", mcp.Name)
	return nil
}

// InstallCommand writes a command to .claude/commands/<name>.md.
func (c *ClaudeCode) InstallCommand(projectDir string, cmd asset.CommandData) error {
	dir := filepath.Join(projectDir, ".claude", "commands")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(dir, cmd.Name+".md")
	if err := os.WriteFile(dest, []byte(cmd.Content), 0644); err != nil {
		return err
	}
	fmt.Printf("  [claude-code] Installed command %s\n", cmd.Name)
	return nil
}
