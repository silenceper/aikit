package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type LibraryService interface {
	PrepareAdd(context.Context, AddPrepareRequest, []config.Skill) (LibraryMutation, error)
	PrepareUpdate(context.Context, []UpdatePrepareItem) (LibraryMutation, error)
	PrepareRemove(context.Context, config.Skill) (LibraryMutation, error)
}

type AddPrepareRequest struct {
	Source     string
	Selections []string
	SourcePath string
	Ref        *config.Ref
	Force      bool
}

type UpdatePrepareItem struct {
	Skill config.Skill
	Ref   *config.Ref
}

type LibraryMutation interface {
	Entries() []config.Skill
	Commit(context.Context) error
	Abort() error
}

type UpdateChecker interface {
	Check(context.Context, []config.Skill, updatecheck.CheckOptions) (updatecheck.CheckReport, error)
}

// LibraryRecovery is the integration seam for crash leftovers created by a
// library batch. It intentionally does not add a new config pending kind.
type LibraryRecovery interface {
	RecoverBatches(context.Context, []config.Skill) ([]library.RecoveryIssue, error)
}

type Dependencies struct {
	Store           config.Store
	Paths           config.Paths
	UserHome        string
	Library         LibraryService
	Updates         UpdateChecker
	LibraryRecovery LibraryRecovery
	Execute         func(link.Plan, bool) link.Result
	Recover         func(string, []config.PendingOperation, link.Selector, bool) link.Result
	Inspect         func(*config.Config, string, string) status.Report
}

type App struct{ deps Dependencies }

func New(deps Dependencies) *App {
	if deps.Paths.Home == "" {
		deps.Paths = deps.Store.Paths
	}
	if deps.Store.Paths.Home == "" {
		deps.Store.Paths = deps.Paths
	}
	service := library.Service{LibraryRoot: deps.Paths.LibrarySkills, CacheRoot: deps.Paths.Cache, Runner: library.CommandRunner{}}
	if deps.Library == nil {
		deps.Library = NewLibraryService(service)
	}
	if deps.LibraryRecovery == nil {
		deps.LibraryRecovery = service
	}
	if deps.Execute == nil {
		deps.Execute = link.Execute
	}
	if deps.Recover == nil {
		deps.Recover = link.Recover
	}
	if deps.Inspect == nil {
		deps.Inspect = status.Inspect
	}
	return &App{deps: deps}
}

func (a *App) Snapshot(ctx context.Context, request StatusRequest) (Snapshot, error) {
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Config: *cfg, Status: a.deps.Inspect(cfg, a.deps.UserHome, a.deps.Paths.LibrarySkills)}
	if !snapshot.Status.Healthy() {
		snapshot.Exit = ExitPartial
	}
	if request.Offline || a.deps.Updates == nil {
		return snapshot, nil
	}
	report, checkErr := a.deps.Updates.Check(ctx, cfg.Library.Skills, updatecheck.CheckOptions{ForceRefresh: request.ForceRefresh})
	snapshot.Updates = report
	if checkErr != nil {
		snapshot.Exit = ExitPartial
		snapshot.Updates.Warnings = append(snapshot.Updates.Warnings, checkErr.Error())
	}
	for _, item := range report.Results {
		if item.State == updatecheck.StateCheckFailed {
			snapshot.Exit = ExitPartial
		}
	}
	// A remote failure is represented in CheckReport. Preserve the filesystem
	// snapshot even when the checker also returns an aggregate error.
	return snapshot, nil
}

func (a *App) beforeMutation(ctx context.Context, ledger []config.Skill) error {
	if a.deps.LibraryRecovery != nil {
		if err := os.MkdirAll(a.deps.Paths.LibrarySkills, 0o755); err != nil {
			return err
		}
		issues, err := a.deps.LibraryRecovery.RecoverBatches(ctx, ledger)
		if err != nil {
			return err
		}
		if len(issues) > 0 {
			return fmt.Errorf("library batch recovery requires attention: %s", issues[0].Detail)
		}
	} else {
		return fmt.Errorf("library recovery service is required for mutations")
	}
	return nil
}

func (a *App) recoverLibrary(ctx context.Context, ledger []config.Skill) error {
	issues, err := a.deps.LibraryRecovery.RecoverBatches(ctx, ledger)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return fmt.Errorf("library batch recovery requires attention: %s", issues[0].Detail)
	}
	return nil
}

func classify(result link.Result) ExitClass {
	if len(result.Failures) > 0 || len(result.Issues) > 0 {
		return ExitPartial
	}
	return ExitOK
}

func completedWithoutFailures(result link.Result) bool {
	return len(result.Failures) == 0 && len(result.Issues) == 0
}

func removeCompleted(cfg *config.Config, ids []string) {
	if len(ids) == 0 {
		return
	}
	done := make(map[string]struct{}, len(ids))
	for _, id := range ids {
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

func resultError(result link.Result) error {
	var values []error
	for _, issue := range result.Issues {
		values = append(values, fmt.Errorf("%s: %s", issue.Kind, issue.Message))
	}
	for _, failure := range result.Failures {
		values = append(values, fmt.Errorf("%s: %s", failure.Kind, failure.Message))
	}
	return errors.Join(values...)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	if value == "" || contains(values, value) {
		return values
	}
	return append(values, value)
}

func removeValue(values []string, value string) []string {
	result := values[:0]
	for _, current := range values {
		if current != value {
			result = append(result, current)
		}
	}
	return result
}

func findProject(cfg *config.Config, name string) (*config.Project, error) {
	for i := range cfg.Projects {
		if cfg.Projects[i].Name == name {
			return &cfg.Projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %q not found", name)
}

func findSkill(cfg *config.Config, idOrName string) (config.Skill, error) {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == idOrName {
			return skill, nil
		}
	}
	var matches []config.Skill
	for _, skill := range cfg.Library.Skills {
		if skill.Name == idOrName {
			matches = append(matches, skill)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return config.Skill{}, fmt.Errorf("skill name %q is ambiguous; use a full id", idOrName)
	}
	return config.Skill{}, fmt.Errorf("skill %q not found", idOrName)
}

func findPreset(cfg *config.Config, name string) (*config.Preset, error) {
	for i := range cfg.Presets {
		if cfg.Presets[i].Name == name {
			return &cfg.Presets[i], nil
		}
	}
	return nil, fmt.Errorf("preset %q not found", name)
}

func validateEffective(cfg *config.Config) error {
	issues := scopeIssues(cfg)
	if len(issues) == 0 {
		return nil
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Message+" ["+strings.Join(issue.IDs, ", ")+"]")
	}
	sort.Strings(parts)
	return fmt.Errorf("effective scope conflict: %s", strings.Join(parts, "; "))
}

func existingDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
