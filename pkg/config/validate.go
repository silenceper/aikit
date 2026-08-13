package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var hexObjectID = regexp.MustCompile(`^[0-9a-fA-F]+$`)

var validAgents = map[string]struct{}{
	"cursor": {}, "claude-code": {}, "codex": {}, "copilot": {}, "windsurf": {},
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	skills := make(map[string]Skill, len(c.Library.Skills))
	for i, skill := range c.Library.Skills {
		if err := validateSkill(skill); err != nil {
			return fmt.Errorf("library.skills[%d]: %w", i, err)
		}
		if _, exists := skills[skill.ID]; exists {
			return fmt.Errorf("duplicate skill id %q", skill.ID)
		}
		skills[skill.ID] = skill
	}

	presets := make(map[string]struct{}, len(c.Presets))
	for i, preset := range c.Presets {
		if err := validateSimpleName(preset.Name, "preset name"); err != nil {
			return fmt.Errorf("presets[%d]: %w", i, err)
		}
		if _, exists := presets[preset.Name]; exists {
			return fmt.Errorf("duplicate preset name %q", preset.Name)
		}
		presets[preset.Name] = struct{}{}
	}
	for i, preset := range c.Presets {
		if err := validateSkillRefs(preset.Skills, skills); err != nil {
			return fmt.Errorf("preset %q: %w", c.Presets[i].Name, err)
		}
	}

	for name, binding := range c.Agents {
		if !isValidAgent(name) {
			return fmt.Errorf("unknown agent %q", name)
		}
		if err := validateBinding(binding, skills, presets); err != nil {
			return fmt.Errorf("agent %q: %w", name, err)
		}
	}

	projectNames := make(map[string]struct{}, len(c.Projects))
	projectPaths := make(map[string]string, len(c.Projects))
	for i, project := range c.Projects {
		if err := validateSimpleName(project.Name, "project name"); err != nil {
			return fmt.Errorf("projects[%d]: %w", i, err)
		}
		if _, exists := projectNames[project.Name]; exists {
			return fmt.Errorf("duplicate project name %q", project.Name)
		}
		projectNames[project.Name] = struct{}{}
		canonical, err := canonicalProjectPath(project.Path)
		if err != nil {
			return fmt.Errorf("project %q: %w", project.Name, err)
		}
		if previous, exists := projectPaths[canonical]; exists {
			return fmt.Errorf("projects %q and %q have the same canonical path %q", previous, project.Name, canonical)
		}
		projectPaths[canonical] = project.Name

		declared := make(map[string]struct{}, len(project.Agents))
		for _, name := range project.Agents {
			if !isValidAgent(name) {
				return fmt.Errorf("project %q: unknown agent %q", project.Name, name)
			}
			if _, exists := declared[name]; exists {
				return fmt.Errorf("project %q: duplicate agent %q", project.Name, name)
			}
			declared[name] = struct{}{}
		}
		if err := validateBinding(project.Binding, skills, presets); err != nil {
			return fmt.Errorf("project %q common binding: %w", project.Name, err)
		}
		for name, binding := range project.AgentBindings {
			if _, exists := declared[name]; !exists {
				return fmt.Errorf("project %q: agent binding %q is not declared", project.Name, name)
			}
			if err := validateBinding(binding, skills, presets); err != nil {
				return fmt.Errorf("project %q agent %q: %w", project.Name, name, err)
			}
		}
	}

	operationIDs := make(map[string]struct{}, len(c.PendingOperations))
	for i, operation := range c.PendingOperations {
		if err := c.validateOperation(operation); err != nil {
			return fmt.Errorf("pending_operations[%d]: %w", i, err)
		}
		if _, exists := operationIDs[operation.ID]; exists {
			return fmt.Errorf("duplicate pending operation id %q", operation.ID)
		}
		operationIDs[operation.ID] = struct{}{}
	}
	return nil
}

func validateSkill(skill Skill) error {
	if !safeRelativeSlashPath(skill.ID) {
		return fmt.Errorf("unsafe id %q", skill.ID)
	}
	if err := validateSimpleName(skill.Name, "skill name"); err != nil {
		return err
	}
	if skill.SourcePath != "" && !safeRepositoryPath(skill.SourcePath) {
		return fmt.Errorf("unsafe source_path %q", skill.SourcePath)
	}
	if skill.Source != "" && skill.SourcePath == "" {
		return fmt.Errorf("remote skill source_path is required; use . for repository root")
	}
	if skill.Ref != nil {
		switch skill.Ref.Kind {
		case "branch", "tag", "commit":
		default:
			return fmt.Errorf("invalid ref kind %q", skill.Ref.Kind)
		}
		if skill.Ref.Value == "" {
			return fmt.Errorf("ref value is empty")
		}
	}
	if skill.Source != "" && skill.Ref == nil {
		return fmt.Errorf("remote skill ref is required")
	}
	if skill.Resolved != "" && ((len(skill.Resolved) != 40 && len(skill.Resolved) != 64) || !hexObjectID.MatchString(skill.Resolved)) {
		return fmt.Errorf("resolved must be a full hexadecimal object id")
	}
	if skill.Source != "" && skill.Resolved == "" {
		return fmt.Errorf("remote skill resolved object id is required")
	}
	return nil
}

func validateBinding(binding Binding, skills map[string]Skill, presets map[string]struct{}) error {
	if err := validateSkillRefs(binding.Skills, skills); err != nil {
		return err
	}
	for _, name := range binding.Presets {
		if _, exists := presets[name]; !exists {
			return fmt.Errorf("unknown preset %q", name)
		}
	}
	return nil
}

func validateSkillRefs(ids []string, skills map[string]Skill) error {
	for _, id := range ids {
		if _, exists := skills[id]; !exists {
			return fmt.Errorf("unknown skill %q", id)
		}
	}
	return nil
}

func (c *Config) validateOperation(operation PendingOperation) error {
	if operation.ID == "" {
		return fmt.Errorf("id is empty")
	}
	if operation.Kind != OperationCleanup && operation.Kind != OperationAdopt {
		return fmt.Errorf("invalid kind %q", operation.Kind)
	}
	if !cleanAbsolute(operation.Target) {
		return fmt.Errorf("target must be absolute")
	}
	if !safeRelativeSlashPath(operation.SkillID) {
		return fmt.Errorf("unsafe skill id %q", operation.SkillID)
	}
	if !isValidAgent(operation.Scope.Agent) {
		return fmt.Errorf("unknown scope agent %q", operation.Scope.Agent)
	}
	expectedRoot, err := c.operationSkillRoot(operation.Scope)
	if err != nil {
		return err
	}
	targetParent, err := canonicalProjectPath(filepath.Dir(operation.Target))
	if err != nil {
		return fmt.Errorf("resolve target parent: %w", err)
	}
	expectedRoot, err = canonicalProjectPath(expectedRoot)
	if err != nil {
		return fmt.Errorf("resolve scope skill root: %w", err)
	}
	if targetParent != expectedRoot {
		return fmt.Errorf("target %q is outside scope skill root %q", operation.Target, expectedRoot)
	}
	if err := validateSimpleName(filepath.Base(operation.Target), "target name"); err != nil {
		return err
	}
	if operation.Kind == OperationAdopt {
		if !cleanAbsolute(operation.Temp) || !cleanAbsolute(operation.Backup) {
			return fmt.Errorf("adopt temp and backup must be absolute")
		}
		if filepath.Dir(operation.Temp) != filepath.Dir(operation.Target) || filepath.Dir(operation.Backup) != filepath.Dir(operation.Target) {
			return fmt.Errorf("adopt temp and backup must share the target parent")
		}
		if !strings.HasPrefix(filepath.Base(operation.Temp), ".aikit-adopt-temp-") || !strings.HasPrefix(filepath.Base(operation.Backup), ".aikit-adopt-backup-") {
			return fmt.Errorf("adopt temp and backup must use reserved aikit names")
		}
		if operation.Original == nil || operation.Original.Kind == "" {
			return fmt.Errorf("adopt original fingerprint is required")
		}
		if len(operation.JournalHash) != 64 || !hexObjectID.MatchString(operation.JournalHash) {
			return fmt.Errorf("adopt journal_hash must be a full sha256 digest")
		}
	}
	return nil
}

func (c *Config) operationSkillRoot(scope Scope) (string, error) {
	var base string
	if scope.Project == "" {
		if scope.ProjectPath != "" {
			return "", fmt.Errorf("global scope cannot have project_path")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		base = home
	} else {
		base = scope.ProjectPath
		for _, project := range c.Projects {
			if project.Name != scope.Project {
				continue
			}
			if base != "" {
				given, err := canonicalProjectPath(base)
				if err != nil {
					return "", err
				}
				registered, err := canonicalProjectPath(project.Path)
				if err != nil {
					return "", err
				}
				if given != registered {
					return "", fmt.Errorf("scope project_path does not match project %q", scope.Project)
				}
			}
			base = project.Path
			break
		}
		if base == "" || !filepath.IsAbs(base) {
			return "", fmt.Errorf("project scope requires an absolute project_path")
		}
	}

	parts, ok := agentSkillPath(scope.Agent, scope.Project != "")
	if !ok {
		return "", fmt.Errorf("unknown scope agent %q", scope.Agent)
	}
	return filepath.Join(append([]string{base}, parts...)...), nil
}

func agentSkillPath(name string, project bool) ([]string, bool) {
	if project {
		switch name {
		case "cursor":
			return []string{".cursor", "skills"}, true
		case "claude-code":
			return []string{".claude", "skills"}, true
		case "codex":
			return []string{".codex", "skills"}, true
		case "copilot":
			return []string{".agents", "skills"}, true
		case "windsurf":
			return []string{".windsurf", "skills"}, true
		}
	} else {
		switch name {
		case "cursor":
			return []string{".cursor", "skills"}, true
		case "claude-code":
			return []string{".claude", "skills"}, true
		case "codex":
			return []string{".codex", "skills"}, true
		case "copilot":
			return []string{".copilot", "skills"}, true
		case "windsurf":
			return []string{".codeium", "windsurf", "skills"}, true
		}
	}
	return nil, false
}

func cleanAbsolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

func validateSimpleName(name, label string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("invalid %s %q", label, name)
	}
	return nil
}

func safeRelativeSlashPath(path string) bool {
	if path == "" || strings.ContainsRune(path, '\x00') || strings.Contains(path, "\\") || strings.HasPrefix(path, "/") {
		return false
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) == path
}

func safeRepositoryPath(path string) bool {
	if path == "." {
		return true
	}
	return safeRelativeSlashPath(path)
}

func isValidAgent(name string) bool {
	_, ok := validAgents[name]
	return ok
}
