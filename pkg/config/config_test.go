package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestPathsHonorAIKITHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AIKIT_HOME", home)

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Config != filepath.Join(home, "config.yaml") {
		t.Fatalf("config path = %q", paths.Config)
	}
	if paths.Lock != filepath.Join(home, "config.lock") {
		t.Fatalf("lock path = %q", paths.Lock)
	}
	if paths.LibrarySkills != filepath.Join(home, "library", "skills") {
		t.Fatalf("library path = %q", paths.LibrarySkills)
	}
	if paths.UpdateCache != filepath.Join(home, "cache", ".update-check") {
		t.Fatalf("update cache path = %q", paths.UpdateCache)
	}
}

func TestConfigRoundTripIncludesStructuredState(t *testing.T) {
	cfg := New()
	projectPath := t.TempDir()
	cfg.Library.Skills = []Skill{{
		ID: "example.com/group/repo/review", Name: "review",
		Source: "example.com/group/repo", SourcePath: "skills/review",
		Ref:      &Ref{Kind: "tag", Value: "v1.2.0"},
		Resolved: "0123456789abcdef0123456789abcdef01234567",
	}}
	cfg.Presets = []Preset{{Name: "dev", Skills: []string{"example.com/group/repo/review"}}}
	cfg.Agents = map[string]Binding{"cursor": {Presets: []string{"dev"}}}
	cfg.Projects = []Project{{
		Name: "aikit", Path: projectPath, Agents: []string{"cursor"},
		Binding:       Binding{Skills: []string{"example.com/group/repo/review"}},
		AgentBindings: map[string]Binding{"cursor": {Presets: []string{"dev"}}},
	}}
	cfg.PendingOperations = []PendingOperation{{
		ID: "op-1", Kind: OperationAdopt,
		Scope:  Scope{Project: "aikit", ProjectPath: projectPath, Agent: "cursor"},
		Target: filepath.Join(projectPath, ".cursor", "skills", "review"), SkillID: "example.com/group/repo/review",
		Temp:        filepath.Join(projectPath, ".cursor", "skills", ".aikit-adopt-temp-1"),
		Backup:      filepath.Join(projectPath, ".cursor", "skills", ".aikit-adopt-backup-1"),
		Original:    &Fingerprint{Kind: "directory", Hash: strings.Repeat("a", 64)},
		JournalHash: strings.Repeat("b", 64),
	}}

	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped config invalid: %v\n%s", err, b)
	}
	if got.Library.Skills[0].Ref == nil || got.Library.Skills[0].Ref.Kind != "tag" {
		t.Fatalf("structured ref lost: %#v", got.Library.Skills[0].Ref)
	}
	if got.Projects[0].AgentBindings["cursor"].Presets[0] != "dev" {
		t.Fatalf("agent binding lost: %#v", got.Projects[0].AgentBindings)
	}
	if got.PendingOperations[0].Original.Hash != strings.Repeat("a", 64) {
		t.Fatalf("pending operation lost: %#v", got.PendingOperations[0])
	}
}

func TestValidateRejectsInvalidReferencesAndPaths(t *testing.T) {
	valid := func() *Config {
		cfg := New()
		cfg.Library.Skills = []Skill{{ID: "local/review", Name: "review"}}
		cfg.Presets = []Preset{{Name: "dev", Skills: []string{"local/review"}}}
		cfg.Agents = map[string]Binding{"cursor": {Presets: []string{"dev"}}}
		cfg.Projects = []Project{{
			Name: "project", Path: t.TempDir(), Agents: []string{"cursor"},
			AgentBindings: map[string]Binding{"cursor": {Skills: []string{"local/review"}}},
		}}
		return cfg
	}

	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"unsafe skill id", func(c *Config) { c.Library.Skills[0].ID = "../review" }},
		{"unsafe skill name", func(c *Config) { c.Library.Skills[0].Name = "bad/name" }},
		{"unsafe source path", func(c *Config) { c.Library.Skills[0].SourcePath = "../skills/review" }},
		{"unknown skill", func(c *Config) { c.Agents["cursor"] = Binding{Skills: []string{"missing/id"}} }},
		{"unknown preset", func(c *Config) { c.Agents["cursor"] = Binding{Presets: []string{"missing"}} }},
		{"invalid agent", func(c *Config) { c.Agents["other"] = Binding{} }},
		{"agent binding not declared", func(c *Config) { c.Projects[0].AgentBindings["codex"] = Binding{} }},
		{"relative project path", func(c *Config) { c.Projects[0].Path = "relative" }},
		{"invalid ref kind", func(c *Config) { c.Library.Skills[0].Ref = &Ref{Kind: "head", Value: "main"} }},
		{"short resolved object", func(c *Config) { c.Library.Skills[0].Resolved = "abc123" }},
		{"remote without source path", func(c *Config) {
			c.Library.Skills[0].Source = "example.com/group/repo"
			c.Library.Skills[0].Ref = &Ref{Kind: "branch", Value: "main"}
			c.Library.Skills[0].Resolved = strings.Repeat("a", 40)
		}},
		{"remote without ref", func(c *Config) {
			c.Library.Skills[0].Source = "example.com/group/repo"
			c.Library.Skills[0].SourcePath = "."
			c.Library.Skills[0].Resolved = strings.Repeat("a", 40)
		}},
		{"remote without resolved", func(c *Config) {
			c.Library.Skills[0].Source = "example.com/group/repo"
			c.Library.Skills[0].SourcePath = "."
			c.Library.Skills[0].Ref = &Ref{Kind: "branch", Value: "main"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.edit(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() succeeded")
			}
		})
	}
}

func TestValidateAcceptsRemoteSkillAtRepositoryRoot(t *testing.T) {
	cfg := New()
	cfg.Library.Skills = []Skill{{
		ID: "example.com/group/repo/review", Name: "review",
		Source: "example.com/group/repo", SourcePath: ".",
		Ref: &Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40),
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("repository-root source path rejected: %v", err)
	}
}

func TestValidateRejectsDuplicateNamesAndIDs(t *testing.T) {
	projectPath := t.TempDir()
	tests := []struct {
		name string
		cfg  *Config
	}{
		{"skill id", &Config{Library: Library{Skills: []Skill{{ID: "local/a", Name: "a"}, {ID: "local/a", Name: "b"}}}}},
		{"preset name", &Config{Library: Library{Skills: []Skill{}}, Presets: []Preset{{Name: "dev"}, {Name: "dev"}}}},
		{"project name", &Config{Library: Library{Skills: []Skill{}}, Projects: []Project{{Name: "repo", Path: projectPath}, {Name: "repo", Path: filepath.Join(projectPath, "other")}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted duplicate")
			}
		})
	}
}

func TestValidateRejectsDuplicateCanonicalProjectPaths(t *testing.T) {
	realDir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(realDir, alias); err != nil {
		t.Fatal(err)
	}
	cfg := New()
	cfg.Projects = []Project{
		{Name: "one", Path: realDir, Agents: []string{"cursor"}},
		{Name: "two", Path: alias, Agents: []string{"codex"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted duplicate canonical project paths")
	}
}

func TestValidateCanonicalizesExistingSymlinkParents(t *testing.T) {
	realParent := t.TempDir()
	aliasParent := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(realParent, aliasParent); err != nil {
		t.Fatal(err)
	}
	cfg := New()
	cfg.Projects = []Project{
		{Name: "one", Path: filepath.Join(realParent, "missing", "repo"), Agents: []string{"cursor"}},
		{Name: "two", Path: filepath.Join(aliasParent, "missing", "repo"), Agents: []string{"codex"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted aliases beneath a symlinked parent")
	}
}

func TestValidateRejectsUnresolvableProjectPath(t *testing.T) {
	loop := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	cfg := New()
	cfg.Projects = []Project{{Name: "loop", Path: loop, Agents: []string{"cursor"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a symlink loop")
	}
}

func TestValidateRejectsPendingOperationOutsideScope(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	cfg := New()
	cfg.PendingOperations = []PendingOperation{{
		ID: "op-1", Kind: OperationCleanup, Scope: Scope{Agent: "cursor"},
		Target: filepath.Join(t.TempDir(), "review"), SkillID: "local/review",
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a cleanup target outside the Agent scope")
	}

	projectPath := t.TempDir()
	cfg.Projects = []Project{{Name: "repo", Path: projectPath, Agents: []string{"cursor"}}}
	cfg.PendingOperations = []PendingOperation{{
		ID: "op-2", Kind: OperationAdopt,
		Scope:  Scope{Project: "repo", ProjectPath: projectPath, Agent: "cursor"},
		Target: filepath.Join(projectPath, ".cursor", "skills", "review"), SkillID: "local/review",
		Temp:     filepath.Join(t.TempDir(), ".aikit-adopt-temp-1"),
		Backup:   filepath.Join(projectPath, ".cursor", "skills", ".aikit-adopt-backup-1"),
		Original: &Fingerprint{Kind: "directory", Hash: strings.Repeat("a", 64)},
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an adopt temp outside the target parent")
	}
}

func TestStoreLoadDefaultsAndCheckpointDurability(t *testing.T) {
	home := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	store := Store{Paths: PathsForHome(home)}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Library.Skills == nil {
		t.Fatal("default library skills must be an empty non-nil slice")
	}

	wantErr := "filesystem failed"
	err = store.WithLock(context.Background(), func(tx *Tx) error {
		tx.Config.Library.Skills = append(tx.Config.Library.Skills, Skill{ID: "local/review", Name: "review"})
		tx.Config.PendingOperations = append(tx.Config.PendingOperations, PendingOperation{
			ID: "op-1", Kind: OperationCleanup,
			Scope: Scope{Agent: "cursor"}, Target: filepath.Join(userHome, ".cursor", "skills", "review"),
			SkillID: "local/review", Reason: "disable",
		})
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		return contextError(wantErr)
	})
	if err == nil || err.Error() != wantErr {
		t.Fatalf("WithLock() error = %v", err)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Library.Skills) != 1 || len(got.PendingOperations) != 1 {
		t.Fatalf("first checkpoint not durable: %#v", got)
	}

	if err := store.WithLock(context.Background(), func(tx *Tx) error {
		tx.Config.PendingOperations = nil
		return tx.Checkpoint()
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.PendingOperations) != 0 {
		t.Fatalf("second checkpoint did not clear operation: %#v", got.PendingOperations)
	}
}

func TestWithLockSerializesAndReloadsAfterAcquiring(t *testing.T) {
	store := Store{Paths: PathsForHome(t.TempDir())}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- store.WithLock(context.Background(), func(tx *Tx) error {
			tx.Config.Library.Skills = append(tx.Config.Library.Skills, Skill{ID: "local/one", Name: "one"})
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- store.WithLock(context.Background(), func(tx *Tx) error {
			close(secondEntered)
			tx.Config.Library.Skills = append(tx.Config.Library.Skills, Skill{ID: "local/two", Name: "two"})
			return tx.Checkpoint()
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second transaction entered while first held the lock")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Library.Skills) != 2 {
		t.Fatalf("lost update: %#v", cfg.Library.Skills)
	}
}

func TestWithLockSupportsTwoCheckpointsAndCancellation(t *testing.T) {
	store := Store{Paths: PathsForHome(t.TempDir())}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.WithLock(context.Background(), func(tx *Tx) error {
			tx.Config.Library.Skills = append(tx.Config.Library.Skills, Skill{ID: "local/one", Name: "one"})
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			tx.Config.Library.Skills = append(tx.Config.Library.Skills, Skill{ID: "local/two", Name: "two"})
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	called := false
	err := store.WithLock(ctx, func(*Tx) error {
		called = true
		return nil
	})
	if err == nil || ctx.Err() == nil {
		t.Fatalf("lock wait was not canceled: err=%v ctx=%v", err, ctx.Err())
	}
	if called {
		t.Fatal("canceled transaction callback was called")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Library.Skills) != 2 {
		t.Fatalf("second checkpoint missing: %#v", cfg.Library.Skills)
	}
}

type contextError string

func (e contextError) Error() string { return string(e) }
