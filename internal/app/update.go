package app

import (
	"context"
	"fmt"

	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) Update(ctx context.Context, request UpdateRequest) (Result, error) {
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return Result{}, err
	}
	if request.Confirmed && !request.Offline && !request.CheckOnly && len(cfg.PendingOperations) > 0 {
		return Result{}, pendingRecoveryError(cfg.PendingOperations)
	}
	if request.Offline {
		report := updatecheck.CheckReport{Results: make([]updatecheck.Result, 0, len(cfg.Library.Skills))}
		for _, skill := range cfg.Library.Skills {
			report.Results = append(report.Results, updatecheck.Result{SkillID: skill.ID, Current: skill.Resolved, State: updatecheck.StateOffline})
		}
		return Result{Updates: report}, nil
	}
	var report updatecheck.CheckReport
	if a.deps.Updates != nil {
		report, err = a.deps.Updates.Check(ctx, cfg.Library.Skills, updatecheck.CheckOptions{ForceRefresh: request.Refresh})
		if err != nil {
			report.Warnings = append(report.Warnings, err.Error())
		}
		if request.CheckOnly {
			return Result{Updates: report, Exit: updateReportExit(report)}, nil
		}
	}
	selected, err := selectUpdates(cfg, request, report)
	if err != nil {
		return Result{Updates: report}, err
	}
	if len(selected) == 0 {
		return Result{Updates: report, Exit: updateReportExit(report)}, nil
	}
	if !request.Confirmed {
		exit := ExitUpdatesAvailable
		if updateReportExit(report) == ExitPartial {
			exit = ExitPartial
		}
		return Result{Updates: report, Exit: exit}, nil
	}
	if a.deps.Library == nil {
		return Result{}, fmt.Errorf("library mutation service is required")
	}
	confirmed := make(map[string]config.Skill, len(selected))
	for _, id := range selected {
		index := skillIndex(cfg, id)
		if index >= 0 {
			confirmed[id] = cfg.Library.Skills[index]
		}
	}
	var output Result
	err = a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		oldConfig := cloneConfig(tx.Config)
		items := make([]UpdatePrepareItem, 0, len(selected))
		for _, id := range selected {
			index := skillIndex(tx.Config, id)
			if index < 0 {
				return fmt.Errorf("skill %q disappeared while waiting for lock", id)
			}
			expected, ok := request.Expected[id]
			if !ok {
				return fmt.Errorf("skill %q is missing its confirmation token", id)
			}
			current := tx.Config.Library.Skills[index]
			if !sameConfirmedSkill(current, confirmed[id]) {
				return fmt.Errorf("skill %q changed after confirmation; check again", id)
			}
			if expected.Resolved != "" && current.Resolved != expected.Resolved {
				return fmt.Errorf("skill %q resolved object changed after confirmation", id)
			}
			if expected.Ref != nil && !sameRef(current.Ref, expected.Ref) {
				return fmt.Errorf("skill %q ref changed after confirmation", id)
			}
			items = append(items, UpdatePrepareItem{Skill: current, Ref: request.Ref})
		}
		mutation, err := a.deps.Library.PrepareUpdate(ctx, items)
		if err != nil {
			return err
		}
		checkpointed := false
		defer func() {
			if !checkpointed {
				_ = mutation.Abort()
			}
		}()
		updated := mutation.Entries()
		if len(updated) != len(selected) {
			return fmt.Errorf("prepared update returned %d entries for %d selections", len(updated), len(selected))
		}
		for _, skill := range updated {
			index := skillIndex(tx.Config, skill.ID)
			if index < 0 {
				return fmt.Errorf("prepared update returned unknown skill %q", skill.ID)
			}
			tx.Config.Library.Skills[index] = skill
			if expected := request.Expected[skill.ID]; expected.Remote != "" && skill.Resolved != expected.Remote {
				return fmt.Errorf("skill %q remote changed after confirmation", skill.ID)
			}
		}
		if err := tx.Config.Validate(); err != nil {
			return err
		}
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		checkpointed = true
		if err := mutation.Commit(ctx); err != nil {
			output = Result{Updates: report, Changed: true, Exit: ExitPartial}
			commitErr := err
			*tx.Config = *cloneConfig(oldConfig)
			if restoreErr := tx.Checkpoint(); restoreErr != nil {
				return fmt.Errorf("commit library update: %v; restore old ledger: %w", commitErr, restoreErr)
			}
			if recoverErr := a.recoverLibrary(ctx, tx.Config.Library.Skills); recoverErr != nil {
				return fmt.Errorf("commit library update: %v; recover old library: %w", commitErr, recoverErr)
			}
			return commitErr
		}
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output.Skills = append(output.Skills, updated...)
		output.Changed = len(updated) > 0
		output.Updates = report
		if updateReportExit(report) == ExitPartial {
			output.Exit = ExitPartial
		}
		return nil
	})
	return output, err
}

func sameConfirmedSkill(current, confirmed config.Skill) bool {
	return current.ID == confirmed.ID && current.Source == confirmed.Source && current.SourcePath == confirmed.SourcePath &&
		current.Resolved == confirmed.Resolved && sameRef(current.Ref, confirmed.Ref)
}

func sameRef(left, right *config.Ref) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Kind == right.Kind && left.Value == right.Value
}

func updateReportExit(report updatecheck.CheckReport) ExitClass {
	exit := ExitOK
	if len(report.Warnings) > 0 {
		exit = ExitPartial
	}
	for _, result := range report.Results {
		switch result.State {
		case updatecheck.StateCheckFailed:
			return ExitPartial
		case updatecheck.StateUpdateAvailable:
			if exit == ExitOK {
				exit = ExitUpdatesAvailable
			}
		}
	}
	return exit
}

func selectUpdates(cfg *config.Config, request UpdateRequest, report updatecheck.CheckReport) ([]string, error) {
	if request.Ref != nil && len(request.SkillIDs) != 1 {
		return nil, fmt.Errorf("--ref requires exactly one skill")
	}
	if len(request.SkillIDs) > 0 {
		available := map[string]struct{}{}
		for _, item := range report.Results {
			if item.State == updatecheck.StateUpdateAvailable {
				available[item.SkillID] = struct{}{}
			}
		}
		result := make([]string, 0, len(request.SkillIDs))
		for _, value := range request.SkillIDs {
			skill, err := findSkill(cfg, value)
			if err != nil {
				return nil, err
			}
			if skill.Source == "" || skill.Ref == nil {
				continue
			}
			if !request.Force && request.Ref == nil && skill.Ref.Kind != "branch" {
				continue
			}
			if !request.Force && request.Ref == nil {
				if _, ok := available[skill.ID]; !ok {
					continue
				}
			}
			result = appendUnique(result, skill.ID)
		}
		return result, nil
	}
	available := map[string]struct{}{}
	for _, item := range report.Results {
		if item.State == updatecheck.StateUpdateAvailable {
			available[item.SkillID] = struct{}{}
		}
	}
	var result []string
	for _, skill := range cfg.Library.Skills {
		if skill.Source == "" || skill.Ref == nil {
			continue
		}
		if request.Force {
			result = append(result, skill.ID)
		} else if _, ok := available[skill.ID]; ok {
			result = append(result, skill.ID)
		}
	}
	return result, nil
}
