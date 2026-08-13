package migrate

import (
	"context"
	"fmt"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func (s *Service) importDiscovered(ctx context.Context, tx *config.Tx, item discovered, adopt bool) (app.ScanItem, error) {
	output := app.ScanItem{Origin: item.root.origin, Target: item.target}
	if item.managedID != "" {
		skill, ok := findSkill(tx.Config, item.managedID)
		if !ok {
			return output, fmt.Errorf("managed link references unknown skill %q", item.managedID)
		}
		output.Skill = skill
		if !adopt {
			return output, nil
		}
		before := cloneConfig(tx.Config)
		addBinding(tx.Config, item.root, skill.ID)
		if err := validateMutation(tx.Config); err != nil {
			*tx.Config = *before
			return output, err
		}
		if err := tx.Checkpoint(); err != nil {
			*tx.Config = *before
			return output, err
		}
		output.Adopted = true
		return output, nil
	}

	mutation, err := s.deps.Library.PrepareLocal(ctx, item.candidate.Root, nil, tx.Config.Library.Skills)
	if err != nil {
		return output, err
	}
	checkpointed := false
	defer func() {
		if !checkpointed {
			_ = mutation.Abort()
		}
	}()
	if len(mutation.Skills) != 1 || mutation.Skills[0].ID != item.allocated.ID || mutation.Skills[0].Hash != item.allocated.Hash {
		return output, fmt.Errorf("prepared local allocation changed for %s", item.target)
	}
	skill := mutation.Skills[0]
	output.Skill = skill
	before := cloneConfig(tx.Config)
	appendSkill(tx.Config, skill)
	var operation *config.PendingOperation
	if adopt {
		addBinding(tx.Config, item.root, skill.ID)
		op, opErr := link.NewAdoptOperation("", itemScope(item.root), item.target, skill.ID)
		if opErr != nil {
			*tx.Config = *before
			return output, opErr
		}
		operation = &op
		tx.Config.PendingOperations = append(tx.Config.PendingOperations, op)
	}
	if err := validateMutation(tx.Config); err != nil {
		*tx.Config = *before
		return output, err
	}
	if err := tx.Checkpoint(); err != nil {
		*tx.Config = *before
		return output, err
	}
	checkpointed = true
	if err := mutation.Commit(ctx); err != nil {
		commitErr := err
		*tx.Config = *before
		if restoreErr := tx.Checkpoint(); restoreErr != nil {
			return output, fmt.Errorf("commit scanned skill: %v; restore ledger: %w", commitErr, restoreErr)
		}
		if recoverErr := s.recoverLibrary(ctx, tx.Config.Library.Skills); recoverErr != nil {
			return output, fmt.Errorf("commit scanned skill: %v; recover library: %w", commitErr, recoverErr)
		}
		return output, commitErr
	}
	if operation == nil {
		return output, nil
	}
	recovered := s.deps.Recover(s.deps.Paths.LibrarySkills, []config.PendingOperation{*operation}, link.Selector{Agent: item.root.agent, Project: item.root.project}, false)
	removeCompleted(tx.Config, recovered.Completed)
	if err := tx.Checkpoint(); err != nil {
		return output, err
	}
	if len(recovered.Failures) > 0 {
		return output, fmt.Errorf("adopt %s: %s", item.target, recovered.Failures[0].Message)
	}
	if len(recovered.Issues) > 0 {
		return output, fmt.Errorf("adopt %s: %s", item.target, recovered.Issues[0].Message)
	}
	output.Adopted = true
	return output, nil
}

func itemScope(root scanRoot) config.Scope {
	result := config.Scope{Agent: root.agent}
	if root.project != "" {
		result.Project = root.project
		result.ProjectPath = root.projectPath
	}
	return result
}

func addBinding(cfg *config.Config, root scanRoot, id string) {
	if root.project == "" {
		binding := cfg.Agents[root.agent]
		binding.Skills = appendUnique(binding.Skills, id)
		if cfg.Agents == nil {
			cfg.Agents = map[string]config.Binding{}
		}
		cfg.Agents[root.agent] = binding
		return
	}
	for index := range cfg.Projects {
		if cfg.Projects[index].Name != root.project {
			continue
		}
		if cfg.Projects[index].AgentBindings == nil {
			cfg.Projects[index].AgentBindings = map[string]config.Binding{}
		}
		binding := cfg.Projects[index].AgentBindings[root.agent]
		binding.Skills = appendUnique(binding.Skills, id)
		cfg.Projects[index].AgentBindings[root.agent] = binding
		return
	}
}

func appendSkill(cfg *config.Config, skill config.Skill) {
	if _, ok := findSkill(cfg, skill.ID); !ok {
		cfg.Library.Skills = append(cfg.Library.Skills, skill)
	}
}

func findSkill(cfg *config.Config, id string) (config.Skill, bool) {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == id {
			return skill, true
		}
	}
	return config.Skill{}, false
}

func validateMutation(cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return validateEffective(cfg)
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func removeCompleted(cfg *config.Config, completed []string) {
	done := make(map[string]struct{}, len(completed))
	for _, id := range completed {
		done[id] = struct{}{}
	}
	kept := cfg.PendingOperations[:0]
	for _, operation := range cfg.PendingOperations {
		if _, ok := done[operation.ID]; !ok {
			kept = append(kept, operation)
		}
	}
	cfg.PendingOperations = kept
}

func cloneConfig(cfg *config.Config) *config.Config {
	clone := *cfg
	clone.Library.Skills = append([]config.Skill(nil), cfg.Library.Skills...)
	clone.Agents = make(map[string]config.Binding, len(cfg.Agents))
	for name, binding := range cfg.Agents {
		clone.Agents[name] = config.Binding{Skills: append([]string(nil), binding.Skills...), Presets: append([]string(nil), binding.Presets...)}
	}
	clone.Projects = append([]config.Project(nil), cfg.Projects...)
	for index := range clone.Projects {
		clone.Projects[index].Skills = append([]string(nil), cfg.Projects[index].Skills...)
		clone.Projects[index].Presets = append([]string(nil), cfg.Projects[index].Presets...)
		clone.Projects[index].Agents = append([]string(nil), cfg.Projects[index].Agents...)
		clone.Projects[index].AgentBindings = make(map[string]config.Binding, len(cfg.Projects[index].AgentBindings))
		for name, binding := range cfg.Projects[index].AgentBindings {
			clone.Projects[index].AgentBindings[name] = config.Binding{Skills: append([]string(nil), binding.Skills...), Presets: append([]string(nil), binding.Presets...)}
		}
	}
	clone.PendingOperations = append([]config.PendingOperation(nil), cfg.PendingOperations...)
	return &clone
}
