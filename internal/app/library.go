package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) PreviewAdd(ctx context.Context, request AddPreviewRequest) (AddPreview, error) {
	if request.Source == "" {
		return AddPreview{}, fmt.Errorf("source is required")
	}
	if a.deps.AddDiscoverer == nil {
		return AddPreview{}, fmt.Errorf("add discovery service is required")
	}
	return a.deps.AddDiscoverer.Preview(ctx, request)
}

func (a *App) Add(ctx context.Context, request AddRequest) (Result, error) {
	if request.Source == "" {
		return Result{}, fmt.Errorf("source is required")
	}
	if a.deps.Library == nil {
		return Result{}, fmt.Errorf("library mutation service is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		var selectors []link.Selector
		var recovered link.Result
		if request.Agent != "" || request.Project != "" {
			var selectorErr error
			selectors, selectorErr = bindingSelectors(tx.Config, BindingRequest{Agent: request.Agent, Project: request.Project})
			if selectorErr != nil {
				return selectorErr
			}
			recovered, selectorErr = a.recoverSelectors(tx, selectors)
			if selectorErr != nil {
				output = Result{Link: recovered, Exit: ExitPartial}
				return selectorErr
			}
		}
		oldConfig := cloneConfig(tx.Config)
		mutation, err := a.deps.Library.PrepareAdd(ctx, AddPrepareRequest{
			Source: request.Source, Selections: append([]string(nil), request.Skills...),
			SourcePath: request.SourcePath, Ref: request.Ref, Force: request.Force,
		}, tx.Config.Library.Skills)
		if err != nil {
			return err
		}
		checkpointed := false
		defer func() {
			if !checkpointed {
				_ = mutation.Abort()
			}
		}()
		added := mutation.Entries()
		if len(added) == 0 {
			return fmt.Errorf("source contains no selected skills")
		}
		if err := validateExpectedAdd(added, request); err != nil {
			return err
		}
		if (request.Agent != "" || request.Project != "") && len(added) != 1 {
			return fmt.Errorf("add-and-enable requires exactly one selected skill")
		}
		for _, skill := range added {
			index := skillIndex(tx.Config, skill.ID)
			if index >= 0 {
				if !request.Force && tx.Config.Library.Skills[index].Hash != skill.Hash {
					return fmt.Errorf("skill id %q already exists", skill.ID)
				}
				tx.Config.Library.Skills[index] = skill
			} else {
				tx.Config.Library.Skills = append(tx.Config.Library.Skills, skill)
			}
		}
		if request.Agent != "" || request.Project != "" {
			if err := mutateBinding(tx.Config, BindingRequest{SkillID: added[0].ID, Agent: request.Agent, Project: request.Project}, true); err != nil {
				return err
			}
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
		checkpointed = true
		if err := mutation.Commit(ctx); err != nil {
			output = Result{Changed: true, Exit: ExitPartial}
			commitErr := err
			*tx.Config = *cloneConfig(oldConfig)
			if restoreErr := tx.Checkpoint(); restoreErr != nil {
				return fmt.Errorf("commit library add: %v; restore old ledger: %w", commitErr, restoreErr)
			}
			if recoverErr := a.recoverLibrary(ctx, tx.Config.Library.Skills); recoverErr != nil {
				return fmt.Errorf("commit library add: %v; recover old library: %w", commitErr, recoverErr)
			}
			return commitErr
		}
		var plan link.Plan
		var executed link.Result
		if len(selectors) > 0 {
			plan = plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(tx.Config, a.deps.UserHome), tx.Config.PendingOperations, selectors)
			executed = a.deps.Execute(plan, false)
		}
		mergeLinkResult(&executed, recovered)
		removeCompleted(tx.Config, executed.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Skills: added, Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func validateExpectedAdd(added []config.Skill, request AddRequest) error {
	if request.ExpectedResolved != "" {
		for _, skill := range added {
			if skill.Resolved != request.ExpectedResolved {
				return fmt.Errorf("source changed after preview: resolved object is %q, expected %q", skill.Resolved, request.ExpectedResolved)
			}
		}
	}
	if len(request.ExpectedCandidates) == 0 {
		return nil
	}
	if len(request.ExpectedCandidates) != len(added) {
		return fmt.Errorf("source changed after preview: selected candidate count changed")
	}
	used := make([]bool, len(added))
	for _, expected := range request.ExpectedCandidates {
		matched := false
		for i, skill := range added {
			if used[i] || skill.Name != expected.Name || skill.Hash != expected.Hash {
				continue
			}
			if skill.Source != "" && expected.RelativePath != "" && skill.SourcePath != expected.RelativePath {
				continue
			}
			used[i], matched = true, true
			break
		}
		if !matched {
			return fmt.Errorf("source changed after preview: candidate %q no longer matches", expected.RelativePath)
		}
	}
	return nil
}

func skillIndex(cfg *config.Config, id string) int {
	for i := range cfg.Library.Skills {
		if cfg.Library.Skills[i].ID == id {
			return i
		}
	}
	return -1
}

func (a *App) Remove(ctx context.Context, request RemoveRequest) (Result, error) {
	if request.SkillID == "" {
		return Result{}, fmt.Errorf("skill is required")
	}
	if a.deps.Library == nil {
		return Result{}, fmt.Errorf("library mutation service is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		skill, err := findSkill(tx.Config, request.SkillID)
		if err != nil {
			return err
		}
		references := skillReferences(tx.Config, skill.ID)
		if len(references) > 0 && !request.Force {
			return fmt.Errorf("skill %q is still referenced by %v", skill.ID, references)
		}
		selectors := selectorsForSkill(tx.Config, skill.ID)
		for _, operation := range tx.Config.PendingOperations {
			if operation.SkillID == skill.ID {
				selectors = append(selectors, link.Selector{Project: operation.Scope.Project, Agent: operation.Scope.Agent})
			}
		}
		selectors = uniqueSelectors(selectors)
		recovered, err := a.recoverSelectors(tx, selectors)
		output.Plan = link.Plan{Actions: append([]link.Action(nil), recovered.Actions...), Issues: append([]link.Issue(nil), recovered.Issues...), Warnings: append([]link.Issue(nil), recovered.Warnings...)}
		if err != nil {
			output.Link = recovered
			output.Exit = ExitPartial
			return err
		}
		mutation, err := a.deps.Library.PrepareRemove(ctx, skill)
		if err != nil {
			return err
		}
		firstCheckpoint := false
		defer func() {
			if !firstCheckpoint {
				_ = mutation.Abort()
			}
		}()
		cleanup, err := cleanupForSkill(tx.Config, a.deps.UserHome, skill.ID, selectors)
		if err != nil {
			return err
		}
		if request.Force {
			pruneSkillReferences(tx.Config, skill.ID)
		}
		tx.Config.PendingOperations = append(tx.Config.PendingOperations, cleanup...)
		if err := tx.Config.Validate(); err != nil {
			return err
		}
		mergePlan(&output.Plan, cleanupPlan(a.deps.Paths.LibrarySkills, cleanup))
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		firstCheckpoint = true
		cleanupResult := a.deps.Recover(a.deps.Paths.LibrarySkills, cleanup, link.Selector{}, false)
		mergeLinkResult(&cleanupResult, recovered)
		if !completedWithoutFailures(cleanupResult) {
			output.Link = cleanupResult
			output.Changed = true
			output.Exit = ExitPartial
			return nil
		}
		removeCompleted(tx.Config, cleanupResult.Completed)
		preRemove := cloneConfig(tx.Config)
		index := skillIndex(tx.Config, skill.ID)
		if index >= 0 {
			tx.Config.Library.Skills = append(tx.Config.Library.Skills[:index], tx.Config.Library.Skills[index+1:]...)
		}
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		if err := mutation.Commit(ctx); err != nil {
			output.Link = cleanupResult
			output.Changed = true
			output.Exit = ExitPartial
			commitErr := err
			*tx.Config = *cloneConfig(preRemove)
			if restoreErr := tx.Checkpoint(); restoreErr != nil {
				return fmt.Errorf("commit library remove: %v; restore skill ledger: %w", commitErr, restoreErr)
			}
			if recoverErr := a.recoverLibrary(ctx, tx.Config.Library.Skills); recoverErr != nil {
				return fmt.Errorf("commit library remove: %v; recover retained library: %w", commitErr, recoverErr)
			}
			return commitErr
		}
		output.Link = cleanupResult
		output.Changed = true
		output.Exit = ExitOK
		return nil
	})
	return output, err
}

func cleanupPlan(libraryRoot string, operations []config.PendingOperation) link.Plan {
	result := link.Recover(libraryRoot, operations, link.Selector{}, true)
	return link.Plan{Actions: result.Actions, Issues: result.Issues, Warnings: result.Warnings}
}

func mergePlan(dst *link.Plan, src link.Plan) {
	dst.Actions = append(dst.Actions, src.Actions...)
	dst.Issues = append(dst.Issues, src.Issues...)
	dst.Warnings = append(dst.Warnings, src.Warnings...)
}

func cleanupForSkill(cfg *config.Config, home, id string, selectors []link.Selector) ([]config.PendingOperation, error) {
	var result []config.PendingOperation
	seen := map[string]struct{}{}
	for _, target := range buildTargets(cfg, home) {
		matched := false
		for _, selector := range selectors {
			if selector.Matches(target.Scope) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for name, desiredID := range target.Desired {
			if desiredID != id {
				continue
			}
			path := filepath.Join(target.Dir, name)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			op, err := link.NewCleanupOperation("", target.Scope, path, id, "skill removed")
			if err != nil {
				return nil, err
			}
			result = append(result, op)
		}
	}
	return result, nil
}

func skillReferences(cfg *config.Config, id string) []string {
	var refs []string
	for _, preset := range cfg.Presets {
		if contains(preset.Skills, id) {
			refs = append(refs, "preset:"+preset.Name)
		}
	}
	for name, binding := range cfg.Agents {
		if contains(binding.Skills, id) {
			refs = append(refs, "agent:"+name)
		}
	}
	for _, project := range cfg.Projects {
		if contains(project.Skills, id) {
			refs = append(refs, "project:"+project.Name)
		}
		for name, binding := range project.AgentBindings {
			if contains(binding.Skills, id) {
				refs = append(refs, "project:"+project.Name+":"+name)
			}
		}
	}
	return refs
}

func pruneSkillReferences(cfg *config.Config, id string) {
	for i := range cfg.Presets {
		cfg.Presets[i].Skills = removeValue(cfg.Presets[i].Skills, id)
	}
	for name, binding := range cfg.Agents {
		binding.Skills = removeValue(binding.Skills, id)
		cfg.Agents[name] = binding
	}
	for i := range cfg.Projects {
		cfg.Projects[i].Skills = removeValue(cfg.Projects[i].Skills, id)
		for name, binding := range cfg.Projects[i].AgentBindings {
			binding.Skills = removeValue(binding.Skills, id)
			cfg.Projects[i].AgentBindings[name] = binding
		}
	}
}

func selectorsForSkill(cfg *config.Config, id string) []link.Selector {
	var selectors []link.Selector
	presetNames := map[string]struct{}{}
	for _, preset := range cfg.Presets {
		if contains(preset.Skills, id) {
			presetNames[preset.Name] = struct{}{}
		}
	}
	uses := func(binding config.Binding) bool {
		if contains(binding.Skills, id) {
			return true
		}
		for _, preset := range binding.Presets {
			if _, ok := presetNames[preset]; ok {
				return true
			}
		}
		return false
	}
	for agentName, binding := range cfg.Agents {
		if uses(binding) {
			selectors = append(selectors, link.Selector{Agent: agentName})
			for _, project := range cfg.Projects {
				if contains(project.Agents, agentName) {
					selectors = append(selectors, link.Selector{Project: project.Name, Agent: agentName})
				}
			}
		}
	}
	for _, project := range cfg.Projects {
		if uses(project.Binding) {
			selectors = append(selectors, link.Selector{Project: project.Name})
		}
		for agentName, binding := range project.AgentBindings {
			if uses(binding) {
				selectors = append(selectors, link.Selector{Project: project.Name, Agent: agentName})
			}
		}
	}
	return uniqueSelectors(selectors)
}
