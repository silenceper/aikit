package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/silenceper/aikit/pkg/config"
)

func TestCompareReportsMetadataFileAndContentDifferences(t *testing.T) {
	application, paths, _, _ := testApp(t)
	rightPath := filepath.Join(paths.LibrarySkills, "local", "other")
	if err := os.MkdirAll(rightPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rightPath, "SKILL.md"), []byte("---\nname: other\n---\nright\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rightPath, "only-right.txt"), []byte("right"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, config.Skill{ID: "local/other", Name: "other", Description: "different", Hash: "other-hash"})
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	result, err := application.Compare(context.Background(), CompareRequest{LeftSkillID: "local/demo", RightSkillID: "local/other"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Equal || len(result.Metadata) == 0 || len(result.Files) == 0 || len(result.Content) == 0 {
		t.Fatalf("incomplete compare result: %+v", result)
	}
}

func TestCompareSymlinksUsesRawTargetWithoutFollowing(t *testing.T) {
	application, paths, _, _ := testApp(t)
	leftPath := filepath.Join(paths.LibrarySkills, "local", "demo")
	rightPath := filepath.Join(paths.LibrarySkills, "local", "other")
	if err := os.MkdirAll(rightPath, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-secret")
	if err := os.WriteFile(outside, []byte("must not be read"), 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside-target", filepath.Join(leftPath, "reference")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(rightPath, "reference")); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, config.Skill{ID: "local/other", Name: "other"})
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	result, err := application.Compare(context.Background(), CompareRequest{LeftSkillID: "local/demo", RightSkillID: "local/other"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diff := range result.Files {
		if diff.Path == "reference" && diff.Kind == FileChanged {
			found = true
		}
	}
	if !found {
		t.Fatalf("raw symlink targets were not compared: %+v", result.Files)
	}
	if got := result.Left.Files[len(result.Left.Files)-1].LinkTarget; got != "inside-target" {
		t.Fatalf("left raw link target = %q", got)
	}
	if got := result.Right.Files[len(result.Right.Files)-1].LinkTarget; got != outside {
		t.Fatalf("right raw link target = %q", got)
	}
}
