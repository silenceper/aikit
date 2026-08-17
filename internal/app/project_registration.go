package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) PreviewProjectRegistration(ctx context.Context, request ProjectRegistrationRequest) (ProjectRegistrationPreview, error) {
	path, identity, err := validateProjectDirectory(request.Path)
	if err != nil {
		return ProjectRegistrationPreview{}, err
	}
	agents, warnings, err := detectProjectAgents(path)
	if err != nil {
		return ProjectRegistrationPreview{}, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = filepath.Base(path)
	}
	preview := ProjectRegistrationPreview{
		Name:         name,
		Path:         path,
		PathIdentity: identity,
		Agents:       agents,
		Warnings:     warnings,
	}
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return ProjectRegistrationPreview{}, err
	}
	for _, project := range cfg.Projects {
		registered, _, registeredErr := validateProjectDirectory(project.Path)
		if registeredErr == nil && registered == path {
			return ProjectRegistrationPreview{}, fmt.Errorf("project path %q is already registered as %q", path, project.Name)
		}
	}
	if !validProjectName(name) {
		preview.NeedsName = true
		preview.NameIssue = ProjectNameInvalid
		return preview, nil
	}
	if projectNameConflict(cfg, name) {
		preview.NeedsName = true
		preview.NameIssue = ProjectNameDuplicate
		return preview, nil
	}
	plan, err := a.previewProjectEditWithConfig(cfg, ProjectEditRequest{
		Name:                 name,
		Path:                 path,
		AddAgents:            agents,
		ExpectedPathIdentity: identity,
	})
	if err != nil {
		return ProjectRegistrationPreview{}, err
	}
	preview.Preview = plan
	return preview, nil
}

func validateProjectDirectory(path string) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("project path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("project path must be an existing directory: %q", abs)
		}
		return "", "", fmt.Errorf("resolve project path %q: %w", abs, err)
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("inspect project path %q: %w", resolved, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("project path must be a directory: %q", resolved)
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return "", "", fmt.Errorf("open project directory %q: %w", resolved, err)
	}
	if err := root.Close(); err != nil {
		return "", "", fmt.Errorf("close project directory %q: %w", resolved, err)
	}
	identity, err := projectPathIdentity(resolved)
	if err != nil {
		return "", "", fmt.Errorf("identify project directory %q: %w", resolved, err)
	}
	return resolved, identity, nil
}

func detectProjectAgents(projectPath string) ([]string, []string, error) {
	root, err := os.OpenRoot(projectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open project directory %q: %w", projectPath, err)
	}
	defer root.Close()
	var detected []string
	var warnings []string
	for _, candidate := range agent.All() {
		relative, err := filepath.Rel(projectPath, candidate.ProjectSkillDir(projectPath))
		if err != nil || relative == "." || relative == "" || filepath.IsAbs(relative) {
			warnings = append(warnings, fmt.Sprintf("%s: unsafe project skill directory", candidate.Name()))
			continue
		}
		parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
		current := ""
		missing := false
		unsafe := false
		for _, part := range parts {
			if current == "" {
				current = part
			} else {
				current = filepath.Join(current, part)
			}
			info, statErr := root.Lstat(current)
			if os.IsNotExist(statErr) {
				missing = true
				break
			}
			if statErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s: cannot inspect %s: %v", candidate.Name(), relative, statErr))
				unsafe = true
				break
			}
			if info.Mode()&os.ModeSymlink != 0 || projectPathComponentUnsafe(info) {
				warnings = append(warnings, fmt.Sprintf("%s: %s contains a symlink or reparse point", candidate.Name(), relative))
				unsafe = true
				break
			}
			if !info.IsDir() {
				warnings = append(warnings, fmt.Sprintf("%s: %s is not a directory", candidate.Name(), relative))
				unsafe = true
				break
			}
		}
		if missing || unsafe {
			continue
		}
		opened, openErr := root.OpenRoot(relative)
		if openErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: cannot open %s: %v", candidate.Name(), relative, openErr))
			continue
		}
		if closeErr := opened.Close(); closeErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: cannot close %s: %v", candidate.Name(), relative, closeErr))
			continue
		}
		detected = append(detected, candidate.Name())
	}
	return detected, warnings, nil
}

func validProjectName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

func sameProjectPathIdentity(expected, actual string) bool {
	return expected == "" || expected == actual
}

func projectNameConflict(cfg *config.Config, name string) bool {
	for _, project := range cfg.Projects {
		if project.Name == name {
			return true
		}
	}
	return false
}
