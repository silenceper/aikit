package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) EditProject(ctx context.Context, request ProjectEditRequest) (Result, error) {
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		var validatedPath string
		if request.Project == "" || request.Path != "" {
			path, identity, err := validateProjectDirectory(request.Path)
			if err != nil {
				return err
			}
			if !sameProjectPathIdentity(request.ExpectedPathIdentity, identity) {
				return fmt.Errorf("project directory changed since preview")
			}
			validatedPath = path
		}
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		if request.Project == "" {
			if request.Name == "" || request.Path == "" {
				return fmt.Errorf("new project name and path are required")
			}
			project := config.Project{Name: request.Name, Path: validatedPath, Agents: append([]string(nil), request.AddAgents...), AgentBindings: map[string]config.Binding{}}
			tx.Config.Projects = append(tx.Config.Projects, project)
			if err := tx.Config.Validate(); err != nil {
				return err
			}
			if err := validateEffective(tx.Config); err != nil {
				return err
			}
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			plan := link.BuildPlan(a.deps.Paths.LibrarySkills, buildTargets(tx.Config, a.deps.UserHome), tx.Config.PendingOperations, link.Selector{Project: project.Name})
			executed := a.deps.Execute(plan, false)
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			output = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
			return nil
		}
		project, err := findProject(tx.Config, request.Project)
		if err != nil {
			return err
		}
		old := *project
		old.AgentBindings = cloneBindings(project.AgentBindings)
		old.Agents = append([]string(nil), project.Agents...)
		recovered, err := a.recoverSelectors(tx, []link.Selector{{Project: old.Name}})
		if err != nil {
			output = Result{Link: recovered, Exit: ExitPartial}
			return err
		}
		pathChanged := false
		if request.Path != "" {
			pathChanged = validatedPath != project.Path
			if pathChanged && !request.Confirmed {
				return fmt.Errorf("project path rebind requires confirmation")
			}
			project.Path = validatedPath
		}
		if request.Name != "" {
			project.Name = request.Name
		}
		for _, name := range request.AddAgents {
			if _, ok := agent.ByName(name); !ok {
				return fmt.Errorf("unknown agent %q", name)
			}
			project.Agents = appendUnique(project.Agents, name)
		}
		removed := map[string]struct{}{}
		for _, name := range request.RemoveAgents {
			if contains(project.Agents, name) {
				removed[name] = struct{}{}
			}
			project.Agents = removeValue(project.Agents, name)
			delete(project.AgentBindings, name)
		}
		newName := project.Name
		if err := tx.Config.Validate(); err != nil {
			return err
		}
		if err := validateEffective(tx.Config); err != nil {
			return err
		}
		var cleanup []config.PendingOperation
		if pathChanged && existingDirectory(old.Path) {
			ops, err := cleanupForProject(tx.Config, old, newName, nil, "project path rebind")
			if err != nil {
				return err
			}
			cleanup = append(cleanup, ops...)
		} else if len(removed) > 0 {
			ops, err := cleanupForProject(tx.Config, old, newName, removed, "project agent removed")
			if err != nil {
				return err
			}
			cleanup = append(cleanup, ops...)
		}
		tx.Config.PendingOperations = append(tx.Config.PendingOperations, cleanup...)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		var cleanupResult link.Result
		if len(cleanup) > 0 {
			cleanupResult = a.deps.Recover(a.deps.Paths.LibrarySkills, cleanup, link.Selector{}, false)
			removeCompleted(tx.Config, cleanupResult.Completed)
		}
		if !completedWithoutFailures(cleanupResult) {
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			mergeLinkResult(&cleanupResult, recovered)
			output = Result{Link: cleanupResult, Changed: true, Exit: ExitPartial}
			return nil
		}
		plan := link.BuildPlan(a.deps.Paths.LibrarySkills, buildTargets(tx.Config, a.deps.UserHome), tx.Config.PendingOperations, link.Selector{Project: newName})
		executed := a.deps.Execute(plan, false)
		mergeLinkResult(&executed, recovered)
		mergeLinkResult(&executed, cleanupResult)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func (a *App) RemoveProject(ctx context.Context, request ProjectRemoveRequest) (Result, error) {
	if request.Project == "" {
		return Result{}, fmt.Errorf("project is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		index := -1
		for i := range tx.Config.Projects {
			if tx.Config.Projects[i].Name == request.Project {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("project %q not found", request.Project)
		}
		old := tx.Config.Projects[index]
		priorRecovery, err := a.recoverSelectors(tx, []link.Selector{{Project: old.Name}})
		if err != nil {
			output = Result{Link: priorRecovery, Exit: ExitPartial}
			return err
		}
		cleanup, err := cleanupForProject(tx.Config, old, old.Name, nil, "project removed")
		if err != nil {
			return err
		}
		tx.Config.Projects = append(tx.Config.Projects[:index], tx.Config.Projects[index+1:]...)
		tx.Config.PendingOperations = append(tx.Config.PendingOperations, cleanup...)
		if err := tx.Config.Validate(); err != nil {
			return err
		}
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		recovered := a.deps.Recover(a.deps.Paths.LibrarySkills, cleanup, link.Selector{}, false)
		mergeLinkResult(&recovered, priorRecovery)
		removeCompleted(tx.Config, recovered.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Link: recovered, Changed: true, Exit: classify(recovered)}
		return nil
	})
	return output, err
}

func cleanupForProject(cfg *config.Config, project config.Project, scopeName string, agents map[string]struct{}, reason string) ([]config.PendingOperation, error) {
	var operations []config.PendingOperation
	for _, agentName := range project.Agents {
		if agents != nil {
			if _, ok := agents[agentName]; !ok {
				continue
			}
		}
		a, ok := agent.ByName(agentName)
		if !ok {
			continue
		}
		view := scope.Project(cfg, project, agentName)
		names := make([]string, 0, len(view.Skills))
		for name := range view.Skills {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			skill := view.Skills[name]
			target := filepath.Join(a.ProjectSkillDir(project.Path), name)
			op, err := link.NewCleanupOperation("", config.Scope{Project: scopeName, ProjectPath: project.Path, Agent: agentName}, target, skill.ID, reason)
			if err != nil {
				return nil, err
			}
			operations = append(operations, op)
		}
	}
	return operations, nil
}

func (a *App) PreviewProjectEdit(ctx context.Context, request ProjectEditRequest) (ProjectEditPreview, error) {
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return ProjectEditPreview{}, err
	}
	return a.previewProjectEditWithConfig(cfg, request)
}

func (a *App) previewProjectEditWithConfig(cfg *config.Config, request ProjectEditRequest) (ProjectEditPreview, error) {
	copy := cloneConfig(cfg)
	if request.Project == "" {
		if request.Name == "" || request.Path == "" {
			return ProjectEditPreview{}, fmt.Errorf("new project name and path are required")
		}
		path, identity, err := validateProjectDirectory(request.Path)
		if err != nil {
			return ProjectEditPreview{}, err
		}
		project := config.Project{Name: request.Name, Path: path, Agents: append([]string(nil), request.AddAgents...), AgentBindings: map[string]config.Binding{}}
		copy.Projects = append(copy.Projects, project)
		if err := validatePreviewConfig(copy); err != nil {
			return ProjectEditPreview{}, err
		}
		next := link.BuildPlan(a.deps.Paths.LibrarySkills, buildTargets(copy, a.deps.UserHome), copy.PendingOperations, link.Selector{Project: project.Name})
		return ProjectEditPreview{Next: next, PathIdentity: identity}, nil
	}
	project, err := findProject(copy, request.Project)
	if err != nil {
		return ProjectEditPreview{}, err
	}
	old := *project
	old.Agents = append([]string(nil), project.Agents...)
	old.AgentBindings = cloneBindings(project.AgentBindings)
	if request.Name != "" {
		project.Name = request.Name
	}
	if request.Path != "" {
		var identity string
		project.Path, identity, err = validateProjectDirectory(request.Path)
		if err != nil {
			return ProjectEditPreview{}, err
		}
		request.ExpectedPathIdentity = identity
	}
	for _, name := range request.AddAgents {
		project.Agents = appendUnique(project.Agents, name)
	}
	removed := map[string]struct{}{}
	for _, name := range request.RemoveAgents {
		removed[name] = struct{}{}
		project.Agents = removeValue(project.Agents, name)
		delete(project.AgentBindings, name)
	}
	if err := validatePreviewConfig(copy); err != nil {
		return ProjectEditPreview{}, err
	}
	var operations []config.PendingOperation
	if old.Path != project.Path && existingDirectory(old.Path) {
		operations, err = cleanupForProject(copy, old, project.Name, nil, "project path rebind")
	} else if len(removed) > 0 {
		operations, err = cleanupForProject(copy, old, project.Name, removed, "project agent removed")
	}
	if err != nil {
		return ProjectEditPreview{}, err
	}
	cleanupPlan := link.BuildPlan(a.deps.Paths.LibrarySkills, nil, operations, link.Selector{})
	nextPlan := link.BuildPlan(a.deps.Paths.LibrarySkills, buildTargets(copy, a.deps.UserHome), nil, link.Selector{Project: project.Name})
	return ProjectEditPreview{Cleanup: cleanupPlan, Next: nextPlan, PathIdentity: request.ExpectedPathIdentity}, nil
}

func (a *App) PreviewProjectRemove(ctx context.Context, request ProjectRemoveRequest) (MutationPreview, error) {
	if request.Project == "" {
		return MutationPreview{}, fmt.Errorf("project is required")
	}
	loaded, err := a.deps.Store.Load(ctx)
	if err != nil {
		return MutationPreview{}, err
	}
	next := cloneConfig(loaded)
	project, err := findProject(next, request.Project)
	if err != nil {
		return MutationPreview{}, err
	}
	old := *project
	old.Agents = append([]string(nil), project.Agents...)
	old.AgentBindings = cloneBindings(project.AgentBindings)
	selectors := []link.Selector{{Project: old.Name}}
	preview := MutationPreview{
		Title:                "Remove project",
		Summary:              fmt.Sprintf("Remove project %q and clean its managed paths", old.Name),
		AffectedScopes:       affectedScopes(next, selectors),
		RequiresConfirmation: true,
	}
	for _, scope := range preview.AffectedScopes {
		agentName := scope.Agent
		if agentName == "" {
			agentName = "common"
		}
		preview.References = append(preview.References, "project:"+old.Name+":"+agentName)
	}
	cleanup, err := cleanupForProject(next, old, old.Name, nil, "project removed")
	if err != nil {
		return MutationPreview{}, err
	}
	for i := range next.Projects {
		if next.Projects[i].Name == old.Name {
			next.Projects = append(next.Projects[:i], next.Projects[i+1:]...)
			break
		}
	}
	if err := validatePreviewConfig(next); err != nil {
		return MutationPreview{}, err
	}
	preview.Plan = link.BuildPlan(a.deps.Paths.LibrarySkills, nil, cleanup, link.Selector{})
	addPlanDiagnostics(&preview)
	return preview, nil
}

func cloneConfig(cfg *config.Config) *config.Config {
	copy := *cfg
	copy.Library.Skills = append([]config.Skill(nil), cfg.Library.Skills...)
	copy.Presets = append([]config.Preset(nil), cfg.Presets...)
	for i := range copy.Presets {
		copy.Presets[i].Skills = append([]string(nil), cfg.Presets[i].Skills...)
	}
	copy.Agents = make(map[string]config.Binding, len(cfg.Agents))
	for name, binding := range cfg.Agents {
		binding.Skills = append([]string(nil), binding.Skills...)
		binding.Presets = append([]string(nil), binding.Presets...)
		copy.Agents[name] = binding
	}
	copy.Projects = append([]config.Project(nil), cfg.Projects...)
	for i := range copy.Projects {
		copy.Projects[i].Agents = append([]string(nil), cfg.Projects[i].Agents...)
		copy.Projects[i].Skills = append([]string(nil), cfg.Projects[i].Skills...)
		copy.Projects[i].Presets = append([]string(nil), cfg.Projects[i].Presets...)
		copy.Projects[i].AgentBindings = cloneBindings(cfg.Projects[i].AgentBindings)
	}
	copy.PendingOperations = append([]config.PendingOperation(nil), cfg.PendingOperations...)
	return &copy
}

func cloneBindings(values map[string]config.Binding) map[string]config.Binding {
	result := make(map[string]config.Binding, len(values))
	for key, value := range values {
		value.Skills = append([]string(nil), value.Skills...)
		value.Presets = append([]string(nil), value.Presets...)
		result[key] = value
	}
	return result
}
