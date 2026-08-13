package status_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/pkg/config"
)

func TestInspectIsReadOnlyAndReportsAllCoreCategories(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	library := filepath.Join(root, "aikit", "library", "skills")
	good := filepath.Join(library, "local", "good")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(library, "a", "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Library: config.Library{Skills: []config.Skill{{ID: "local/good", Name: "good"}, {ID: "local/lost", Name: "lost"}, {ID: "a/shared", Name: "same"}, {ID: "b/shared", Name: "same"}}}, Agents: map[string]config.Binding{"cursor": {Skills: []string{"local/good", "local/lost", "a/shared"}}}}
	dir := filepath.Join(home, ".cursor", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "good"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "unmanaged"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(good, filepath.Join(dir, "orphan")); err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{{ID: "c", Kind: config.OperationCleanup, Scope: config.Scope{Agent: "cursor"}, Target: filepath.Join(dir, "old"), SkillID: "local/good"}, {ID: "a", Kind: config.OperationAdopt, Scope: config.Scope{Agent: "cursor"}, Target: filepath.Join(dir, "adopt"), SkillID: "local/good", Temp: filepath.Join(dir, ".aikit-adopt-temp-x"), Backup: filepath.Join(dir, ".aikit-adopt-backup-x"), Original: &config.Fingerprint{Kind: "directory", Hash: "x"}, JournalHash: strings.Repeat("a", 64)}}
	r := status.Inspect(cfg, home, library)
	for _, kind := range []status.Kind{status.Conflict, status.LibraryMissing, status.Missing, status.Unmanaged, status.OrphanedLink, status.PendingCleanup, status.AdoptRecovery} {
		if !contains(r, kind) {
			t.Fatalf("missing %s: %#v", kind, r.Items)
		}
	}
	for _, item := range r.Items {
		if item.Kind == status.AdoptRecovery && (item.Journal == "" || item.DeleteRoot == "" || item.Manifest == "") {
			t.Fatalf("adopt journal recovery paths hidden: %#v", item)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "good")); err != nil {
		t.Fatalf("status changed disk: %v", err)
	}
}

func TestStatusReportsScopeConflictAndSkipsMissingProjectPath(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	library := filepath.Join(root, "library")
	cfg := &config.Config{Library: config.Library{Skills: []config.Skill{{ID: "a/shared", Name: "shared"}, {ID: "b/shared", Name: "shared"}}}, Agents: map[string]config.Binding{"cursor": {Skills: []string{"a/shared"}}}, Projects: []config.Project{{Name: "gone", Path: filepath.Join(root, "gone"), Agents: []string{"cursor"}, Binding: config.Binding{Skills: []string{"b/shared"}}}}}
	r := status.Inspect(cfg, home, library)
	if !contains(r, status.ScopeConflict) {
		t.Fatalf("scope conflict missing: %#v", r.Items)
	}
	if len(r.Warnings) != 1 || r.Warnings[0].Kind != status.ProjectMissing {
		t.Fatalf("missing project warning absent: %#v", r.Warnings)
	}
}

func TestExpectedConflictAndLibraryMissingCoexistWithoutUnmanaged(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	library := filepath.Join(root, "library")
	dir := filepath.Join(home, ".cursor", "skills")
	if err := os.MkdirAll(filepath.Join(dir, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Library: config.Library{Skills: []config.Skill{{ID: "local/x", Name: "x"}}}, Agents: map[string]config.Binding{"cursor": {Skills: []string{"local/x"}}}}
	r := status.Inspect(cfg, home, library)
	if !contains(r, status.Conflict) || !contains(r, status.LibraryMissing) {
		t.Fatalf("coexisting drift missing: %#v", r.Items)
	}
	for _, item := range r.Items {
		if item.Path == filepath.Join(dir, "x") && item.Kind == status.Unmanaged {
			t.Fatalf("expected conflict double-counted unmanaged: %#v", r.Items)
		}
	}
}

func TestReadErrorsAreVisibleAndUnhealthy(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	library := filepath.Join(root, "library")
	dir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := status.Inspect(&config.Config{Library: config.Library{Skills: []config.Skill{}}}, home, library)
	if !contains(r, status.IOError) || r.Healthy() {
		t.Fatalf("IO error hidden: %#v", r)
	}
}

func TestStatusRejectsEscapingLibraryAndOverlaySymlinks(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	library := filepath.Join(root, "library", "skills")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(library, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(library, "local", "x")); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Library: config.Library{Skills: []config.Skill{{ID: "local/x", Name: "x"}}}, Agents: map[string]config.Binding{"cursor": {Skills: []string{"local/x"}}}}
	r := status.Inspect(cfg, home, library)
	if !contains(r, status.LibraryMissing) {
		t.Fatalf("escaping library hidden: %#v", r)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".cursor")); err != nil {
		t.Fatal(err)
	}
	r = status.Inspect(cfg, home, library)
	if !contains(r, status.IOError) {
		t.Fatalf("escaping overlay hidden: %#v", r)
	}
}

func contains(r status.Report, k status.Kind) bool {
	for _, i := range r.Items {
		if i.Kind == k {
			return true
		}
	}
	return false
}
