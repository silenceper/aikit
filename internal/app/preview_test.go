package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestPreviewBindingScopesAndPlansAreReadOnly(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	cases := []struct {
		name    string
		request BindingPreviewRequest
		prepare func(*testing.T, *config.Config)
		want    config.Scope
	}{
		{
			name:    "global skill enable",
			request: BindingPreviewRequest{Binding: BindingRequest{SkillID: "local/demo", Agent: "cursor"}, Enable: true},
			want:    config.Scope{Agent: "cursor"},
		},
		{
			name:    "project common enable",
			request: BindingPreviewRequest{Binding: BindingRequest{SkillID: "local/demo", Project: "demo"}, Enable: true},
			want:    config.Scope{Project: "demo", ProjectPath: projectPath, Agent: "cursor"},
		},
		{
			name:    "project agent disable",
			request: BindingPreviewRequest{Binding: BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}},
			prepare: func(t *testing.T, cfg *config.Config) {
				cfg.Projects[0].AgentBindings["cursor"] = config.Binding{Skills: []string{"local/demo"}}
				managed := filepath.Join(projectPath, ".cursor", "skills", "demo")
				if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(paths.LibrarySkills, "local", "demo"), managed); err != nil {
					t.Fatal(err)
				}
			},
			want: config.Scope{Project: "demo", ProjectPath: projectPath, Agent: "cursor"},
		},
		{
			name:    "preset application",
			request: BindingPreviewRequest{Binding: BindingRequest{Preset: "review", Agent: "cursor"}, Enable: true},
			prepare: func(_ *testing.T, cfg *config.Config) {
				cfg.Presets = []config.Preset{{Name: "review", Skills: []string{"local/demo"}}}
			},
			want: config.Scope{Agent: "cursor"},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := (config.Store{Paths: paths}).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cfg.Agents = map[string]config.Binding{}
			cfg.Presets = nil
			cfg.Projects[0].Binding = config.Binding{}
			cfg.Projects[0].AgentBindings = map[string]config.Binding{}
			if test.prepare != nil {
				test.prepare(t, cfg)
			}
			if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, paths.Home, userHome, projectPath)
			preview, err := application.PreviewBinding(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Title == "" || preview.Summary == "" || !preview.RequiresConfirmation {
				t.Fatalf("incomplete preview: %+v", preview)
			}
			if !containsScope(preview.AffectedScopes, test.want) {
				t.Fatalf("affected scopes = %+v, want %+v", preview.AffectedScopes, test.want)
			}
			if len(preview.Plan.Actions) == 0 {
				t.Fatalf("preview has no exact link actions: %+v", preview.Plan)
			}
			after := snapshotTree(t, paths.Home, userHome, projectPath)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("preview mutated filesystem\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestPreviewAddLocalIsStrictlyReadOnlyAndGitDeclaresNetworkRequirement(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: local-preview\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath, source)
	preview, err := application.PreviewAdd(context.Background(), AddPreviewRequest{Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if preview.NetworkRequired || len(preview.Candidates) != 1 || preview.Candidates[0].Name != "local-preview" {
		t.Fatalf("local preview = %+v", preview)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath, source); !reflect.DeepEqual(before, after) {
		t.Fatalf("local add preview mutated durable state\nbefore=%v\nafter=%v", before, after)
	}

	gitPreview, err := application.PreviewAdd(context.Background(), AddPreviewRequest{Source: "https://example.test/acme/repo.git"})
	if err != nil {
		t.Fatal(err)
	}
	if !gitPreview.NetworkRequired || len(gitPreview.Candidates) != 0 {
		t.Fatalf("git preview did not declare deferred network requirement: %+v", gitPreview)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath, source); !reflect.DeepEqual(before, after) {
		t.Fatalf("git add preview mutated durable state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPreviewRemoveReportsReferencesAndForceWithoutWrites(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{"local/demo"}}}
	cfg.Presets = []config.Preset{{Name: "review", Skills: []string{"local/demo"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(userHome, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(paths.LibrarySkills, "local", "demo"), managed); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	preview, err := application.PreviewRemove(context.Background(), RemoveRequest{SkillID: "local/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresForce || !preview.RequiresConfirmation || len(preview.References) != 2 {
		t.Fatalf("remove preview = %+v", preview)
	}
	if len(preview.Plan.Actions) != 1 || preview.Plan.Actions[0].Kind != link.ActionRecover || preview.Plan.Actions[0].Path != managed {
		t.Fatalf("remove plan = %+v", preview.Plan)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("remove preview mutated filesystem\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPreviewRemoveExcludesUnrelatedManagedOrphan(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	otherRoot := filepath.Join(paths.LibrarySkills, "local", "other")
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherRoot, "SKILL.md"), []byte("---\nname: other\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, config.Skill{ID: "local/other", Name: "other", Hash: "other"})
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{"local/demo"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(userHome, ".cursor", "skills")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"demo": filepath.Join(paths.LibrarySkills, "local", "demo"), "orphan": otherRoot} {
		if err := os.Symlink(target, filepath.Join(overlay, name)); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	preview, err := application.PreviewRemove(context.Background(), RemoveRequest{SkillID: "local/demo"})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range preview.Plan.Actions {
		if action.Path == filepath.Join(overlay, "orphan") {
			t.Fatalf("skill removal included unrelated orphan: %+v", preview.Plan)
		}
	}
	if len(preview.Plan.Actions) != 1 || preview.Plan.Actions[0].Path != filepath.Join(overlay, "demo") {
		t.Fatalf("skill-specific cleanup plan = %+v", preview.Plan)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("remove preview mutated state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPreviewRemoveIncludesPendingOnlyScopeAndMatchesMutationPlan(t *testing.T) {
	application, paths, userHome, _ := previewTestApp(t)
	pendingTarget := filepath.Join(userHome, ".cursor", "skills", "demo")
	op, err := link.NewCleanupOperation("pending-demo", config.Scope{Agent: "cursor"}, pendingTarget, "local/demo", "pending cleanup")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	preview, err := application.PreviewRemove(context.Background(), RemoveRequest{SkillID: "local/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Plan.Actions) != 1 || preview.Plan.Actions[0].Operation != op.ID || !containsScope(preview.AffectedScopes, config.Scope{Agent: "cursor"}) {
		t.Fatalf("pending-only removal preview = %+v", preview)
	}

	application.deps.Execute = link.Execute
	application.deps.Recover = link.Recover
	application.deps.LibraryRecovery = &recordingRecovery{}
	if _, err := application.Remove(context.Background(), RemoveRequest{SkillID: "local/demo", Force: true}); err == nil {
		t.Fatal("remove implicitly recovered a pending operation")
	}
}

func TestPreviewBindingSurfacesPlanConflictsWithoutWrites(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	conflict := filepath.Join(userHome, ".cursor", "skills", "demo")
	if err := os.MkdirAll(conflict, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflict, "mine.txt"), []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	preview, err := application.PreviewBinding(context.Background(), BindingPreviewRequest{
		Binding: BindingRequest{SkillID: "local/demo", Agent: "cursor"}, Enable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Conflicts) != 1 || len(preview.Plan.Issues) != 1 || preview.Plan.Issues[0].Path != conflict {
		t.Fatalf("conflicts were not preserved: %+v", preview)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("conflict preview mutated filesystem\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPreviewBindingSurfacesMissingProjectWarning(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Projects[0].Path = filepath.Join(t.TempDir(), "missing")
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, projectPath)
	preview, err := application.PreviewBinding(context.Background(), BindingPreviewRequest{
		Binding: BindingRequest{SkillID: "local/demo", Project: "demo"}, Enable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Warnings) != 1 || len(preview.Plan.Warnings) != 1 {
		t.Fatalf("warnings were not preserved: %+v", preview)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("warning preview mutated filesystem\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPreviewPresetRemoveAndMemberChangeAreReadOnly(t *testing.T) {
	application, paths, userHome, projectPath := previewTestApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Presets = []config.Preset{{Name: "review", Skills: []string{"local/demo"}}}
	cfg.Agents = map[string]config.Binding{"cursor": {Presets: []string{"review"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	before := snapshotTree(t, paths.Home, userHome, projectPath)
	removed, err := application.PreviewPreset(context.Background(), PresetPreviewRequest{Name: "review", Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	if !removed.RequiresForce || len(removed.References) != 1 || len(removed.AffectedScopes) == 0 {
		t.Fatalf("preset removal preview = %+v", removed)
	}
	changed, err := application.PreviewPreset(context.Background(), PresetPreviewRequest{Name: "review", Skills: []string{"local/demo"}, Remove: true})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Title == "" || !changed.RequiresConfirmation {
		t.Fatalf("preset member preview = %+v", changed)
	}
	if after := snapshotTree(t, paths.Home, userHome, projectPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("preset preview mutated filesystem\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPreviewPresetMutationRenameReferencesAndApplyPlanAreReadOnly(t *testing.T) {
	application, paths, _, _ := testApp(t)
	store := config.Store{Paths: paths}
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Presets = []config.Preset{{Name: "review", Skills: []string{"local/demo"}}}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]config.Binding)
	}
	binding := cfg.Agents["cursor"]
	binding.Presets = []string{"review"}
	cfg.Agents["cursor"] = binding
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Load(context.Background())

	rename, err := application.PreviewPresetMutation(context.Background(), PresetMutationRequest{Operation: PresetRename, Name: "review", NewName: "renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rename.References) == 0 || len(rename.AffectedScopes) == 0 || len(rename.Plan.Actions) == 0 || !rename.RequiresConfirmation {
		t.Fatalf("rename preview lost references/scopes: %+v", rename)
	}
	if !strings.Contains(rename.Summary, `"review"`) || !strings.Contains(rename.Summary, `"renamed"`) {
		t.Fatalf("rename preview does not identify old -> new: %q", rename.Summary)
	}
	apply, err := application.PreviewPresetMutation(context.Background(), PresetMutationRequest{Operation: PresetApply, Name: "review", Binding: BindingRequest{Preset: "review", Project: "demo", Agent: "cursor"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(apply.AffectedScopes) == 0 || len(apply.Plan.Actions) == 0 {
		t.Fatalf("apply preview lost exact target plan: %+v", apply)
	}
	if !strings.Contains(apply.Summary, "Project / demo / cursor") {
		t.Fatalf("apply preview does not identify exact target: %q", apply.Summary)
	}
	after, _ := store.Load(context.Background())
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("preset mutation preview wrote config:\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestPreviewProjectCreateAndRebindAreReadOnly(t *testing.T) {
	application, paths, userHome, oldPath := previewTestApp(t)
	newProject := filepath.Join(t.TempDir(), "new-project")
	if err := os.MkdirAll(newProject, 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, paths.Home, userHome, oldPath, newProject)
	created, err := application.PreviewProjectEdit(context.Background(), ProjectEditRequest{Name: "new", Path: newProject, AddAgents: []string{"cursor"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Next.Actions) != 0 || len(created.Cleanup.Actions) != 0 {
		t.Fatalf("empty project unexpectedly planned links: %+v", created)
	}

	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Projects[0].AgentBindings == nil {
		cfg.Projects[0].AgentBindings = map[string]config.Binding{}
	}
	cfg.Projects[0].AgentBindings["cursor"] = config.Binding{Skills: []string{"local/demo"}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	oldLink := filepath.Join(oldPath, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(oldLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(paths.LibrarySkills, "local", "demo"), oldLink); err != nil {
		t.Fatal(err)
	}
	before = snapshotTree(t, paths.Home, userHome, oldPath, newProject)
	rebound, err := application.PreviewProjectEdit(context.Background(), ProjectEditRequest{Project: "demo", Path: newProject})
	if err != nil {
		t.Fatal(err)
	}
	if len(rebound.Cleanup.Actions) != 1 || rebound.Cleanup.Actions[0].Path != oldLink || len(rebound.Next.Actions) != 1 {
		t.Fatalf("rebind preview = %+v", rebound)
	}
	if after := snapshotTree(t, paths.Home, userHome, oldPath, newProject); !reflect.DeepEqual(before, after) {
		t.Fatalf("project preview mutated filesystem\nbefore=%v\nafter=%v", before, after)
	}
}

func previewTestApp(t *testing.T) (*App, config.Paths, string, string) {
	t.Helper()
	application, paths, userHome, projectPath := testApp(t)
	application.deps.Execute = func(link.Plan, bool) link.Result { panic("preview executed a link plan") }
	application.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		panic("preview invoked recovery")
	}
	application.deps.LibraryRecovery = panicRecovery{}
	return application, paths, userHome, projectPath
}

type panicRecovery struct{}

func (panicRecovery) RecoverBatches(context.Context, []config.Skill) ([]library.RecoveryIssue, error) {
	panic("preview invoked library recovery")
}

func containsScope(scopes []config.Scope, wanted config.Scope) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func snapshotTree(t *testing.T, roots ...string) []string {
	t.Helper()
	var result []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			record := root + "\x00" + filepath.ToSlash(rel) + "\x00" + info.Mode().String()
			if info.Mode().IsRegular() {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				record += "\x00" + string(content)
			} else if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				record += "\x00" + target
			}
			result = append(result, record)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(result)
	return result
}
