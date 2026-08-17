package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/migrate"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Service    app.Service
	Migration  app.MigrationService
	IsTTY      func() bool
	LaunchTUI  func(context.Context, string) error
	WorkingDir string
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}
func (e *ExitError) Unwrap() error { return e.Err }

var rootCmd = NewRoot(defaultDependencies())

func defaultDependencies() Dependencies {
	paths, err := config.DefaultPaths()
	if err != nil {
		return Dependencies{Service: errorService{err}}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Dependencies{Service: errorService{err}}
	}
	lib := library.Service{LibraryRoot: paths.LibrarySkills, CacheRoot: paths.Cache, Runner: library.CommandRunner{}}
	checker := updatecheck.New(paths.UpdateCache, updatecheck.CommandGitRunner{CacheRoot: paths.Cache, Executor: library.CommandRunner{}})
	service := app.New(app.Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: home, Library: app.NewLibraryService(lib), LibraryRecovery: lib, Updates: checker})
	workingDir, _ := os.Getwd()
	migration := migrate.New(migrate.Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: home, WorkingDir: workingDir, Library: lib})
	deps := Dependencies{Service: service, Migration: migration, WorkingDir: workingDir}
	deps.LaunchTUI = func(ctx context.Context, selected string) error {
		view, action := tui.ViewOverview, tui.ActionNone
		switch selected {
		case "agents":
			view = tui.ViewAgents
		case "projects":
			view = tui.ViewProjects
		case "presets":
			view = tui.ViewPresets
		case "status":
			view = tui.ViewStatus
		case "updates":
			action = tui.ActionUpdate
		case "scan":
			action = tui.ActionScan
		case "add":
			view = tui.ViewLibrary
		}
		return (tui.Launcher{Service: service, Migration: deps.Migration}).Launch(ctx, view, action)
	}
	return deps
}

func NewRoot(deps Dependencies) *cobra.Command {
	if deps.IsTTY == nil {
		deps.IsTTY = func() bool { return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd()) }
	}
	if deps.LaunchTUI == nil {
		deps.LaunchTUI = func(context.Context, string) error { return fmt.Errorf("interactive TUI is unavailable") }
	}
	if deps.WorkingDir == "" {
		deps.WorkingDir, _ = os.Getwd()
	}
	root := &cobra.Command{Use: "aikit", Short: "Manage skills across AI development agents", SilenceUsage: true, SilenceErrors: true}
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		if !deps.IsTTY() {
			return fmt.Errorf("aikit requires a command when stdin is not a TTY")
		}
		return deps.LaunchTUI(cmd.Context(), "main")
	}
	root.PersistentFlags().Bool("json", false, "write machine-readable JSON")
	root.AddCommand(
		newAddCommand(deps), newListCommand(deps), newRemoveCommand(deps),
		newBindingCommand(deps, true), newBindingCommand(deps, false),
		newPresetCommand(deps), newProjectCommand(deps), newSyncCommand(deps),
		newStatusCommand(deps), newUpdateCommand(deps), newScanCommand(deps),
		newMigrateCommand(deps), versionCommand(),
	)
	return root
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exit *ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.Code)
		}
		os.Exit(1)
	}
}

func requireOrTUI(cmd *cobra.Command, deps Dependencies, complete bool, view string) error {
	if complete {
		return nil
	}
	if !deps.IsTTY() {
		return fmt.Errorf("missing required arguments for %s", cmd.CommandPath())
	}
	return deps.LaunchTUI(cmd.Context(), view)
}

func resultError(result app.Result) error {
	if result.Exit == app.ExitUpdatesAvailable {
		return &ExitError{Code: 2}
	}
	if result.Exit == app.ExitPartial {
		return &ExitError{Code: 1, Err: fmt.Errorf("operation completed with issues")}
	}
	return nil
}

func writeValue(cmd *cobra.Command, value any, text string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if !jsonOutput {
		jsonOutput, _ = cmd.Root().PersistentFlags().GetBool("json")
	}
	if jsonOutput {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	if text != "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), text)
		return err
	}
	return nil
}

func parseRef(value string) (*config.Ref, error) {
	if value == "" {
		return nil, nil
	}
	for _, kind := range []string{"branch", "tag", "commit"} {
		prefix := kind + ":"
		if len(value) > len(prefix) && value[:len(prefix)] == prefix {
			return &config.Ref{Kind: kind, Value: value[len(prefix):]}, nil
		}
	}
	return nil, fmt.Errorf("ref must be branch:<name>, tag:<name>, or commit:<object-id>")
}

func absoluteCWD(value string) (string, error) {
	if value == "" {
		value = "."
	}
	return filepath.Abs(value)
}

func inferProject(ctx context.Context, deps Dependencies) (string, error) {
	snapshot, err := deps.Service.Snapshot(ctx, app.StatusRequest{Offline: true})
	if err != nil {
		return "", err
	}
	cwd, err := filepath.EvalSymlinks(deps.WorkingDir)
	if err != nil {
		return "", err
	}
	best, bestLength := "", -1
	for _, project := range snapshot.Config.Projects {
		path, pathErr := filepath.EvalSymlinks(project.Path)
		if pathErr != nil {
			continue
		}
		relative, relErr := filepath.Rel(path, cwd)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && len(path) > bestLength {
			best, bestLength = project.Name, len(path)
		}
	}
	return best, nil
}

type errorService struct{ err error }

func (s errorService) fail() error { return s.err }
func (s errorService) Snapshot(context.Context, app.StatusRequest) (app.Snapshot, error) {
	return app.Snapshot{}, s.fail()
}
func (s errorService) PreviewAdd(context.Context, app.AddPreviewRequest) (app.AddPreview, error) {
	return app.AddPreview{}, s.fail()
}
func (s errorService) PreviewBinding(context.Context, app.BindingPreviewRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, s.fail()
}
func (s errorService) PreviewRemove(context.Context, app.RemoveRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, s.fail()
}
func (s errorService) PreviewPreset(context.Context, app.PresetPreviewRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, s.fail()
}
func (s errorService) PreviewPresetMutation(context.Context, app.PresetMutationRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, s.fail()
}
func (s errorService) SkillDetail(context.Context, string) (app.SkillDetail, error) {
	return app.SkillDetail{}, s.fail()
}
func (s errorService) Configuration(context.Context) (app.ConfigurationDetail, error) {
	return app.ConfigurationDetail{}, s.fail()
}
func (s errorService) ValidateConfiguration(context.Context) (app.ConfigurationValidation, error) {
	return app.ConfigurationValidation{}, s.fail()
}
func (s errorService) Add(context.Context, app.AddRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Remove(context.Context, app.RemoveRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Enable(context.Context, app.BindingRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Disable(context.Context, app.BindingRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) PutPreset(context.Context, app.PresetRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) RemovePreset(context.Context, app.PresetRemoveRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Sync(context.Context, app.SyncRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Update(context.Context, app.UpdateRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) EditProject(context.Context, app.ProjectEditRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) PreviewProjectRegistration(context.Context, app.ProjectRegistrationRequest) (app.ProjectRegistrationPreview, error) {
	return app.ProjectRegistrationPreview{}, s.fail()
}
func (s errorService) PreviewProjectEdit(context.Context, app.ProjectEditRequest) (app.ProjectEditPreview, error) {
	return app.ProjectEditPreview{}, s.fail()
}
func (s errorService) PreviewProjectRemove(context.Context, app.ProjectRemoveRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, s.fail()
}
func (s errorService) RemoveProject(context.Context, app.ProjectRemoveRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Batch(context.Context, app.BatchRequest) (app.BatchResult, error) {
	return app.BatchResult{}, s.fail()
}
func (s errorService) MutatePreset(context.Context, app.PresetMutationRequest) (app.Result, error) {
	return app.Result{}, s.fail()
}
func (s errorService) Compare(context.Context, app.CompareRequest) (app.CompareResult, error) {
	return app.CompareResult{}, s.fail()
}
func (s errorService) PreviewRecovery(context.Context, app.RecoveryRequest) (app.RecoveryPreview, error) {
	return app.RecoveryPreview{}, s.fail()
}
func (s errorService) ResumeRecovery(context.Context, app.RecoveryRequest) (app.RecoveryResult, error) {
	return app.RecoveryResult{}, s.fail()
}
func (s errorService) RollbackRecovery(context.Context, app.RecoveryRequest) (app.RecoveryResult, error) {
	return app.RecoveryResult{}, s.fail()
}
