package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/pkg/config"
)

func TestSkillDetailReturnsSafeMetadataBindingsContentAndStableFiles(t *testing.T) {
	application, paths, _, _ := testApp(t)
	skillRoot := filepath.Join(paths.LibrarySkills, "local", "demo")
	content := "---\nname: demo\ndescription: Demo skill\n---\n\nInstructions\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "z-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "z-dir", "note.txt"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills[0].Description = "Demo skill"
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{"local/demo"}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	detail, err := application.SkillDetail(context.Background(), "local/demo")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Skill.ID != "local/demo" || detail.Skill.Description != "Demo skill" || detail.SkillMD != content || detail.SkillMDTruncated {
		t.Fatalf("detail metadata/content = %+v", detail)
	}
	if !containsScope(detail.EnabledLocations, config.Scope{Agent: "cursor"}) {
		t.Fatalf("enabled locations = %+v", detail.EnabledLocations)
	}
	wantFiles := []SkillFile{
		{Path: "SKILL.md", Kind: SkillFileRegular, Size: int64(len(content))},
		{Path: "a.txt", Kind: SkillFileRegular, Size: 1},
		{Path: "z-dir", Kind: SkillFileDirectory},
		{Path: "z-dir/note.txt", Kind: SkillFileRegular, Size: 4},
	}
	if !reflect.DeepEqual(detail.Files, wantFiles) {
		t.Fatalf("files = %#v, want %#v", detail.Files, wantFiles)
	}
}

func TestValidateConfigurationIsReadOnly(t *testing.T) {
	application, paths, _, _ := testApp(t)
	before, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := application.ValidateConfiguration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Path != paths.Config {
		t.Fatalf("validation=%+v", result)
	}
	after, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("ValidateConfiguration changed config bytes")
	}
}

func TestSkillDetailTruncatesSkillMDAtDocumentedByteLimit(t *testing.T) {
	application, paths, _, _ := testApp(t)
	skillFile := filepath.Join(paths.LibrarySkills, "local", "demo", "SKILL.md")
	content := strings.Repeat("x", SkillPreviewByteLimit+17)
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	detail, err := application.SkillDetail(context.Background(), "local/demo")
	if err != nil {
		t.Fatal(err)
	}
	if !detail.SkillMDTruncated || len(detail.SkillMD) != SkillPreviewByteLimit {
		t.Fatalf("preview length=%d truncated=%v", len(detail.SkillMD), detail.SkillMDTruncated)
	}
}

func TestSkillDetailRejectsEscapingSkillRoot(t *testing.T) {
	application, paths, _, _ := testApp(t)
	skillRoot := filepath.Join(paths.LibrarySkills, "local", "demo")
	outside := t.TempDir()
	secret := "outside sentinel"
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(skillRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, skillRoot); err != nil {
		t.Fatal(err)
	}
	if detail, err := application.SkillDetail(context.Background(), "local/demo"); err == nil || strings.Contains(detail.SkillMD, secret) {
		t.Fatalf("escaping root was read: detail=%+v err=%v", detail, err)
	}
}

func TestSkillDetailRejectsRootSwapBetweenAuthorizationAndOpen(t *testing.T) {
	application, paths, _, _ := testApp(t)
	skillRoot := filepath.Join(paths.LibrarySkills, "local", "demo")
	original := skillRoot + ".original"
	outside := t.TempDir()
	secret := "outside sentinel"
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	application.deps.OpenSkillRoot = func(libraryRoot, id string) (library.VerifiedSkillRoot, error) {
		if err := os.Rename(skillRoot, original); err != nil {
			return nil, err
		}
		if err := os.Symlink(outside, skillRoot); err != nil {
			return nil, err
		}
		return library.OpenVerifiedSkillRoot(libraryRoot, id)
	}
	detail, err := application.SkillDetail(context.Background(), "local/demo")
	if err == nil || strings.Contains(detail.SkillMD, secret) {
		t.Fatalf("root swap escaped detail containment: detail=%+v err=%v", detail, err)
	}
}

func TestSkillDetailListsButNeverTraversesNestedEscapingSymlink(t *testing.T) {
	application, paths, _, _ := testApp(t)
	skillRoot := filepath.Join(paths.LibrarySkills, "local", "demo")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(skillRoot, "escape")); err != nil {
		t.Fatal(err)
	}
	detail, err := application.SkillDetail(context.Background(), "local/demo")
	if err != nil {
		t.Fatal(err)
	}
	want := SkillFile{Path: "escape", Kind: SkillFileSymlink}
	found := false
	for _, file := range detail.Files {
		if file.Path == "escape/secret.txt" {
			t.Fatalf("detail traversed escaping symlink: %#v", detail.Files)
		}
		if file == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("symlink entry missing from stable file list: %#v", detail.Files)
	}
}

func TestConfigurationDetailUsesResolvedExplicitAndDefaultPaths(t *testing.T) {
	t.Run("explicit AIKIT_HOME", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "aikit home")
		t.Setenv("AIKIT_HOME", home)
		application := New(Dependencies{})
		detail, err := application.Configuration(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		paths := config.PathsForHome(home)
		if detail.Config != paths.Config || detail.Library != paths.LibrarySkills || detail.Cache != paths.Cache {
			t.Fatalf("configuration = %+v", detail)
		}
	})

	t.Run("default paths", func(t *testing.T) {
		userHome := t.TempDir()
		t.Setenv("AIKIT_HOME", "")
		t.Setenv("HOME", userHome)
		application := New(Dependencies{})
		detail, err := application.Configuration(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := config.PathsForHome(filepath.Join(userHome, ".aikit"))
		if detail.Config != want.Config || detail.Library != want.LibrarySkills || detail.Cache != want.Cache {
			t.Fatalf("configuration = %+v, want %+v", detail, want)
		}
	})
}
