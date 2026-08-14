package app

import (
	"context"
	"fmt"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) PutPreset(ctx context.Context, request PresetRequest) (Result, error) {
	if request.Name == "" {
		return Result{}, fmt.Errorf("preset name is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		preset, findErr := findPreset(tx.Config, request.Name)
		if findErr != nil {
			if !request.Create || request.Remove {
				return findErr
			}
			tx.Config.Presets = append(tx.Config.Presets, config.Preset{Name: request.Name})
			preset = &tx.Config.Presets[len(tx.Config.Presets)-1]
		}
		selectors := selectorsForPreset(tx.Config, request.Name)
		recovered, err := a.recoverSelectors(tx, selectors)
		if err != nil {
			output = Result{Link: recovered, Exit: ExitPartial}
			return err
		}
		for _, requested := range request.Skills {
			skill, err := findSkill(tx.Config, requested)
			if err != nil {
				return err
			}
			if request.Remove {
				preset.Skills = removeValue(preset.Skills, skill.ID)
			} else {
				preset.Skills = appendUnique(preset.Skills, skill.ID)
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
		plan := plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(tx.Config, a.deps.UserHome), tx.Config.PendingOperations, selectors)
		executed := a.deps.Execute(plan, false)
		mergeLinkResult(&executed, recovered)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func (a *App) RemovePreset(ctx context.Context, request PresetRemoveRequest) (Result, error) {
	if request.Name == "" {
		return Result{}, fmt.Errorf("preset name is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		index := -1
		for i := range tx.Config.Presets {
			if tx.Config.Presets[i].Name == request.Name {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("preset %q not found", request.Name)
		}
		selectors := selectorsForPreset(tx.Config, request.Name)
		if len(selectors) > 0 && !request.Force {
			return fmt.Errorf("preset %q is still referenced", request.Name)
		}
		recovered, err := a.recoverSelectors(tx, selectors)
		if err != nil {
			output = Result{Link: recovered, Exit: ExitPartial}
			return err
		}
		if request.Force {
			prunePresetReferences(tx.Config, request.Name)
		}
		tx.Config.Presets = append(tx.Config.Presets[:index], tx.Config.Presets[index+1:]...)
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
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func selectorsForPreset(cfg *config.Config, name string) []link.Selector {
	var selectors []link.Selector
	for agentName, binding := range cfg.Agents {
		if contains(binding.Presets, name) {
			selectors = append(selectors, link.Selector{Agent: agentName})
			for _, project := range cfg.Projects {
				if contains(project.Agents, agentName) {
					selectors = append(selectors, link.Selector{Project: project.Name, Agent: agentName})
				}
			}
		}
	}
	for _, project := range cfg.Projects {
		if contains(project.Presets, name) {
			selectors = append(selectors, link.Selector{Project: project.Name})
		}
		for agentName, binding := range project.AgentBindings {
			if contains(binding.Presets, name) {
				selectors = append(selectors, link.Selector{Project: project.Name, Agent: agentName})
			}
		}
	}
	return uniqueSelectors(selectors)
}

func prunePresetReferences(cfg *config.Config, name string) {
	for agentName, binding := range cfg.Agents {
		binding.Presets = removeValue(binding.Presets, name)
		cfg.Agents[agentName] = binding
	}
	for i := range cfg.Projects {
		cfg.Projects[i].Presets = removeValue(cfg.Projects[i].Presets, name)
		for agentName, binding := range cfg.Projects[i].AgentBindings {
			binding.Presets = removeValue(binding.Presets, name)
			cfg.Projects[i].AgentBindings[agentName] = binding
		}
	}
}
