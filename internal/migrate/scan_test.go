package migrate

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestScanAllGlobalAgentsOnlyImportsLibrary(t *testing.T) {
	service, paths, home := testService(t)
	for _, item := range agent.All() {
		writeSkill(t, filepath.Join(item.GlobalSkillDir(home), item.Name()+"-skill"), item.Name()+"-skill", item.Name())
	}

	result, err := service.Scan(context.Background(), app.ScanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 5 || result.Exit != app.ExitOK {
		t.Fatalf("scan result = %+v", result)
	}
	cfg := loadConfig(t, paths)
	if len(cfg.Library.Skills) != 5 {
		t.Fatalf("library = %+v", cfg.Library.Skills)
	}
	if len(cfg.Agents) != 0 {
		t.Fatalf("plain scan wrote bindings: %+v", cfg.Agents)
	}
	for _, item := range agent.All() {
		path := filepath.Join(item.GlobalSkillDir(home), item.Name()+"-skill")
		if info, err := os.Lstat(path); err != nil || !info.IsDir() {
			t.Fatalf("plain scan changed %s: %v %v", path, info, err)
		}
	}
}

func TestScanStableOriginAllocationAndHashReuse(t *testing.T) {
	service, paths, home := testService(t)
	writeSkill(t, filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "review"), "review", "first")
	writeSkill(t, filepath.Join(agent.ClaudeCode{}.GlobalSkillDir(home), "review"), "review", "second")
	writeSkill(t, filepath.Join(agent.Codex{}.GlobalSkillDir(home), "review"), "review", "first")

	result, err := service.Scan(context.Background(), app.ScanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items = %+v", result.Items)
	}
	byOrigin := map[string]string{}
	for _, item := range result.Items {
		byOrigin[item.Origin] = item.Skill.ID
	}
	if byOrigin["g/cursor"] != "local/review" || byOrigin["g/codex"] != "local/review" {
		t.Fatalf("same hash not stably reused: %+v", byOrigin)
	}
	if got := byOrigin["g/claude-code"]; got == "local/review" || len(got) != len("local/review-")+12 {
		t.Fatalf("different hash id = %q", got)
	}
	if got := len(loadConfig(t, paths).Library.Skills); got != 2 {
		t.Fatalf("library contains %d entries", got)
	}
}

func TestScanAdoptWritesExactGlobalAndProjectAgentBindings(t *testing.T) {
	service, paths, home := testService(t)
	projectPath := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Projects = []config.Project{{Name: "demo", Path: projectPath, Agents: []string{"cursor"}, AgentBindings: map[string]config.Binding{}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "global")
	project := filepath.Join(agent.Cursor{}.ProjectSkillDir(projectPath), "project")
	writeSkill(t, global, "global", "global")
	writeSkill(t, project, "project", "project")

	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", Project: "demo", Adopt: true, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || !result.Items[0].Adopted || !result.Items[1].Adopted {
		t.Fatalf("adopt result = %+v", result)
	}
	cfg = loadConfig(t, paths)
	if got := cfg.Agents["cursor"].Skills; len(got) != 1 || got[0] != "local/global" {
		t.Fatalf("global binding = %v", got)
	}
	if got := cfg.Projects[0].AgentBindings["cursor"].Skills; len(got) != 1 || got[0] != "local/project" {
		t.Fatalf("project binding = %v", got)
	}
	for _, path := range []string{global, project} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s was not adopted: %v %v", path, info, err)
		}
	}
	if len(cfg.PendingOperations) != 0 {
		t.Fatalf("completed adopt remains pending: %+v", cfg.PendingOperations)
	}
}

func TestScanAdoptExistingAikitLinkOnlyAddsBindingAndSelectionFilters(t *testing.T) {
	service, paths, home := testService(t)
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "existing")
	writeSkill(t, libraryPath, "existing", "library")
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{{ID: "local/existing", Name: "existing", Hash: mustHash(t, libraryPath)}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	dir := agent.Cursor{}.GlobalSkillDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "existing")
	if err := os.Symlink(libraryPath, target); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(dir, "ignored"), "ignored", "ignored")

	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", Adopt: true, Skills: []string{"existing"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Skill.ID != "local/existing" || !result.Items[0].Adopted {
		t.Fatalf("result = %+v", result)
	}
	cfg = loadConfig(t, paths)
	if len(cfg.Library.Skills) != 1 || len(cfg.Agents["cursor"].Skills) != 1 {
		t.Fatalf("selection imported extra content: %+v", cfg)
	}
	linkTarget, err := os.Readlink(target)
	if err != nil || linkTarget != libraryPath {
		t.Fatalf("existing link changed: %q %v", linkTarget, err)
	}
}

func TestPlainScanSkipsExistingAikitLink(t *testing.T) {
	service, paths, home := testService(t)
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "existing")
	writeSkill(t, libraryPath, "existing", "library")
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{{ID: "local/existing", Name: "existing", Hash: mustHash(t, libraryPath)}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	dir := agent.Cursor{}.GlobalSkillDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(libraryPath, filepath.Join(dir, "existing")); err != nil {
		t.Fatal(err)
	}
	result, err := service.Scan(context.Background(), app.ScanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("plain scan did not skip managed link: %+v", result.Items)
	}
}

func TestScanIsIdempotent(t *testing.T) {
	service, paths, home := testService(t)
	writeSkill(t, filepath.Join(agent.Windsurf{}.GlobalSkillDir(home), "demo"), "demo", "same")
	for i := 0; i < 2; i++ {
		if _, err := service.Scan(context.Background(), app.ScanRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(loadConfig(t, paths).Library.Skills); got != 1 {
		t.Fatalf("idempotent scan produced %d entries", got)
	}
}

func TestScanDryRunDoesNotWriteConfigOrLibrary(t *testing.T) {
	service, paths, home := testService(t)
	target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "preview")
	writeSkill(t, target, "preview", "content")

	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true, Targets: []string{target}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Target != target || result.Items[0].Skill.ID != "local/preview" {
		t.Fatalf("dry-run result = %+v", result)
	}
	if _, err := os.Stat(paths.Config); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config: %v", err)
	}
	if _, err := os.Stat(paths.LibrarySkills); !os.IsNotExist(err) {
		t.Fatalf("dry-run created or changed library root: %v", err)
	}
}

func TestScanDryRunTargetsSelectOneOfDuplicateIDs(t *testing.T) {
	service, _, home := testService(t)
	first := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "review")
	second := filepath.Join(agent.Codex{}.GlobalSkillDir(home), "review")
	writeSkill(t, first, "review", "same")
	writeSkill(t, second, "review", "same")

	result, err := service.Scan(context.Background(), app.ScanRequest{DryRun: true, Targets: []string{second}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Target != second || result.Items[0].Skill.ID != "local/review" {
		t.Fatalf("target-filtered dry-run = %+v", result.Items)
	}
}

func TestScanDiscoveryFailureIsPartial(t *testing.T) {
	service, _, home := testService(t)
	target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "broken")
	writeSkill(t, target, "bad/name", "broken")
	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != app.ExitPartial || len(result.Warnings) == 0 {
		t.Fatalf("discovery failure = %+v", result)
	}
}

func TestScanAdoptRecoversExistingPendingBeforeDiscovery(t *testing.T) {
	service, paths, home := testService(t)
	target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "demo")
	writeSkill(t, target, "demo", "content")
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "demo")
	writeSkill(t, libraryPath, "demo", "content")
	skill := config.Skill{ID: "local/demo", Name: "demo", Hash: mustHash(t, libraryPath)}
	op, err := link.NewAdoptOperation("", config.Scope{Agent: "cursor"}, target, skill.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{skill}
	cfg.Agents = map[string]config.Binding{"cursor": {Skills: []string{skill.ID}}}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", Adopt: true, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != app.ExitOK || len(result.Items) != 1 || !result.Items[0].Adopted {
		t.Fatalf("recovered scan = %+v", result)
	}
	cfg = loadConfig(t, paths)
	if len(cfg.PendingOperations) != 0 {
		t.Fatalf("completed pending operation remains: %+v", cfg.PendingOperations)
	}
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("pending adopt was not recovered: %v %v", info, err)
	}
}

func testService(t *testing.T) (*Service, config.Paths, string) {
	t.Helper()
	aikitHome, home := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", home)
	paths := config.PathsForHome(aikitHome)
	return New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: home, WorkingDir: t.TempDir()}), paths, home
}

func writeSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("---\nname: " + name + "\ndescription: test\n---\n" + body + "\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustHash(t *testing.T, dir string) string {
	t.Helper()
	candidates, err := library.Discover(dir)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("discover: %v %+v", err, candidates)
	}
	return candidates[0].Hash
}

func loadConfig(t *testing.T, paths config.Paths) *config.Config {
	t.Helper()
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(cfg.Library.Skills, func(i, j int) bool { return cfg.Library.Skills[i].ID < cfg.Library.Skills[j].ID })
	return cfg
}
