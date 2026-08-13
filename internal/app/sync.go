package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

func scopeIssues(cfg *config.Config) []scope.Issue { return scope.ValidateEffective(cfg) }

func (a *App) Sync(ctx context.Context, request SyncRequest) (Result, error) {
	selector := link.Selector{Agent: request.Agent, Project: request.Project}
	if request.DryRun {
		cfg, err := a.deps.Store.Load(ctx)
		if err != nil {
			return Result{}, err
		}
		plan := link.BuildPlan(a.deps.Paths.LibrarySkills, buildTargets(cfg, a.deps.UserHome), cfg.PendingOperations, selector)
		executed := a.deps.Execute(plan, true)
		return Result{Plan: plan, Link: executed, Exit: classify(executed)}, nil
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config.Library.Skills); err != nil {
			return err
		}
		recovered := a.deps.Recover(a.deps.Paths.LibrarySkills, tx.Config.PendingOperations, selector, false)
		removeCompleted(tx.Config, recovered.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		plan := link.BuildPlan(a.deps.Paths.LibrarySkills, buildTargets(tx.Config, a.deps.UserHome), tx.Config.PendingOperations, selector)
		executed := a.deps.Execute(plan, false)
		mergeLinkResult(&executed, recovered)
		removeCompleted(tx.Config, executed.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Plan: plan, Link: executed, Changed: len(executed.Applied) > 0, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func (a *App) Enable(ctx context.Context, request BindingRequest) (Result, error) {
	return a.changeBinding(ctx, request, true)
}

func (a *App) Disable(ctx context.Context, request BindingRequest) (Result, error) {
	return a.changeBinding(ctx, request, false)
}

func (a *App) changeBinding(ctx context.Context, request BindingRequest, enable bool) (Result, error) {
	if (request.SkillID == "") == (request.Preset == "") {
		return Result{}, fmt.Errorf("exactly one skill or preset is required")
	}
	if request.Agent == "" && request.Project == "" {
		return Result{}, fmt.Errorf("agent or project is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config.Library.Skills); err != nil {
			return err
		}
		selectors, err := bindingSelectors(tx.Config, request)
		if err != nil {
			return err
		}
		var recovered link.Result
		for _, selector := range selectors {
			mergeLinkResult(&recovered, a.deps.Recover(a.deps.Paths.LibrarySkills, tx.Config.PendingOperations, selector, false))
		}
		removeCompleted(tx.Config, recovered.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		if !completedWithoutFailures(recovered) {
			output = Result{Link: recovered, Exit: ExitPartial}
			return resultError(recovered)
		}
		if request.SkillID != "" {
			skill, err := findSkill(tx.Config, request.SkillID)
			if err != nil {
				return err
			}
			request.SkillID = skill.ID
		} else {
			if _, err := findPreset(tx.Config, request.Preset); err != nil {
				return err
			}
		}
		if err := mutateBinding(tx.Config, request, enable); err != nil {
			return err
		}
		if err := tx.Config.Validate(); err != nil {
			return err
		}
		if err := validateEffective(tx.Config); err != nil {
			return err
		}
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		plan := plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(tx.Config, a.deps.UserHome), tx.Config.PendingOperations, selectors)
		executed := a.deps.Execute(plan, false)
		mergeLinkResult(&executed, recovered)
		removeCompleted(tx.Config, executed.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func mutateBinding(cfg *config.Config, request BindingRequest, enable bool) error {
	mutate := func(binding config.Binding) config.Binding {
		if request.SkillID != "" {
			if enable {
				binding.Skills = appendUnique(binding.Skills, request.SkillID)
			} else {
				binding.Skills = removeValue(binding.Skills, request.SkillID)
			}
		} else if enable {
			binding.Presets = appendUnique(binding.Presets, request.Preset)
		} else {
			binding.Presets = removeValue(binding.Presets, request.Preset)
		}
		return binding
	}
	if request.Project == "" {
		if _, ok := agent.ByName(request.Agent); !ok {
			return fmt.Errorf("unknown agent %q", request.Agent)
		}
		if cfg.Agents == nil {
			cfg.Agents = map[string]config.Binding{}
		}
		cfg.Agents[request.Agent] = mutate(cfg.Agents[request.Agent])
		return nil
	}
	project, err := findProject(cfg, request.Project)
	if err != nil {
		return err
	}
	if request.Agent == "" {
		project.Binding = mutate(project.Binding)
		return nil
	}
	if !contains(project.Agents, request.Agent) {
		return fmt.Errorf("agent %q is not declared by project %q; add it first", request.Agent, project.Name)
	}
	if project.AgentBindings == nil {
		project.AgentBindings = map[string]config.Binding{}
	}
	project.AgentBindings[request.Agent] = mutate(project.AgentBindings[request.Agent])
	return nil
}

func bindingSelectors(cfg *config.Config, request BindingRequest) ([]link.Selector, error) {
	if request.Project != "" {
		project, err := findProject(cfg, request.Project)
		if err != nil {
			return nil, err
		}
		if request.Agent != "" {
			if !contains(project.Agents, request.Agent) {
				return nil, fmt.Errorf("agent %q is not declared by project %q", request.Agent, project.Name)
			}
			return []link.Selector{{Project: project.Name, Agent: request.Agent}}, nil
		}
		return []link.Selector{{Project: project.Name}}, nil
	}
	if _, ok := agent.ByName(request.Agent); !ok {
		return nil, fmt.Errorf("unknown agent %q", request.Agent)
	}
	selectors := []link.Selector{{Agent: request.Agent}}
	for _, project := range cfg.Projects {
		if contains(project.Agents, request.Agent) {
			selectors = append(selectors, link.Selector{Project: project.Name, Agent: request.Agent})
		}
	}
	return selectors, nil
}

func buildTargets(cfg *config.Config, home string) []link.Target {
	var targets []link.Target
	for _, a := range agent.All() {
		view := scope.Global(cfg, a.Name())
		targets = append(targets, link.Target{Scope: view.Scope, Home: home, Dir: a.GlobalSkillDir(home), Desired: view.Desired(), Blocked: issueMessage(view.Issues)})
	}
	for _, project := range cfg.Projects {
		for _, name := range project.Agents {
			a, ok := agent.ByName(name)
			if !ok {
				continue
			}
			view := scope.Project(cfg, project, name)
			targets = append(targets, link.Target{Scope: view.Scope, Dir: a.ProjectSkillDir(project.Path), Desired: view.Desired(), Blocked: issueMessage(view.Issues)})
		}
	}
	return targets
}

func issueMessage(issues []scope.Issue) string {
	if len(issues) == 0 {
		return ""
	}
	return issues[0].Message
}

func plansForSelectors(root string, targets []link.Target, pending []config.PendingOperation, selectors []link.Selector) link.Plan {
	var result link.Plan
	seenActions := map[string]struct{}{}
	for _, selector := range selectors {
		plan := link.BuildPlan(root, targets, pending, selector)
		for _, action := range plan.Actions {
			key := string(action.Kind) + "\x00" + action.Path + "\x00" + action.Operation
			if _, ok := seenActions[key]; ok {
				continue
			}
			seenActions[key] = struct{}{}
			result.Actions = append(result.Actions, action)
		}
		result.Issues = append(result.Issues, plan.Issues...)
		result.Warnings = append(result.Warnings, plan.Warnings...)
	}
	return result
}

func uniqueSelectors(selectors []link.Selector) []link.Selector {
	seen := map[string]struct{}{}
	result := make([]link.Selector, 0, len(selectors))
	for _, selector := range selectors {
		key := selector.Project + "\x00" + selector.Agent
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, selector)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Project == result[j].Project {
			return result[i].Agent < result[j].Agent
		}
		return result[i].Project < result[j].Project
	})
	return result
}

func mergeLinkResult(dst *link.Result, src link.Result) {
	dst.Actions = append(dst.Actions, src.Actions...)
	dst.Applied = append(dst.Applied, src.Applied...)
	dst.Failures = append(dst.Failures, src.Failures...)
	dst.Issues = append(dst.Issues, src.Issues...)
	dst.Warnings = append(dst.Warnings, src.Warnings...)
	dst.Completed = append(dst.Completed, src.Completed...)
}

func (a *App) recoverSelectors(tx *config.Tx, selectors []link.Selector) (link.Result, error) {
	var recovered link.Result
	for _, selector := range uniqueSelectors(selectors) {
		mergeLinkResult(&recovered, a.deps.Recover(a.deps.Paths.LibrarySkills, tx.Config.PendingOperations, selector, false))
	}
	removeCompleted(tx.Config, recovered.Completed)
	if err := tx.Checkpoint(); err != nil {
		return recovered, err
	}
	if !completedWithoutFailures(recovered) {
		return recovered, resultError(recovered)
	}
	return recovered, nil
}
