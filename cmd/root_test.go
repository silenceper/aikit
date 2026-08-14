package cmd

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type fakeService struct {
	update      app.Result
	updateFunc  func(app.UpdateRequest) (app.Result, error)
	updates     []app.UpdateRequest
	snapshot    app.Snapshot
	preview     app.AddPreview
	last        string
	lastAdd     app.AddRequest
	lastProject app.ProjectEditRequest
}

type fakeMigration struct {
	scan    app.ScanResult
	migrate app.MigrateResult
}

func (f fakeMigration) Scan(context.Context, app.ScanRequest) (app.ScanResult, error) {
	return f.scan, nil
}

func (f fakeMigration) Inventory(context.Context, app.InventoryRequest) <-chan app.InventoryEvent {
	events := make(chan app.InventoryEvent)
	close(events)
	return events
}

func (f fakeMigration) Migrate(context.Context, app.MigrateRequest) (app.MigrateResult, error) {
	return f.migrate, nil
}

func (f *fakeService) Snapshot(context.Context, app.StatusRequest) (app.Snapshot, error) {
	f.last = "status"
	return f.snapshot, nil
}
func (f *fakeService) PreviewAdd(context.Context, app.AddPreviewRequest) (app.AddPreview, error) {
	return f.preview, nil
}
func (f *fakeService) PreviewBinding(context.Context, app.BindingPreviewRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, nil
}
func (f *fakeService) PreviewRemove(context.Context, app.RemoveRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, nil
}
func (f *fakeService) PreviewPreset(context.Context, app.PresetPreviewRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, nil
}
func (f *fakeService) PreviewPresetMutation(context.Context, app.PresetMutationRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, nil
}
func (f *fakeService) SkillDetail(context.Context, string) (app.SkillDetail, error) {
	return app.SkillDetail{}, nil
}
func (f *fakeService) Configuration(context.Context) (app.ConfigurationDetail, error) {
	return app.ConfigurationDetail{}, nil
}
func (f *fakeService) ValidateConfiguration(context.Context) (app.ConfigurationValidation, error) {
	return app.ConfigurationValidation{Valid: true}, nil
}
func (f *fakeService) Add(_ context.Context, request app.AddRequest) (app.Result, error) {
	f.last = "add"
	f.lastAdd = request
	return app.Result{}, nil
}
func (f *fakeService) Remove(context.Context, app.RemoveRequest) (app.Result, error) {
	f.last = "remove"
	return app.Result{}, nil
}
func (f *fakeService) Enable(context.Context, app.BindingRequest) (app.Result, error) {
	f.last = "enable"
	return app.Result{}, nil
}
func (f *fakeService) Disable(context.Context, app.BindingRequest) (app.Result, error) {
	f.last = "disable"
	return app.Result{}, nil
}
func (f *fakeService) PutPreset(context.Context, app.PresetRequest) (app.Result, error) {
	f.last = "preset"
	return app.Result{}, nil
}
func (f *fakeService) RemovePreset(context.Context, app.PresetRemoveRequest) (app.Result, error) {
	return app.Result{}, nil
}
func (f *fakeService) Sync(context.Context, app.SyncRequest) (app.Result, error) {
	f.last = "sync"
	return app.Result{}, nil
}
func (f *fakeService) Update(_ context.Context, request app.UpdateRequest) (app.Result, error) {
	f.last = "update"
	f.updates = append(f.updates, request)
	if f.updateFunc != nil {
		return f.updateFunc(request)
	}
	return f.update, nil
}
func (f *fakeService) EditProject(_ context.Context, request app.ProjectEditRequest) (app.Result, error) {
	f.last = "project"
	f.lastProject = request
	return app.Result{}, nil
}
func (f *fakeService) PreviewProjectEdit(context.Context, app.ProjectEditRequest) (app.ProjectEditPreview, error) {
	return app.ProjectEditPreview{}, nil
}
func (f *fakeService) PreviewProjectRemove(context.Context, app.ProjectRemoveRequest) (app.MutationPreview, error) {
	return app.MutationPreview{}, nil
}
func (f *fakeService) RemoveProject(context.Context, app.ProjectRemoveRequest) (app.Result, error) {
	return app.Result{}, nil
}
func (f *fakeService) Batch(context.Context, app.BatchRequest) (app.BatchResult, error) {
	return app.BatchResult{}, nil
}
func (f *fakeService) MutatePreset(context.Context, app.PresetMutationRequest) (app.Result, error) {
	return app.Result{}, nil
}
func (f *fakeService) Compare(context.Context, app.CompareRequest) (app.CompareResult, error) {
	return app.CompareResult{}, nil
}
func (f *fakeService) PreviewRecovery(context.Context, app.RecoveryRequest) (app.RecoveryPreview, error) {
	return app.RecoveryPreview{}, nil
}
func (f *fakeService) ResumeRecovery(context.Context, app.RecoveryRequest) (app.RecoveryResult, error) {
	return app.RecoveryResult{}, nil
}
func (f *fakeService) RollbackRecovery(context.Context, app.RecoveryRequest) (app.RecoveryResult, error) {
	return app.RecoveryResult{}, nil
}

func TestCommandTreeHasOnlySupportedTopLevelCommands(t *testing.T) {
	root := NewRoot(Dependencies{Service: &fakeService{}, IsTTY: func() bool { return false }})
	want := map[string]bool{"add": true, "list": true, "remove": true, "enable": true, "disable": true, "preset": true, "project": true, "sync": true, "status": true, "update": true, "scan": true, "migrate": true, "version": true}
	for _, command := range root.Commands() {
		delete(want, command.Name())
		if command.Name() == "catalog" || command.Name() == "init" || command.Name() == "publish" {
			t.Fatalf("legacy command %q is registered", command.Name())
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing commands: %v", want)
	}
}

func TestMissingArgumentsRouteTTYAndRejectNonTTY(t *testing.T) {
	launched := ""
	root := NewRoot(Dependencies{Service: &fakeService{}, IsTTY: func() bool { return true }, LaunchTUI: func(_ context.Context, view string) error { launched = view; return nil }})
	root.SetArgs([]string{"add"})
	if err := root.Execute(); err != nil || launched != "add" {
		t.Fatalf("TTY add = view %q, err %v", launched, err)
	}

	root = NewRoot(Dependencies{Service: &fakeService{}, IsTTY: func() bool { return false }})
	root.SetArgs([]string{"add"})
	if err := root.Execute(); err == nil {
		t.Fatal("non-TTY missing add source succeeded")
	}
}

func TestNoArgsTTYLaunchesMainView(t *testing.T) {
	view := ""
	root := NewRoot(Dependencies{Service: &fakeService{}, IsTTY: func() bool { return true }, LaunchTUI: func(_ context.Context, selected string) error { view = selected; return nil }})
	root.SetArgs(nil)
	if err := root.Execute(); err != nil || view != "main" {
		t.Fatalf("root TUI = %q, %v", view, err)
	}
}

func TestUpdateAvailableMapsToExitCodeTwo(t *testing.T) {
	service := &fakeService{update: app.Result{Exit: app.ExitUpdatesAvailable}}
	root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--check"})
	err := root.Execute()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 2 {
		t.Fatalf("update error = %#v, want exit 2", err)
	}
}

func TestAddSingleRootCandidateSelectsByName(t *testing.T) {
	service := &fakeService{preview: app.AddPreview{Candidates: []app.Candidate{{Name: "demo"}}}}
	root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"add", "/tmp/demo"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(service.lastAdd.Skills) != 1 || service.lastAdd.Skills[0] != "demo" {
		t.Fatalf("add selections = %#v, want demo", service.lastAdd.Skills)
	}
}

func TestScanAndMigrateTextOutputIncludesActionableIssues(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{name: "scan", args: []string{"scan"}, want: []string{"permission denied", "local/demo", "broken skill"}},
		{name: "migrate", args: []string{"migrate"}, want: []string{"legacy conflict", "Imported 1", "failed 1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			migration := fakeMigration{
				scan: app.ScanResult{
					Items:    []app.ScanItem{{Origin: "g/cursor", Skill: config.Skill{ID: "local/demo"}, Error: "broken skill"}},
					Warnings: []string{"permission denied"}, Exit: app.ExitPartial,
				},
				migrate: app.MigrateResult{Imported: 1, Failed: 1, Warnings: []string{"legacy conflict"}, Exit: app.ExitPartial},
			}
			output := &bytes.Buffer{}
			root := NewRoot(Dependencies{Service: &fakeService{}, Migration: migration, IsTTY: func() bool { return false }})
			root.SetOut(output)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(test.args)
			_ = root.Execute()
			for _, value := range test.want {
				if !strings.Contains(output.String(), value) {
					t.Fatalf("output %q does not contain %q", output.String(), value)
				}
			}
		})
	}
}

func TestUpdateYesChecksThenSuppliesConfirmationToken(t *testing.T) {
	oldOID := strings.Repeat("a", 40)
	newOID := strings.Repeat("b", 40)
	ref := &config.Ref{Kind: "branch", Value: "main"}
	report := updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/repo/demo", Current: oldOID, Remote: newOID, State: updatecheck.StateUpdateAvailable}}}
	service := &fakeService{snapshot: app.Snapshot{Config: config.Config{Library: config.Library{Skills: []config.Skill{{ID: "acme/repo/demo", Name: "demo", Source: "github.com/acme/repo", Ref: ref, Resolved: oldOID}}}}}}
	service.updateFunc = func(request app.UpdateRequest) (app.Result, error) {
		if len(service.updates) == 1 {
			return app.Result{Updates: report, Exit: app.ExitUpdatesAvailable}, nil
		}
		expected, ok := request.Expected["acme/repo/demo"]
		if !ok {
			return app.Result{}, errors.New("missing confirmation token")
		}
		if expected.Resolved != oldOID || expected.Remote != newOID || expected.Ref != ref {
			return app.Result{}, errors.New("incorrect confirmation token")
		}
		return app.Result{Updates: report, Changed: true}, nil
	}
	root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", "acme/repo/demo", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(service.updates) != 2 || !service.updates[0].CheckOnly || !service.updates[0].Refresh || !service.updates[1].Confirmed {
		t.Fatalf("update requests = %#v", service.updates)
	}
}

func TestExplicitRefUpdateUsesCurrentTokenWithoutRemoteConstraint(t *testing.T) {
	oldOID := strings.Repeat("a", 40)
	current := &config.Ref{Kind: "branch", Value: "main"}
	service := &fakeService{snapshot: app.Snapshot{Config: config.Config{Library: config.Library{Skills: []config.Skill{{ID: "acme/repo/demo", Name: "demo", Source: "github.com/acme/repo", Ref: current, Resolved: oldOID}}}}}}
	service.updateFunc = func(request app.UpdateRequest) (app.Result, error) {
		if len(service.updates) == 1 {
			return app.Result{Updates: updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/repo/demo", Current: oldOID, Remote: strings.Repeat("b", 40), State: updatecheck.StateUpdateAvailable}}}}, nil
		}
		expected := request.Expected["acme/repo/demo"]
		if expected.Ref != current || expected.Resolved != oldOID || expected.Remote != "" {
			return app.Result{}, errors.New("explicit ref token must not constrain target object")
		}
		return app.Result{}, nil
	}
	root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", "acme/repo/demo", "--ref", "tag:v2", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(service.updates) != 2 {
		t.Fatalf("update requests = %d, want check and apply", len(service.updates))
	}
}

func TestListFiltersByAgentProjectAndPreset(t *testing.T) {
	service := &fakeService{snapshot: app.Snapshot{Config: config.Config{
		Library:  config.Library{Skills: []config.Skill{{ID: "id/agent", Name: "agent"}, {ID: "id/project", Name: "project"}, {ID: "id/preset", Name: "preset"}, {ID: "id/other", Name: "other"}}},
		Agents:   map[string]config.Binding{"cursor": {Skills: []string{"id/agent"}}},
		Presets:  []config.Preset{{Name: "base", Skills: []string{"id/preset"}}},
		Projects: []config.Project{{Name: "demo", Binding: config.Binding{Skills: []string{"id/project"}}}},
	}}}
	for _, test := range []struct {
		flag, value, want string
	}{
		{flag: "--agent", value: "cursor", want: "id/agent"},
		{flag: "--project", value: "demo", want: "id/project"},
		{flag: "--preset", value: "base", want: "id/preset"},
	} {
		output := &bytes.Buffer{}
		root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
		root.SetOut(output)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"list", "--offline", test.flag, test.value})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), test.want) || strings.Contains(output.String(), "id/other") {
			t.Fatalf("%s output = %q", test.flag, output.String())
		}
	}
}

func TestProjectAddDefaultsNameToDirectoryBase(t *testing.T) {
	service := &fakeService{}
	path := filepath.Join(t.TempDir(), "demo-project")
	root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"project", "add", path, "--agent", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if service.lastProject.Name != "demo-project" {
		t.Fatalf("project name = %q", service.lastProject.Name)
	}
}

func TestTTYProjectPathEditRequiresYesWithoutLosingContext(t *testing.T) {
	service := &fakeService{}
	root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return true }})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"project", "edit", "demo", "--path", "/tmp/moved"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") || service.last == "project" {
		t.Fatalf("project edit error=%v last=%q", err, service.last)
	}
}

func TestTextStatusAndUpdateIncludeActionableDetails(t *testing.T) {
	oldOID := strings.Repeat("a", 40)
	newOID := strings.Repeat("b", 40)
	service := &fakeService{
		snapshot: app.Snapshot{
			Status:  status.Report{LibrarySkills: 1, Items: []status.Item{{Kind: status.Conflict, Path: "/tmp/demo", SkillID: "acme/repo/demo", Message: "occupied"}}},
			Updates: updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/repo/demo", Current: oldOID, Remote: newOID, State: updatecheck.StateUpdateAvailable}}, Warnings: []string{"fetch warning"}},
			Exit:    app.ExitPartial,
		},
		update: app.Result{Updates: updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/repo/demo", Current: oldOID, Remote: newOID, State: updatecheck.StateUpdateAvailable}}, Warnings: []string{"fetch warning"}}, Exit: app.ExitUpdatesAvailable},
	}
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"status", "--offline"}, want: []string{"conflict", "acme/repo/demo", "/tmp/demo", "occupied", "update-available", oldOID[:12], newOID[:12], "fetch warning"}},
		{args: []string{"update", "--check"}, want: []string{"acme/repo/demo", "update-available", oldOID[:12], newOID[:12], "fetch warning"}},
	} {
		output := &bytes.Buffer{}
		root := NewRoot(Dependencies{Service: service, IsTTY: func() bool { return false }})
		root.SetOut(output)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(test.args)
		_ = root.Execute()
		for _, want := range test.want {
			if !strings.Contains(output.String(), want) {
				t.Fatalf("%v output %q does not contain %q", test.args, output.String(), want)
			}
		}
	}
}

func TestTTYCommandsWithUntransferableContextRequireExplicitFlags(t *testing.T) {
	launched := false
	service := &fakeService{preview: app.AddPreview{Candidates: []app.Candidate{{Name: "one"}, {Name: "two"}}}}
	deps := Dependencies{Service: service, IsTTY: func() bool { return true }, LaunchTUI: func(context.Context, string) error { launched = true; return nil }}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"add", "/tmp/many"}, want: "--skill"},
		{args: []string{"update", "acme/repo/demo", "--ref", "tag:v2"}, want: "--yes"},
	} {
		root := NewRoot(deps)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(test.args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%v error = %v", test.args, err)
		}
	}
	if launched {
		t.Fatal("context-bearing commands launched a context-free TUI")
	}
}
