package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/asset"
)

type Cursor struct{}

func (c *Cursor) Name() string { return "cursor" }

func (c *Cursor) Detect(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, ".cursor"))
	return err == nil
}

func (c *Cursor) ProjectSkillDir() string { return ".cursor/skills" }

func (c *Cursor) SupportsCommand() bool { return true }

func (c *Cursor) InstallSkill(projectDir, srcDir, skillName string) error {
	dest := filepath.Join(projectDir, c.ProjectSkillDir(), skillName)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	os.RemoveAll(dest)
	if err := copyDir(srcDir, dest); err != nil {
		return err
	}
	fmt.Printf("  [cursor] Installed skill %s\n", skillName)
	return nil
}

// InstallRule writes a .mdc file to .cursor/rules/<name>.mdc.
func (c *Cursor) InstallRule(projectDir string, rule asset.RuleData) error {
	dir := filepath.Join(projectDir, ".cursor", "rules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(dir, rule.Name+".mdc")

	var sb strings.Builder
	sb.WriteString("---\n")
	if rule.AlwaysApply {
		sb.WriteString("alwaysApply: true\n")
	} else if len(rule.Globs) > 0 {
		sb.WriteString(fmt.Sprintf("globs: %s\n", strings.Join(rule.Globs, ", ")))
	}
	sb.WriteString("---\n")
	sb.WriteString(rule.Content)

	if err := os.WriteFile(dest, []byte(sb.String()), 0644); err != nil {
		return err
	}
	fmt.Printf("  [cursor] Installed rule %s -> %s\n", rule.Name, dest)
	return nil
}

// InstallMCP merges MCP config into .cursor/mcp.json.
func (c *Cursor) InstallMCP(projectDir string, mcp asset.MCPData) error {
	mcpFile := filepath.Join(projectDir, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpFile), 0755); err != nil {
		return err
	}

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
	if mcp.ServerInstructions != "" {
		entry["serverInstructions"] = mcp.ServerInstructions
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
	fmt.Printf("  [cursor] Installed MCP %s\n", mcp.Name)
	return nil
}

// InstallCommand writes a command to .cursor/commands/<name>.md.
func (c *Cursor) InstallCommand(projectDir string, cmd asset.CommandData) error {
	dir := filepath.Join(projectDir, ".cursor", "commands")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(dir, cmd.Name+".md")
	if err := os.WriteFile(dest, []byte(cmd.Content), 0644); err != nil {
		return err
	}
	fmt.Printf("  [cursor] Installed command %s\n", cmd.Name)
	return nil
}
