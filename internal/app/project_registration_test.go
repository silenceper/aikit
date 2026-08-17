package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestPreviewProjectRegistrationDefaultsNameAndDetectsAgents(t *testing.T) {
	application, _, _, _ := testApp(t)
	project := filepath.Join(t.TempDir(), "aikit")
	for _, relative := range []string{".cursor/skills", ".codex/skills"} {
		if err := os.MkdirAll(filepath.Join(project, filepath.FromSlash(relative)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	preview, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: project})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Name != "aikit" || preview.Path != canonical || preview.NeedsName || preview.NameIssue != "" {
		t.Fatalf("registration identity = %+v", preview)
	}
	if want := []string{"cursor", "codex"}; !reflect.DeepEqual(preview.Agents, want) {
		t.Fatalf("detected agents = %#v, want %#v", preview.Agents, want)
	}
	if preview.PathIdentity == "" || preview.Preview.PathIdentity != preview.PathIdentity {
		t.Fatalf("opaque path identity was not retained: %+v", preview)
	}
}

func TestPreviewProjectRegistrationNoAgentsDoesNotSelectAll(t *testing.T) {
	application, _, _, _ := testApp(t)
	project := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	preview, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Agents) != 0 {
		t.Fatalf("empty project selected agents: %#v", preview.Agents)
	}
}

func TestPreviewProjectRegistrationReturnsTypedNameConflict(t *testing.T) {
	application, _, _, _ := testApp(t)
	project := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	preview, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: project})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.NeedsName || preview.NameIssue != ProjectNameDuplicate || preview.Name != "demo" {
		t.Fatalf("duplicate-name preview = %+v", preview)
	}
	if len(preview.Preview.Cleanup.Actions)+len(preview.Preview.Next.Actions) != 0 {
		t.Fatalf("conflicting name unexpectedly produced a mutation plan: %+v", preview.Preview)
	}

	invalid := filepath.Join(t.TempDir(), "bad/name")
	if err := os.MkdirAll(invalid, 0o755); err != nil {
		t.Fatal(err)
	}
	preview, err = application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: invalid, Name: "bad/name"})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.NeedsName || preview.NameIssue != ProjectNameInvalid {
		t.Fatalf("invalid-name preview = %+v", preview)
	}
}

func TestPreviewProjectRegistrationIsReadOnly(t *testing.T) {
	application, paths, _, _ := testApp(t)
	project := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(filepath.Join(project, ".codex", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := config.Store{Paths: paths}
	beforeConfig, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	beforeTree := snapshotProjectTree(t, project)
	recoverCalls, executeCalls := 0, 0
	application.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		recoverCalls++
		return link.Result{}
	}
	application.deps.Execute = func(link.Plan, bool) link.Result {
		executeCalls++
		return link.Result{}
	}

	if _, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: project}); err != nil {
		t.Fatal(err)
	}
	afterConfig, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeConfig, afterConfig) {
		t.Fatalf("preview changed config:\nbefore=%+v\nafter=%+v", beforeConfig, afterConfig)
	}
	if afterTree := snapshotProjectTree(t, project); !reflect.DeepEqual(beforeTree, afterTree) {
		t.Fatalf("preview changed project tree:\nbefore=%v\nafter=%v", beforeTree, afterTree)
	}
	if recoverCalls != 0 || executeCalls != 0 {
		t.Fatalf("preview invoked mutation seams: recover=%d execute=%d", recoverCalls, executeCalls)
	}
}

func TestPreviewProjectRegistrationRejectsMissingFileAndUnsafeAgentEntry(t *testing.T) {
	application, _, _, _ := testApp(t)
	t.Run("missing", func(t *testing.T) {
		_, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: filepath.Join(t.TempDir(), "missing")})
		if err == nil || !strings.Contains(err.Error(), "existing directory") {
			t.Fatalf("missing path error = %v", err)
		}
	})
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: path})
		if err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("file path error = %v", err)
		}
	})
	t.Run("unsafe agent entry", func(t *testing.T) {
		project := filepath.Join(t.TempDir(), "unsafe")
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(project, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(project, ".codex", "skills")); err != nil {
			t.Fatal(err)
		}
		preview, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: project})
		if err != nil {
			t.Fatal(err)
		}
		if len(preview.Agents) != 0 || len(preview.Warnings) == 0 || !strings.Contains(strings.Join(preview.Warnings, "\n"), "codex") {
			t.Fatalf("unsafe agent entry preview = %+v", preview)
		}
	})
}

func TestEditProjectRevalidatesDirectoryAtMutationBoundary(t *testing.T) {
	t.Run("create rejects missing path", func(t *testing.T) {
		application, paths, _, _ := testApp(t)
		missing := filepath.Join(t.TempDir(), "missing")
		if _, err := application.EditProject(context.Background(), ProjectEditRequest{Name: "missing", Path: missing}); err == nil {
			t.Fatal("missing project directory was accepted")
		}
		cfg, err := (config.Store{Paths: paths}).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Projects) != 1 {
			t.Fatalf("failed create changed config: %+v", cfg.Projects)
		}
	})

	t.Run("create rejects replacement after preview", func(t *testing.T) {
		application, paths, _, _ := testApp(t)
		project := filepath.Join(t.TempDir(), "replacement")
		if err := os.MkdirAll(project, 0o755); err != nil {
			t.Fatal(err)
		}
		preview, err := application.PreviewProjectRegistration(context.Background(), ProjectRegistrationRequest{Path: project})
		if err != nil {
			t.Fatal(err)
		}
		old := project + "-old"
		if err := os.Rename(project, old); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(project, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err = application.EditProject(context.Background(), ProjectEditRequest{
			Name:                 preview.Name,
			Path:                 preview.Path,
			AddAgents:            preview.Agents,
			ExpectedPathIdentity: preview.PathIdentity,
			Confirmed:            true,
		})
		if err == nil || !strings.Contains(err.Error(), "changed since preview") {
			t.Fatalf("replacement error = %v", err)
		}
		cfg, loadErr := (config.Store{Paths: paths}).Load(context.Background())
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if len(cfg.Projects) != 1 {
			t.Fatalf("replacement was checkpointed: %+v", cfg.Projects)
		}
	})
}

func snapshotProjectTree(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%s|%d", relative, info.Mode().String(), info.Size())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			line += "|" + target
		}
		entries = append(entries, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	return entries
}
