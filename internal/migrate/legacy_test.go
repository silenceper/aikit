package migrate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestMigrateCatalogAndProjectMapsCopilotAndPreservesLegacyFiles(t *testing.T) {
	service, paths, home := testService(t)
	seedLegacyRepository(t, paths, "acme/repo", "review", "cache")
	catalog := filepath.Join(paths.Home, "catalog.yaml")
	writeFile(t, catalog, "skills:\n  - name: review\n    source: acme/repo\n")
	project := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyProject := filepath.Join(project, ".aikit.yaml")
	writeFile(t, legacyProject, "project:\n  name: demo\n  targets: [github-copilot]\nassets:\n  skills:\n    - source: acme/repo\n      name: review\n")

	result, err := service.Migrate(context.Background(), app.MigrateRequest{ProjectPaths: []string{project}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || result.Failed != 0 || result.Exit != app.ExitOK {
		t.Fatalf("migration result = %+v", result)
	}
	cfg := loadConfig(t, paths)
	if len(cfg.Library.Skills) != 1 || cfg.Library.Skills[0].ID != "acme/repo/review" || cfg.Library.Skills[0].SourcePath != "review" || len(cfg.Library.Skills[0].Resolved) != 40 {
		t.Fatalf("remote skill = %+v", cfg.Library.Skills)
	}
	if len(cfg.Projects) != 1 || len(cfg.Projects[0].Agents) != 1 || cfg.Projects[0].Agents[0] != "copilot" {
		t.Fatalf("project mapping = %+v", cfg.Projects)
	}
	if got := cfg.Projects[0].Skills; len(got) != 1 || got[0] != "acme/repo/review" {
		t.Fatalf("project skills = %v", got)
	}
	for _, path := range []string{catalog, legacyProject, filepath.Join(paths.Cache, "acme", "repo"), home} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("legacy path was removed %s: %v", path, err)
		}
	}
}

func TestMigrateWithoutAdoptReportsPendingLocations(t *testing.T) {
	service, paths, _ := testService(t)
	project := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	seedLegacyRepository(t, paths, "acme/repo", "review", "cache")
	writeFile(t, filepath.Join(project, ".aikit.yaml"), "project:\n  name: demo\n  targets: [cursor]\nassets:\n  skills:\n    - source: acme/repo\n      name: review\n")
	writeSkill(t, filepath.Join(agent.Cursor{}.ProjectSkillDir(project), "review"), "review", "installed")
	result, err := service.Migrate(context.Background(), app.MigrateRequest{ProjectPaths: []string{project}})
	if err != nil {
		t.Fatal(err)
	}
	if result.PendingAdopt != 1 || result.Failed != 0 || result.Exit != app.ExitOK {
		t.Fatalf("pending adopt summary = %+v", result)
	}
}

func TestMigrateCWDProjectDefaultsCursorAndIsIdempotentMerge(t *testing.T) {
	service, paths, _ := testService(t)
	project := service.deps.WorkingDir
	seedLegacyRepository(t, paths, "acme/repo", "review", "cache")
	writeFile(t, filepath.Join(project, ".aikit.yaml"), "project:\n  name: demo\nassets:\n  skills:\n    - source: acme/repo\n      name: review\n")
	cfg := config.New()
	cfg.Agents = map[string]config.Binding{"codex": {}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	first, err := service.Migrate(context.Background(), app.MigrateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Migrate(context.Background(), app.MigrateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Imported != 1 || second.Skipped != 1 || second.Failed != 0 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	cfg = loadConfig(t, paths)
	if _, ok := cfg.Agents["codex"]; !ok {
		t.Fatal("non-empty config was overwritten")
	}
	if len(cfg.Projects) != 1 || len(cfg.Projects[0].Agents) != 1 || cfg.Projects[0].Agents[0] != "cursor" {
		t.Fatalf("default project = %+v", cfg.Projects)
	}
}

func TestMigrateDryRunDoesNotWriteConfigLibraryOrCache(t *testing.T) {
	service, paths, _ := testService(t)
	seedLegacyRepository(t, paths, "acme/repo", "review", "cache")
	writeFile(t, filepath.Join(paths.Home, "catalog.yaml"), "skills:\n  - name: review\n    source: acme/repo\n")
	beforeEntries := mustReadDirNames(t, paths.Cache)

	result, err := service.Migrate(context.Background(), app.MigrateRequest{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("dry-run summary = %+v", result)
	}
	if _, err := os.Stat(paths.Config); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config: %v", err)
	}
	if _, err := os.Stat(paths.LibrarySkills); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote library: %v", err)
	}
	afterEntries := mustReadDirNames(t, paths.Cache)
	if len(beforeEntries) != len(afterEntries) {
		t.Fatalf("dry-run changed cache: before=%v after=%v", beforeEntries, afterEntries)
	}
}

func TestMigratePendingRecoveryBlocksAdoptAndNonAdoptWithoutWrites(t *testing.T) {
	for _, adopt := range []bool{false, true} {
		t.Run(map[bool]string{false: "non-adopt", true: "adopt"}[adopt], func(t *testing.T) {
			service, paths, home := testService(t)
			seedLegacyRepository(t, paths, "acme/repo", "review", "cache")
			writeFile(t, filepath.Join(paths.Home, "catalog.yaml"), "skills:\n  - name: review\n    source: acme/repo\n")
			target := filepath.Join(home, ".cursor", "skills", "pending")
			op, err := link.NewCleanupOperation("pending-legacy", config.Scope{Agent: "cursor"}, target, "local/pending", "test")
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.New()
			cfg.Library.Skills = []config.Skill{{ID: "local/pending", Name: "pending", Hash: "pending"}}
			cfg.PendingOperations = []config.PendingOperation{op}
			if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			beforeConfig, err := os.ReadFile(paths.Config)
			if err != nil {
				t.Fatal(err)
			}
			recoveryCalls := 0
			service.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
				recoveryCalls++
				return link.Result{}
			}
			_, err = service.Migrate(context.Background(), app.MigrateRequest{Adopt: adopt})
			var pending *app.PendingRecoveryError
			if !errors.As(err, &pending) {
				t.Fatalf("migration pending error = %T %v", err, err)
			}
			if recoveryCalls != 0 {
				t.Fatalf("migration called recovery %d times", recoveryCalls)
			}
			afterConfig, err := os.ReadFile(paths.Config)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(beforeConfig, afterConfig) {
				t.Fatal("blocked migration changed config bytes")
			}
			if _, err := os.Stat(filepath.Join(paths.LibrarySkills, "acme", "repo", "review")); !os.IsNotExist(err) {
				t.Fatalf("blocked migration changed library: %v", err)
			}
		})
	}
}

func TestMigrateConflictIsPartialAndDoesNotOverwriteExistingEntry(t *testing.T) {
	service, paths, _ := testService(t)
	seedLegacyRepository(t, paths, "acme/repo", "review", "new-cache")
	writeFile(t, filepath.Join(paths.Home, "catalog.yaml"), "skills:\n  - name: review\n    source: acme/repo\n")
	existingPath := filepath.Join(paths.LibrarySkills, "acme", "repo", "review")
	writeSkill(t, existingPath, "review", "old-installed")
	existing := config.Skill{ID: "acme/repo/review", Name: "review", Source: "acme/repo", SourcePath: "review", Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Hash: mustHash(t, existingPath)}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{existing}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	result, err := service.Migrate(context.Background(), app.MigrateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Exit != app.ExitPartial {
		t.Fatalf("conflict summary = %+v", result)
	}
	cfg = loadConfig(t, paths)
	if len(cfg.Library.Skills) != 1 || cfg.Library.Skills[0].Hash != existing.Hash {
		t.Fatalf("existing entry overwritten: %+v", cfg.Library.Skills)
	}
	data, err := os.ReadFile(filepath.Join(existingPath, "SKILL.md"))
	if err != nil || !bytes.Contains(data, []byte("old-installed")) {
		t.Fatalf("existing library changed: %q %v", data, err)
	}
}

func TestMigrateAmbiguousSourcePathSkipsOnlyConflictingEntry(t *testing.T) {
	service, paths, _ := testService(t)
	seedLegacyRepositoryWithSkills(t, paths, "acme/repo", map[string]string{"one/review": "one", "two/review": "two", "ok": "ok"})
	writeFile(t, filepath.Join(paths.Home, "catalog.yaml"), "skills:\n  - name: review\n    source: acme/repo\n  - name: ok\n    source: acme/repo\n")

	result, err := service.Migrate(context.Background(), app.MigrateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Failed != 1 || result.Exit != app.ExitPartial {
		t.Fatalf("partial summary = %+v", result)
	}
	cfg := loadConfig(t, paths)
	if len(cfg.Library.Skills) != 1 || cfg.Library.Skills[0].Name != "ok" {
		t.Fatalf("successful peer was not preserved: %+v", cfg.Library.Skills)
	}
}

func TestMigrateLocalCatalogUsesLocalAllocationRules(t *testing.T) {
	service, paths, _ := testService(t)
	local := filepath.Join(paths.Home, "skills", "review")
	writeSkill(t, local, "review", "local-version")
	writeFile(t, filepath.Join(paths.Home, "catalog.yaml"), "skills:\n  - name: review\n    source: _local\n")
	existingPath := filepath.Join(paths.LibrarySkills, "local", "review")
	writeSkill(t, existingPath, "review", "different-version")
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{{ID: "local/review", Name: "review", Hash: mustHash(t, existingPath)}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	result, err := service.Migrate(context.Background(), app.MigrateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("local migration = %+v", result)
	}
	cfg = loadConfig(t, paths)
	if len(cfg.Library.Skills) != 2 || cfg.Library.Skills[1].ID == "local/review" || !strings.HasPrefix(cfg.Library.Skills[1].ID, "local/review-") {
		t.Fatalf("local allocation = %+v", cfg.Library.Skills)
	}
}

func TestMigrateAdoptInstalledContentDifferentFromCachePreservesBackup(t *testing.T) {
	service, paths, _ := testService(t)
	project := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	seedLegacyRepository(t, paths, "acme/repo", "review", "cache-version")
	writeFile(t, filepath.Join(project, ".aikit.yaml"), "project:\n  name: demo\n  targets: [cursor]\nassets:\n  skills:\n    - source: acme/repo\n      name: review\n")
	installed := filepath.Join(agent.Cursor{}.ProjectSkillDir(project), "review")
	writeSkill(t, installed, "review", "installed-version")

	result, err := service.Migrate(context.Background(), app.MigrateRequest{ProjectPaths: []string{project}, Adopt: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Exit != app.ExitPartial {
		t.Fatalf("differing adopt summary = %+v", result)
	}
	cfg := loadConfig(t, paths)
	if len(cfg.PendingOperations) != 1 || cfg.PendingOperations[0].Kind != config.OperationAdopt {
		t.Fatalf("recoverable adopt journal missing: %+v", cfg.PendingOperations)
	}
	backup := cfg.PendingOperations[0].Backup
	data, err := os.ReadFile(filepath.Join(backup, "SKILL.md"))
	if err != nil || !bytes.Contains(data, []byte("installed-version")) {
		t.Fatalf("installed content was not preserved: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(project, ".aikit.yaml")); err != nil {
		t.Fatalf("legacy project deleted: %v", err)
	}
}

func TestMigrateMissingExplicitProjectIsFailure(t *testing.T) {
	service, _, _ := testService(t)
	missing := filepath.Join(t.TempDir(), "missing")
	result, err := service.Migrate(context.Background(), app.MigrateRequest{ProjectPaths: []string{missing}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Exit != app.ExitPartial || len(result.Warnings) == 0 {
		t.Fatalf("missing explicit project = %+v", result)
	}
}

func TestMigrateFindsOldGitLabSubgroupCacheLayout(t *testing.T) {
	service, paths, _ := testService(t)
	legacySource := "https://gitlab.example.com/group/subgroup/repo.git"
	seedLegacyRepository(t, paths, "gitlab.example.com/subgroup/repo", "review", "cache")
	writeFile(t, filepath.Join(paths.Home, "catalog.yaml"), "skills:\n  - name: review\n    source: "+legacySource+"\n")
	result, err := service.Migrate(context.Background(), app.MigrateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("gitlab migration = %+v", result)
	}
	cfg := loadConfig(t, paths)
	if len(cfg.Library.Skills) != 1 || cfg.Library.Skills[0].Source != "gitlab.example.com/group/subgroup/repo" {
		t.Fatalf("canonical source = %+v", cfg.Library.Skills)
	}
}

func TestMigrateProjectPreflightFailureDoesNotPoisonLaterProject(t *testing.T) {
	service, paths, _ := testService(t)
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(first, ".aikit.yaml"), "project:\n  name: first\n  targets: [cursor]\n")
	writeFile(t, filepath.Join(second, ".aikit.yaml"), "project:\n  name: second\n  targets: [codex]\n")
	globalPath := filepath.Join(paths.LibrarySkills, "local", "global")
	projectPath := filepath.Join(paths.LibrarySkills, "local", "project")
	writeSkill(t, globalPath, "same-name", "global")
	writeSkill(t, projectPath, "same-name", "project")
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{
		{ID: "local/global", Name: "same-name", Hash: mustHash(t, globalPath)},
		{ID: "local/project", Name: "same-name", Hash: mustHash(t, projectPath)},
	}
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{"local/global"}}}
	cfg.Projects = []config.Project{{Name: "first", Path: first, Agents: []string{"codex"}, Binding: config.Binding{Skills: []string{"local/project"}}, AgentBindings: map[string]config.Binding{}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	result, err := service.Migrate(context.Background(), app.MigrateRequest{ProjectPaths: []string{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Exit != app.ExitPartial {
		t.Fatalf("migration summary = %+v", result)
	}
	cfg = loadConfig(t, paths)
	if len(cfg.Projects) != 2 || cfg.Projects[1].Name != "second" {
		t.Fatalf("later project was poisoned by failed preflight: %+v", cfg.Projects)
	}
	if len(cfg.Projects[0].Agents) != 1 || cfg.Projects[0].Agents[0] != "codex" {
		t.Fatalf("failed project mutation leaked: %+v", cfg.Projects[0])
	}
}

func TestMigrateAdoptInspectErrorIsFailure(t *testing.T) {
	service, paths, _ := testService(t)
	project := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	seedLegacyRepository(t, paths, "acme/repo", "review", "cache")
	writeFile(t, filepath.Join(project, ".aikit.yaml"), "project:\n  name: demo\n  targets: [cursor]\nassets:\n  skills:\n    - source: acme/repo\n      name: review\n")
	service.deps.Inspect = func(string, string) (link.State, error) {
		return link.State{}, errors.New("inspect denied")
	}
	result, err := service.Migrate(context.Background(), app.MigrateRequest{ProjectPaths: []string{project}, Adopt: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Exit != app.ExitPartial || !strings.Contains(strings.Join(result.Warnings, "\n"), "inspect denied") {
		t.Fatalf("inspect error was hidden: %+v", result)
	}
}

func seedLegacyRepository(t *testing.T, paths config.Paths, source, name, body string) {
	t.Helper()
	seedLegacyRepositoryWithSkills(t, paths, source, map[string]string{name: body})
}

func seedLegacyRepositoryWithSkills(t *testing.T, paths config.Paths, source string, skills map[string]string) {
	t.Helper()
	repo := filepath.Join(paths.Cache, filepath.FromSlash(source))
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	for rel, body := range skills {
		writeSkill(t, filepath.Join(repo, filepath.FromSlash(rel)), filepath.Base(rel), body)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=aikit-test", "-c", "user.email=aikit@example.invalid", "commit", "-m", "seed")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadDirNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]string, len(entries))
	for i, entry := range entries {
		result[i] = entry.Name()
	}
	return result
}
