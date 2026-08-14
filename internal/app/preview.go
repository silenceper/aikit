package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) PreviewBinding(ctx context.Context, request BindingPreviewRequest) (MutationPreview, error) {
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return MutationPreview{}, err
	}
	next := cloneConfig(cfg)
	binding := request.Binding
	if (binding.SkillID == "") == (binding.Preset == "") {
		return MutationPreview{}, fmt.Errorf("exactly one skill or preset is required")
	}
	if binding.Agent == "" && binding.Project == "" {
		return MutationPreview{}, fmt.Errorf("agent or project is required")
	}
	selectors, err := bindingSelectors(next, binding)
	if err != nil {
		return MutationPreview{}, err
	}
	if binding.SkillID != "" {
		skill, err := findSkill(next, binding.SkillID)
		if err != nil {
			return MutationPreview{}, err
		}
		binding.SkillID = skill.ID
	} else if _, err := findPreset(next, binding.Preset); err != nil {
		return MutationPreview{}, err
	}
	if err := mutateBinding(next, binding, request.Enable); err != nil {
		return MutationPreview{}, err
	}
	if err := validatePreviewConfig(next); err != nil {
		return MutationPreview{}, err
	}
	plan := plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(next, a.deps.UserHome), next.PendingOperations, selectors)
	action := "Disable"
	if request.Enable {
		action = "Enable"
	}
	kind, value := "preset", binding.Preset
	if binding.SkillID != "" {
		kind, value = "skill", binding.SkillID
	}
	preview := MutationPreview{
		Title:                action + " " + kind,
		Summary:              fmt.Sprintf("%s %s %q in %d affected scope(s)", action, kind, value, len(affectedScopes(cfg, selectors))),
		AffectedScopes:       affectedScopes(cfg, selectors),
		Plan:                 plan,
		RequiresConfirmation: true,
	}
	addPlanDiagnostics(&preview)
	return preview, nil
}

func (a *App) PreviewRemove(ctx context.Context, request RemoveRequest) (MutationPreview, error) {
	if request.SkillID == "" {
		return MutationPreview{}, fmt.Errorf("skill is required")
	}
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return MutationPreview{}, err
	}
	next := cloneConfig(cfg)
	skill, err := findSkill(next, request.SkillID)
	if err != nil {
		return MutationPreview{}, err
	}
	references := skillReferences(next, skill.ID)
	sort.Strings(references)
	selectors := selectorsForSkill(next, skill.ID)
	for _, operation := range next.PendingOperations {
		if operation.SkillID == skill.ID {
			selectors = append(selectors, link.Selector{Project: operation.Scope.Project, Agent: operation.Scope.Agent})
		}
	}
	selectors = uniqueSelectors(selectors)
	scopes := affectedScopes(next, selectors)
	for _, operation := range next.PendingOperations {
		if operation.SkillID == skill.ID && !containsScopeValue(scopes, operation.Scope) {
			scopes = append(scopes, operation.Scope)
		}
	}
	sortScopes(scopes)
	cleanup, err := cleanupForSkill(next, a.deps.UserHome, skill.ID, selectors)
	if err != nil {
		return MutationPreview{}, err
	}
	operations := pendingForSkill(next.PendingOperations, skill.ID)
	operations = append(operations, cleanup...)
	pruneSkillReferences(next, skill.ID)
	if index := skillIndex(next, skill.ID); index >= 0 {
		next.Library.Skills = append(next.Library.Skills[:index], next.Library.Skills[index+1:]...)
	}
	if err := validatePreviewConfig(next); err != nil {
		return MutationPreview{}, err
	}
	plan := cleanupPlan(a.deps.Paths.LibrarySkills, operations)
	preview := MutationPreview{
		Title:                "Remove skill",
		Summary:              fmt.Sprintf("Remove skill %q from %d affected scope(s)", skill.ID, len(scopes)),
		AffectedScopes:       scopes,
		References:           references,
		Plan:                 plan,
		RequiresForce:        len(references) > 0,
		RequiresConfirmation: true,
	}
	addPlanDiagnostics(&preview)
	return preview, nil
}

func pendingForSkill(operations []config.PendingOperation, skillID string) []config.PendingOperation {
	result := make([]config.PendingOperation, 0, len(operations))
	for _, operation := range operations {
		if operation.SkillID == skillID {
			result = append(result, operation)
		}
	}
	return result
}

func containsScopeValue(scopes []config.Scope, wanted config.Scope) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func sortScopes(scopes []config.Scope) {
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Project == scopes[j].Project {
			return scopes[i].Agent < scopes[j].Agent
		}
		return scopes[i].Project < scopes[j].Project
	})
}

func (a *App) PreviewPreset(ctx context.Context, request PresetPreviewRequest) (MutationPreview, error) {
	if request.Name == "" {
		return MutationPreview{}, fmt.Errorf("preset name is required")
	}
	if request.Delete && (request.Create || request.Remove || len(request.Skills) > 0) {
		return MutationPreview{}, fmt.Errorf("preset deletion cannot include a member edit")
	}
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return MutationPreview{}, err
	}
	next := cloneConfig(cfg)
	preview := MutationPreview{RequiresConfirmation: true}
	if request.Delete {
		index := presetIndex(next, request.Name)
		if index < 0 {
			return MutationPreview{}, fmt.Errorf("preset %q not found", request.Name)
		}
		selectors := selectorsForPreset(next, request.Name)
		preview.Title = "Remove preset"
		preview.References = presetReferences(next, request.Name)
		preview.RequiresForce = len(preview.References) > 0
		preview.AffectedScopes = affectedScopes(next, selectors)
		prunePresetReferences(next, request.Name)
		next.Presets = append(next.Presets[:index], next.Presets[index+1:]...)
		if err := validatePreviewConfig(next); err != nil {
			return MutationPreview{}, err
		}
		preview.Plan = plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(next, a.deps.UserHome), next.PendingOperations, selectors)
		preview.Summary = fmt.Sprintf("Remove preset %q from %d affected scope(s)", request.Name, len(preview.AffectedScopes))
	} else {
		preset, findErr := findPreset(next, request.Name)
		if findErr != nil {
			if !request.Create {
				return MutationPreview{}, findErr
			}
			next.Presets = append(next.Presets, config.Preset{Name: request.Name})
			preset = &next.Presets[len(next.Presets)-1]
		}
		selectors := selectorsForPreset(next, request.Name)
		for _, requested := range request.Skills {
			skill, err := findSkill(next, requested)
			if err != nil {
				return MutationPreview{}, err
			}
			if request.Remove {
				preset.Skills = removeValue(preset.Skills, skill.ID)
			} else {
				preset.Skills = appendUnique(preset.Skills, skill.ID)
			}
		}
		if err := validatePreviewConfig(next); err != nil {
			return MutationPreview{}, err
		}
		preview.Title = "Update preset"
		if request.Create && len(request.Skills) == 0 {
			preview.Title = "Create preset"
		}
		preview.AffectedScopes = affectedScopes(next, selectors)
		preview.References = presetReferences(next, request.Name)
		preview.Plan = plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(next, a.deps.UserHome), next.PendingOperations, selectors)
		preview.Summary = fmt.Sprintf("Update preset %q in %d affected scope(s)", request.Name, len(preview.AffectedScopes))
	}
	sort.Strings(preview.References)
	addPlanDiagnostics(&preview)
	return preview, nil
}

func (a *App) PreviewPresetMutation(ctx context.Context, request PresetMutationRequest) (MutationPreview, error) {
	if request.Name == "" {
		return MutationPreview{}, fmt.Errorf("preset name is required")
	}
	loaded, err := a.deps.Store.Load(ctx)
	if err != nil {
		return MutationPreview{}, err
	}
	next := cloneConfig(loaded)
	beforeSelectors := selectorsForPreset(next, request.Name)
	references := presetReferences(next, request.Name)
	simulated := request
	requiresForce := request.Operation == PresetDelete && len(beforeSelectors) > 0 && !request.Force
	if requiresForce {
		simulated.Force = true
	}
	if err := mutatePresetConfig(next, simulated); err != nil {
		return MutationPreview{}, err
	}
	if err := next.Validate(); err != nil {
		return MutationPreview{}, err
	}
	if err := validateEffective(next); err != nil {
		return MutationPreview{}, err
	}
	name := request.Name
	if request.Operation == PresetRename || request.Operation == PresetDuplicate {
		name = request.NewName
	}
	afterSelectors := selectorsForPreset(next, name)
	selectors := uniqueSelectors(append(beforeSelectors, afterSelectors...))
	preview := MutationPreview{
		Title:                "Preset " + string(request.Operation),
		Summary:              presetMutationSummary(request, len(selectors)),
		References:           references,
		AffectedScopes:       uniqueScopes(append(affectedScopes(loaded, beforeSelectors), affectedScopes(next, afterSelectors)...)),
		Plan:                 plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(next, a.deps.UserHome), nil, selectors),
		RequiresForce:        requiresForce,
		RequiresConfirmation: true,
	}
	for _, issue := range preview.Plan.Issues {
		preview.Conflicts = append(preview.Conflicts, issue.Message)
	}
	for _, warning := range preview.Plan.Warnings {
		preview.Warnings = append(preview.Warnings, warning.Message)
	}
	sort.Strings(preview.References)
	return preview, nil
}

func presetMutationSummary(request PresetMutationRequest, scopes int) string {
	switch request.Operation {
	case PresetDuplicate:
		return fmt.Sprintf("Duplicate preset %q -> %q in %d affected scope(s)", request.Name, request.NewName, scopes)
	case PresetRename:
		return fmt.Sprintf("Rename preset %q -> %q in %d affected scope(s)", request.Name, request.NewName, scopes)
	case PresetApply:
		target := "Global / " + request.Binding.Agent
		if request.Binding.Project != "" {
			target = "Project / " + request.Binding.Project + " / " + request.Binding.Agent
			if request.Binding.Agent == "" {
				target = "Project / " + request.Binding.Project + " / common"
			}
		}
		return fmt.Sprintf("Apply preset %q to %s", request.Name, target)
	default:
		return fmt.Sprintf("%s preset %q in %d affected scope(s)", request.Operation, request.Name, scopes)
	}
}

func uniqueScopes(scopes []config.Scope) []config.Scope {
	seen := map[string]struct{}{}
	result := make([]config.Scope, 0, len(scopes))
	for _, scope := range scopes {
		key := scope.Project + "\x00" + scope.ProjectPath + "\x00" + scope.Agent
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, scope)
	}
	sortScopes(result)
	return result
}

func validatePreviewConfig(cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return validateEffective(cfg)
}

func presetIndex(cfg *config.Config, name string) int {
	for i := range cfg.Presets {
		if cfg.Presets[i].Name == name {
			return i
		}
	}
	return -1
}

func presetReferences(cfg *config.Config, name string) []string {
	var references []string
	for agentName, binding := range cfg.Agents {
		if contains(binding.Presets, name) {
			references = append(references, "agent:"+agentName)
		}
	}
	for _, project := range cfg.Projects {
		if contains(project.Presets, name) {
			references = append(references, "project:"+project.Name)
		}
		for agentName, binding := range project.AgentBindings {
			if contains(binding.Presets, name) {
				references = append(references, "project:"+project.Name+":"+agentName)
			}
		}
	}
	sort.Strings(references)
	return references
}

func affectedScopes(cfg *config.Config, selectors []link.Selector) []config.Scope {
	var scopes []config.Scope
	seen := map[string]struct{}{}
	for _, target := range buildTargets(cfg, "") {
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
		key := target.Scope.Project + "\x00" + target.Scope.ProjectPath + "\x00" + target.Scope.Agent
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		scopes = append(scopes, target.Scope)
	}
	sortScopes(scopes)
	return scopes
}

func addPlanDiagnostics(preview *MutationPreview) {
	for i := range preview.Plan.Issues {
		issue := &preview.Plan.Issues[i]
		preview.Conflicts = append(preview.Conflicts, issue.Message)
		issue.Err = nil
	}
	for i := range preview.Plan.Warnings {
		warning := &preview.Plan.Warnings[i]
		preview.Warnings = append(preview.Warnings, warning.Message)
		warning.Err = nil
	}
}
