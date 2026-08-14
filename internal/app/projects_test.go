package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestPreviewProjectRemoveIsReadOnlyAndReturnsExactCleanup(t *testing.T) {
	application, paths, _, projectPath := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	store := config.Store{Paths: paths}
	before, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(projectPath, ".cursor", "skills", "demo")
	preview, err := application.PreviewProjectRemove(context.Background(), ProjectRemoveRequest{Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("project removal preview mutated config:\nbefore=%+v\nafter=%+v", before, after)
	}
	if _, err := os.Lstat(managed); err != nil {
		t.Fatalf("project removal preview mutated managed link: %v", err)
	}
	if !preview.RequiresConfirmation || !strings.Contains(preview.Summary, `"demo"`) || len(preview.AffectedScopes) == 0 {
		t.Fatalf("project removal preview lacks exact identity/scopes: %+v", preview)
	}
	found := false
	for _, action := range preview.Plan.Actions {
		found = found || action.Path == managed
	}
	if !found {
		t.Fatalf("project removal preview lacks managed cleanup path %q: %+v", managed, preview.Plan)
	}
}

func TestProjectPathRebindCheckpointsCleanupThenMovesManagedView(t *testing.T) {
	application, paths, _, oldPath := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	oldLink := filepath.Join(oldPath, ".cursor", "skills", "demo")
	assertLink(t, oldLink)
	newPath := filepath.Join(t.TempDir(), "moved")
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := application.EditProject(context.Background(), ProjectEditRequest{Project: "demo", Path: newPath, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(oldLink); !os.IsNotExist(err) {
		t.Fatalf("old link remains: %v", err)
	}
	assertLink(t, filepath.Join(newPath, ".cursor", "skills", "demo"))
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	canonicalNewPath, err := filepath.EvalSymlinks(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Projects[0].Path != canonicalNewPath || len(cfg.PendingOperations) != 0 {
		t.Fatalf("rebind ledger = %+v", cfg)
	}
}

func TestProjectRemoveAgentCleansOldManagedLinks(t *testing.T) {
	application, paths, _, project := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(project, ".cursor", "skills", "demo")
	assertLink(t, linkPath)
	if _, err := application.EditProject(context.Background(), ProjectEditRequest{Project: "demo", RemoveAgents: []string{"cursor"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("removed agent link remains: %v", err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects[0].Agents) != 0 || len(cfg.Projects[0].AgentBindings) != 0 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("agent removal ledger = %+v", cfg.Projects[0])
	}
}

func TestProjectAddRejectsCanonicalPathDuplicate(t *testing.T) {
	application, _, _, project := testApp(t)
	_, err := application.EditProject(context.Background(), ProjectEditRequest{Name: "duplicate", Path: filepath.Join(project, "."), AddAgents: []string{"codex"}})
	if err == nil {
		t.Fatal("duplicate canonical project path accepted")
	}
}

func TestProjectRemoveLeavesUserContentAndRemovesOnlyManagedLink(t *testing.T) {
	application, paths, _, project := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	unmanaged := filepath.Join(project, ".cursor", "skills", "mine")
	if err := os.MkdirAll(unmanaged, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := application.RemoveProject(context.Background(), ProjectRemoveRequest{Project: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatalf("unmanaged content changed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".cursor", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("managed link remains: %v", err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 0 || len(cfg.PendingOperations) != 0 {
		t.Fatalf("project removal ledger = %+v", cfg)
	}
}

func TestProjectUnknownContentSurvivesRebindAndConcurrentReplacement(t *testing.T) {
	t.Run("unknown entries and external symlink", func(t *testing.T) {
		application, _, _, oldProject := testApp(t)
		if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
			t.Fatal(err)
		}
		overlay := filepath.Join(oldProject, ".cursor", "skills")
		unknownDir := filepath.Join(overlay, "unknown-dir")
		unknownFile := filepath.Join(overlay, "unknown.txt")
		outside := filepath.Join(t.TempDir(), "outside.txt")
		external := filepath.Join(overlay, "external")
		if err := os.MkdirAll(unknownDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for path, content := range map[string]string{unknownFile: "keep-file", outside: "keep-outside"} {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink(outside, external); err != nil {
			t.Fatal(err)
		}
		newProject := filepath.Join(t.TempDir(), "new-project")
		if err := os.MkdirAll(newProject, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := application.EditProject(context.Background(), ProjectEditRequest{Project: "demo", Path: newProject, Confirmed: true}); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(unknownFile); err != nil || string(got) != "keep-file" {
			t.Fatalf("unknown file changed: %q %v", got, err)
		}
		if info, err := os.Stat(unknownDir); err != nil || !info.IsDir() {
			t.Fatalf("unknown directory changed: %v", err)
		}
		if target, err := os.Readlink(external); err != nil || target != outside {
			t.Fatalf("external symlink changed: %q %v", target, err)
		}
		if got, err := os.ReadFile(outside); err != nil || string(got) != "keep-outside" {
			t.Fatalf("outside target changed: %q %v", got, err)
		}
	})

	t.Run("managed link replaced after plan", func(t *testing.T) {
		application, paths, _, oldProject := testApp(t)
		if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
			t.Fatal(err)
		}
		managed := filepath.Join(oldProject, ".cursor", "skills", "demo")
		realRecover := application.deps.Recover
		replaced := false
		application.deps.Recover = func(root string, operations []config.PendingOperation, selector link.Selector, dryRun bool) link.Result {
			if len(operations) > 0 && !dryRun && !replaced {
				replaced = true
				if err := os.Remove(managed); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(managed, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(managed, "unknown.txt"), []byte("keep"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			return realRecover(root, operations, selector, dryRun)
		}
		newProject := filepath.Join(t.TempDir(), "new-project")
		if err := os.MkdirAll(newProject, 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := application.EditProject(context.Background(), ProjectEditRequest{Project: "demo", Path: newProject, Confirmed: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Exit != ExitPartial {
			t.Fatalf("concurrent replacement result: %+v", result)
		}
		if got, err := os.ReadFile(filepath.Join(managed, "unknown.txt")); err != nil || string(got) != "keep" {
			t.Fatalf("concurrent replacement changed: %q %v", got, err)
		}
		cfg, err := (config.Store{Paths: paths}).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.PendingOperations) != 1 {
			t.Fatalf("failed cleanup journal was lost: %+v", cfg.PendingOperations)
		}
	})
}
