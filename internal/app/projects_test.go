package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/silenceper/aikit/pkg/config"
)

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
