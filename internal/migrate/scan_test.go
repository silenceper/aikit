package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestScanAllRegisteredProjects(t *testing.T) {
	service, paths, home := testService(t)
	for _, item := range agent.All() {
		writeSkill(t, filepath.Join(item.GlobalSkillDir(home), "global-"+item.Name()), "global-"+item.Name(), item.Name())
	}
	alpha, zeta := filepath.Join(t.TempDir(), "alpha"), filepath.Join(t.TempDir(), "zeta")
	for _, path := range []string{alpha, zeta} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.New()
	// Deliberately reverse both project and declaration order. Inventory order
	// is registry globals, then project name, then registry agent order.
	cfg.Projects = []config.Project{
		{Name: "zeta", Path: zeta, Agents: []string{"windsurf", "cursor"}, AgentBindings: map[string]config.Binding{}},
		{Name: "alpha", Path: alpha, Agents: []string{"codex", "claude-code"}, AgentBindings: map[string]config.Binding{}},
	}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	for _, project := range cfg.Projects {
		for _, name := range project.Agents {
			item, _ := agent.ByName(name)
			writeSkill(t, filepath.Join(item.ProjectSkillDir(project.Path), project.Name+"-"+name), project.Name+"-"+name, name)
		}
	}

	result, err := service.Scan(context.Background(), app.ScanRequest{DryRun: true, AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"g/cursor", "g/claude-code", "g/codex", "g/copilot", "g/windsurf",
		"p/alpha/claude-code", "p/alpha/codex", "p/zeta/cursor", "p/zeta/windsurf",
	}
	got := make([]string, len(result.Items))
	for i, item := range result.Items {
		got[i] = item.Origin
		if item.Scope.Agent != item.Agent || item.Scope.Project != item.Project {
			t.Fatalf("item %d scope = %#v, agent/project = %q/%q", i, item.Scope, item.Agent, item.Project)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins = %#v, want %#v", got, want)
	}
}

func TestScanSelectorUsesOriginAndTarget(t *testing.T) {
	service, paths, home := testService(t)
	first := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "review")
	second := filepath.Join(agent.Codex{}.GlobalSkillDir(home), "review")
	writeSkill(t, first, "review", "same")
	writeSkill(t, second, "review", "same")

	preview, err := service.Scan(context.Background(), app.ScanRequest{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 2 || preview.Items[0].Key == preview.Items[1].Key {
		t.Fatalf("preview identities = %+v", preview.Items)
	}
	selected := preview.Items[1]
	request := app.ScanRequest{Adopt: true, Selectors: []app.ScanSelector{selectorFor(selected)}}
	result, err := service.Scan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Origin != selected.Origin || !result.Items[0].Adopted {
		t.Fatalf("selected result = %+v", result.Items)
	}
	if info, err := os.Lstat(first); err != nil || !info.IsDir() {
		t.Fatalf("unselected origin changed: %v %v", info, err)
	}
	if info, err := os.Lstat(second); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("selected origin not adopted: %v %v", info, err)
	}
	cfg := loadConfig(t, paths)
	if len(cfg.Agents["cursor"].Skills) != 0 || len(cfg.Agents["codex"].Skills) != 1 {
		t.Fatalf("bindings = %#v", cfg.Agents)
	}

	wrongOriginSelector := selectorFor(selected)
	wrongOriginSelector.Origin = "g/cursor"
	wrongOrigin := app.ScanRequest{DryRun: true, Selectors: []app.ScanSelector{wrongOriginSelector}}
	if got, err := service.Scan(context.Background(), wrongOrigin); err == nil || len(got.Items) != 0 {
		t.Fatalf("selector accepted mismatched origin: %+v, %v", got, err)
	}
	wrongTargetSelector := selectorFor(selected)
	wrongTargetSelector.Target = first
	wrongTarget := app.ScanRequest{DryRun: true, Selectors: []app.ScanSelector{wrongTargetSelector}}
	if got, err := service.Scan(context.Background(), wrongTarget); err == nil || len(got.Items) != 0 {
		t.Fatalf("selector accepted mismatched target: %+v, %v", got, err)
	}
	alias := filepath.Dir(selected.Target) + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(filepath.Dir(selected.Target)) + string(filepath.Separator) + filepath.Base(selected.Target)
	aliasSelector := selectorFor(selected)
	aliasSelector.Target = alias
	aliasRequest := app.ScanRequest{DryRun: true, Selectors: []app.ScanSelector{aliasSelector}}
	if got, err := service.Scan(context.Background(), aliasRequest); err == nil || len(got.Items) != 0 {
		t.Fatalf("selector accepted canonical alias %q: %+v, %v", alias, got, err)
	}
}

func TestScanInventoryKeyEncoding(t *testing.T) {
	origin := "p/demo/codex"
	target := filepath.Join(t.TempDir(), "skill")
	var encoded bytes.Buffer
	encoded.WriteString("aikit-inventory-v1\x00")
	for _, value := range []string{origin, target} {
		if err := binary.Write(&encoded, binary.BigEndian, uint64(len([]byte(value)))); err != nil {
			t.Fatal(err)
		}
		encoded.WriteString(value)
	}
	want := sha256.Sum256(encoded.Bytes())
	if got := inventoryKey(origin, target); got != fmtHash(want[:]) {
		t.Fatalf("inventoryKey() = %q, want %q", got, fmtHash(want[:]))
	}
}

func TestScanDryRunIsStrictlyReadOnly(t *testing.T) {
	service, paths, home := testService(t)
	target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "preview")
	writeSkill(t, target, "preview", "content")
	cfg := config.New()
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	beforeConfig, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	beforeLibrary := snapshotTree(t, paths.LibrarySkills)
	service.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		t.Fatal("dry-run called recovery")
		return link.Result{}
	}

	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true, Adopt: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Action != app.ScanActionAdopt || result.Items[0].Adopted {
		t.Fatalf("dry-run adopt plan = %+v", result.Items)
	}
	afterConfig, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeConfig, afterConfig) {
		t.Fatal("dry-run changed config bytes")
	}
	if after := snapshotTree(t, paths.LibrarySkills); !reflect.DeepEqual(beforeLibrary, after) {
		t.Fatalf("dry-run changed library tree: before=%#v after=%#v", beforeLibrary, after)
	}
	if _, err := os.Stat(paths.Lock); !os.IsNotExist(err) {
		t.Fatalf("dry-run acquired persistent lock: %v", err)
	}
}

func TestScanMutationBlocksPendingRecovery(t *testing.T) {
	for _, request := range []app.ScanRequest{
		{Agent: "cursor"},
		{Agent: "cursor", Adopt: true, All: true},
	} {
		name := "import"
		if request.Adopt {
			name = "adopt"
		}
		t.Run(name, func(t *testing.T) {
			service, paths, home := testService(t)
			root := agent.Cursor{}.GlobalSkillDir(home)
			writeSkill(t, filepath.Join(root, "candidate"), "candidate", "new")
			pendingTarget := filepath.Join(root, "missing-pending")
			op, err := link.NewCleanupOperation("pending", config.Scope{Agent: "cursor"}, pendingTarget, "local/pending", "test")
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.New()
			cfg.PendingOperations = []config.PendingOperation{op}
			if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			beforeConfig, err := os.ReadFile(paths.Config)
			if err != nil {
				t.Fatal(err)
			}
			beforeLibrary := snapshotTree(t, paths.LibrarySkills)
			recoverCalls := 0
			service.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
				recoverCalls++
				return link.Result{}
			}

			result, err := service.Scan(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), "pending recovery") {
				t.Fatalf("mutation with pending = %+v, %v", result, err)
			}
			if recoverCalls != 0 {
				t.Fatalf("mutation invoked recovery %d times", recoverCalls)
			}
			afterConfig, readErr := os.ReadFile(paths.Config)
			if readErr != nil || !bytes.Equal(beforeConfig, afterConfig) {
				t.Fatalf("mutation changed config: %v", readErr)
			}
			if after := snapshotTree(t, paths.LibrarySkills); !reflect.DeepEqual(beforeLibrary, after) {
				t.Fatalf("mutation changed library: before=%#v after=%#v", beforeLibrary, after)
			}
			if info, statErr := os.Lstat(filepath.Join(root, "candidate")); statErr != nil || !info.IsDir() {
				t.Fatalf("mutation changed source: %v %v", info, statErr)
			}
		})
	}
}

func TestScanMutationRejectsRootReplacedAfterDiscovery(t *testing.T) {
	service, paths, home := testService(t)
	root := agent.Cursor{}.GlobalSkillDir(home)
	target := filepath.Join(root, "candidate")
	writeSkill(t, target, "candidate", "content")
	preview, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil || len(preview.Items) != 1 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	selected := preview.Items[0]
	service.deps.BeforeMutationValidation = func() {
		held := filepath.Join(filepath.Dir(root), "held-candidate")
		if err := os.Rename(target, held); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(root, root+".old"); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(held, target); err != nil {
			t.Fatal(err)
		}
	}
	request := app.ScanRequest{Adopt: true, Selectors: []app.ScanSelector{selectorFor(selected)}}
	result, err := service.Scan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].State != app.ScanStateError || result.Exit != app.ExitPartial {
		t.Fatalf("root replacement result = %+v", result)
	}
	if _, err := os.Stat(paths.LibrarySkills); !os.IsNotExist(err) {
		t.Fatalf("root replacement wrote library: %v", err)
	}
	if cfg := loadConfig(t, paths); len(cfg.Library.Skills) != 0 || len(cfg.Agents) != 0 {
		t.Fatalf("root replacement changed config: %+v", cfg)
	}
}

func TestScanSelectorRejectsCrossRequestContentOrObjectSwap(t *testing.T) {
	for _, scenario := range []string{"edited-content", "replaced-object"} {
		t.Run(scenario, func(t *testing.T) {
			service, paths, home := testService(t)
			target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "candidate")
			writeSkill(t, target, "candidate", "original")
			preview, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true, Adopt: true})
			if err != nil || len(preview.Items) != 1 {
				t.Fatalf("preview = %+v, %v", preview, err)
			}
			selector := selectorFor(preview.Items[0])
			if selector.ExpectedHash == "" || selector.ExpectedObjectID == "" || selector.ExpectedRootID == "" || selector.ExpectedState == "" || selector.ExpectedSkillID == "" {
				t.Fatalf("selector does not bind preview identity: %+v", selector)
			}
			switch scenario {
			case "edited-content":
				writeSkill(t, target, "candidate", "edited")
			case "replaced-object":
				if err := os.Rename(target, target+".old"); err != nil {
					t.Fatal(err)
				}
				writeSkill(t, target, "candidate", "original")
			}
			result, err := service.Scan(context.Background(), app.ScanRequest{Adopt: true, Selectors: []app.ScanSelector{selector}})
			if err == nil || !strings.Contains(err.Error(), "preview identity") {
				t.Fatalf("cross-request swap = %+v, %v", result, err)
			}
			if _, statErr := os.Stat(paths.Config); !os.IsNotExist(statErr) {
				t.Fatalf("swap wrote config: %v", statErr)
			}
			if _, statErr := os.Stat(paths.LibrarySkills); !os.IsNotExist(statErr) {
				t.Fatalf("swap wrote library: %v", statErr)
			}
			if info, statErr := os.Lstat(target); statErr != nil || !info.IsDir() {
				t.Fatalf("swap replaced source: %v %v", info, statErr)
			}
		})
	}
}

func TestScanAdoptBatchPreflightIsCumulativeAndAtomic(t *testing.T) {
	service, paths, home := testService(t)
	root := agent.Cursor{}.GlobalSkillDir(home)
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	writeSkill(t, first, "review", "first")
	writeSkill(t, second, "review", "second")

	preview, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true, Adopt: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 2 || preview.Exit != app.ExitPartial {
		t.Fatalf("cumulative preview = %+v", preview)
	}
	for _, item := range preview.Items {
		if item.Action != app.ScanActionNone || item.State != app.ScanStateError {
			t.Fatalf("preview did not reject whole batch: %+v", preview.Items)
		}
	}

	inventory, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil || len(inventory.Items) != 2 {
		t.Fatalf("inventory = %+v, %v", inventory, err)
	}
	selectors := []app.ScanSelector{selectorFor(inventory.Items[0]), selectorFor(inventory.Items[1])}
	result, err := service.Scan(context.Background(), app.ScanRequest{Adopt: true, Selectors: selectors})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != app.ExitPartial || len(result.Items) != 2 {
		t.Fatalf("mutation preflight = %+v", result)
	}
	if _, statErr := os.Stat(paths.Config); !os.IsNotExist(statErr) {
		t.Fatalf("batch wrote config: %v", statErr)
	}
	if _, statErr := os.Stat(paths.LibrarySkills); !os.IsNotExist(statErr) {
		t.Fatalf("batch wrote library: %v", statErr)
	}
	for _, target := range []string{first, second} {
		if info, statErr := os.Lstat(target); statErr != nil || !info.IsDir() {
			t.Fatalf("batch changed %s: %v %v", target, info, statErr)
		}
	}
}

func TestScanSameContentVerifiesActualLibraryBytes(t *testing.T) {
	service, paths, home := testService(t)
	target := filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "review")
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "review")
	writeSkill(t, target, "review", "original")
	writeSkill(t, libraryPath, "review", "original")
	ledgerHash := mustHash(t, libraryPath)
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{{ID: "local/review", Name: "review", Hash: ledgerHash}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, libraryPath, "review", "drifted-library")
	beforeConfig, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := snapshotTree(t, target)

	preview, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true, Adopt: true})
	if err != nil || len(preview.Items) != 1 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	item := preview.Items[0]
	if item.Action == app.ScanActionLinkExisting || item.DiagnosticState != app.ScanStateDrifted || item.MatchedLibraryActualHash == "" || item.MatchedLibraryActualHash == ledgerHash {
		t.Fatalf("stale ledger classification = %+v", item)
	}
	selector := selectorFor(item)
	result, err := service.Scan(context.Background(), app.ScanRequest{Adopt: true, Selectors: []app.ScanSelector{selector}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != app.ExitPartial {
		t.Fatalf("stale ledger mutation = %+v", result)
	}
	afterConfig, err := os.ReadFile(paths.Config)
	if err != nil || !bytes.Equal(beforeConfig, afterConfig) {
		t.Fatalf("stale ledger changed config: %v", err)
	}
	if afterSource := snapshotTree(t, target); !reflect.DeepEqual(beforeSource, afterSource) {
		t.Fatalf("stale ledger changed source: before=%#v after=%#v", beforeSource, afterSource)
	}
}

func TestObjectIdentityFailsClosedWithoutPlatformMetadata(t *testing.T) {
	if _, err := objectIdentity("ignored", fakeFileInfo{}); err == nil {
		t.Fatal("object identity used weak metadata fallback")
	}
}

func TestScanTypedClassification(t *testing.T) {
	tests := []struct {
		name       string
		input      classificationInput
		state      app.ScanState
		management app.ScanState
		diagnostic app.ScanState
		action     app.ScanAction
	}{
		{"managed", classificationInput{managed: true, matched: true, bound: true}, app.ScanStateManaged, app.ScanStateManaged, app.ScanStateManaged, app.ScanActionNone},
		{"unmanaged", classificationInput{}, app.ScanStateUnmanaged, app.ScanStateUnmanaged, app.ScanStateUnmanaged, app.ScanActionAdopt},
		{"same content", classificationInput{sameContent: true}, app.ScanStateSameContent, app.ScanStateSameContent, app.ScanStateSameContent, app.ScanActionLinkExisting},
		{"name conflict", classificationInput{nameConflict: true}, app.ScanStateNameConflict, app.ScanStateNameConflict, app.ScanStateNameConflict, app.ScanActionConflict},
		{"broken link", classificationInput{managed: true, broken: true}, app.ScanStateBrokenLink, app.ScanStateManaged, app.ScanStateBrokenLink, app.ScanActionNone},
		{"drifted", classificationInput{managed: true, matched: true, drifted: true}, app.ScanStateDrifted, app.ScanStateManaged, app.ScanStateDrifted, app.ScanActionNone},
		{"update available", classificationInput{managed: true, matched: true, updateAvailable: true}, app.ScanStateUpdateAvailable, app.ScanStateManaged, app.ScanStateUpdateAvailable, app.ScanActionNone},
		{"pending recovery", classificationInput{managed: true, pending: true}, app.ScanStatePendingRecovery, app.ScanStateManaged, app.ScanStatePendingRecovery, app.ScanActionNone},
		{"error", classificationInput{hasError: true}, app.ScanStateError, app.ScanStateUnmanaged, app.ScanStateError, app.ScanActionNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classification := classifyScanItem(tt.input)
			if classification.state != tt.state || classification.management != tt.management || classification.diagnostic != tt.diagnostic || classification.action != tt.action {
				t.Fatalf("classification = %+v, want state=%q management=%q diagnostic=%q action=%q", classification, tt.state, tt.management, tt.diagnostic, tt.action)
			}
		})
	}
}

func TestScanPreviewActionReflectsIntent(t *testing.T) {
	service, _, home := testService(t)
	writeSkill(t, filepath.Join(agent.Cursor{}.GlobalSkillDir(home), "candidate"), "candidate", "content")
	for _, tt := range []struct {
		name   string
		adopt  bool
		action app.ScanAction
	}{
		{"import", false, app.ScanActionImport},
		{"adopt", true, app.ScanActionAdopt},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true, Adopt: tt.adopt})
			if err != nil || len(result.Items) != 1 || result.Items[0].Action != tt.action {
				t.Fatalf("preview = %+v, %v", result, err)
			}
		})
	}
}

func TestScanReportsPendingTargetThatIsAbsent(t *testing.T) {
	service, paths, home := testService(t)
	root := agent.Cursor{}.GlobalSkillDir(home)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "pending")
	writeSkill(t, libraryPath, "pending", "content")
	skill := config.Skill{ID: "local/pending", Name: "pending", Hash: mustHash(t, libraryPath)}
	target := filepath.Join(root, "pending")
	op, err := link.NewCleanupOperation("pending", config.Scope{Agent: "cursor"}, target, skill.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{skill}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Target != target || result.Items[0].State != app.ScanStatePendingRecovery {
		t.Fatalf("pending inventory = %+v", result)
	}
	item := result.Items[0]
	if item.ManagementState != app.ScanStateManaged || item.DiagnosticState != app.ScanStatePendingRecovery || len(item.Issues) == 0 {
		t.Fatalf("pending states/issues = %+v", item)
	}
}

func TestScanReportsPendingWhenDiscoveryCannotUseTarget(t *testing.T) {
	for _, scenario := range []string{"regular-file", "missing-root", "symlink-root"} {
		t.Run(scenario, func(t *testing.T) {
			service, paths, home := testService(t)
			root := agent.Cursor{}.GlobalSkillDir(home)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			libraryPath := filepath.Join(paths.LibrarySkills, "local", "pending")
			writeSkill(t, libraryPath, "pending", "content")
			skill := config.Skill{ID: "local/pending", Name: "pending", Hash: mustHash(t, libraryPath)}
			target := filepath.Join(root, "pending")
			op, err := link.NewCleanupOperation("pending", config.Scope{Agent: "cursor"}, target, skill.ID, "test")
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.New()
			cfg.Library.Skills = []config.Skill{skill}
			cfg.PendingOperations = []config.PendingOperation{op}
			if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "regular-file":
				if err := os.WriteFile(target, []byte("sentinel"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "missing-root":
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
			case "symlink-root":
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
				outside := t.TempDir()
				if err := os.Symlink(outside, root); err != nil {
					t.Fatal(err)
				}
			}

			result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("pending items = %+v", result.Items)
			}
			item := result.Items[0]
			if item.Key == "" || item.Target != target || item.DiagnosticState != app.ScanStatePendingRecovery || len(item.Issues) == 0 {
				t.Fatalf("pending item = %+v", item)
			}
			wantMessage := map[string]string{"regular-file": "regular file", "missing-root": "root is missing", "symlink-root": "root is a symlink"}[scenario]
			if !strings.Contains(item.Issues[0].Message, wantMessage) {
				t.Fatalf("pending issue = %+v, want %q", item.Issues, wantMessage)
			}
		})
	}
}

func TestScanPendingDoesNotDuplicateNormallyDiscoveredItem(t *testing.T) {
	service, paths, home := testService(t)
	root := agent.Cursor{}.GlobalSkillDir(home)
	target := filepath.Join(root, "pending")
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "pending")
	writeSkill(t, libraryPath, "pending", "content")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(libraryPath, target); err != nil {
		t.Fatal(err)
	}
	skill := config.Skill{ID: "local/pending", Name: "pending", Hash: mustHash(t, libraryPath)}
	op, err := link.NewCleanupOperation("pending", config.Scope{Agent: "cursor"}, target, skill.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{skill}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].DiagnosticState != app.ScanStatePendingRecovery {
		t.Fatalf("pending inventory = %+v", result.Items)
	}
}

func TestScanRejectsRootSymlinkAndNestedEscape(t *testing.T) {
	service, _, home := testService(t)
	outside := t.TempDir()
	writeSkill(t, filepath.Join(outside, "root-skill"), "root-skill", "outside sentinel")
	root := agent.Cursor{}.GlobalSkillDir(home)
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 0 || result.Exit != app.ExitPartial || len(result.Warnings) == 0 {
		t.Fatalf("root symlink result = %+v", result)
	}
	if len(result.Issues) != 1 || result.Issues[0].Origin != "g/cursor" || result.Issues[0].Path != root || result.Issues[0].State != app.ScanStateError {
		t.Fatalf("root symlink issue = %+v", result.Issues)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "nested")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "root-skill", "SKILL.md"), filepath.Join(inside, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	result, err = service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 0 || result.Exit != app.ExitPartial {
		t.Fatalf("nested escape result = %+v", result)
	}
}

func TestScanAllowsExplicitExternalLinkWithoutChangingIt(t *testing.T) {
	service, _, home := testService(t)
	outside := filepath.Join(t.TempDir(), "external")
	writeSkill(t, outside, "external", "allowed source")
	root := agent.Cursor{}.GlobalSkillDir(home)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "external")
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Target != target || result.Items[0].Discovered.Name != "external" {
		t.Fatalf("external link result = %+v", result)
	}
	if got, err := os.Readlink(target); err != nil || got != outside {
		t.Fatalf("external link changed: %q, %v", got, err)
	}
}

func TestScanFailsClosedWhenRootOrTargetIsReplaced(t *testing.T) {
	for _, replace := range []string{"root", "target"} {
		t.Run(replace, func(t *testing.T) {
			service, _, home := testService(t)
			root := agent.Cursor{}.GlobalSkillDir(home)
			target := filepath.Join(root, "demo")
			writeSkill(t, target, "demo", "inside")
			outside := filepath.Join(t.TempDir(), "outside")
			writeSkill(t, outside, "outside", "sentinel")
			before, err := os.ReadFile(filepath.Join(outside, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			replaced := false
			service.deps.Inspect = func(path, libraryRoot string) (link.State, error) {
				if !replaced {
					replaced = true
					if replace == "root" {
						old := root + ".old"
						if err := os.Rename(root, old); err != nil {
							return link.State{}, err
						}
						if err := os.Symlink(filepath.Dir(outside), root); err != nil {
							return link.State{}, err
						}
					} else {
						if err := os.Rename(target, target+".old"); err != nil {
							return link.State{}, err
						}
						if err := os.Symlink(outside, target); err != nil {
							return link.State{}, err
						}
					}
				}
				return link.Inspect(path, libraryRoot)
			}
			result, err := service.Scan(context.Background(), app.ScanRequest{Agent: "cursor", DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 0 || result.Exit != app.ExitPartial {
				t.Fatalf("replacement result = %+v", result)
			}
			after, err := os.ReadFile(filepath.Join(outside, "SKILL.md"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("outside sentinel changed: %q, %v", after, err)
			}
		})
	}
}

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

func TestScanAdoptDoesNotRecoverExistingPending(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "pending recovery") || len(result.Items) != 0 {
		t.Fatalf("pending scan = %+v, %v", result, err)
	}
	cfg = loadConfig(t, paths)
	if len(cfg.PendingOperations) != 1 {
		t.Fatalf("pending operation was changed: %+v", cfg.PendingOperations)
	}
	info, err := os.Lstat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("pending target was changed: %v %v", info, err)
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

func fmtHash(value []byte) string { return hex.EncodeToString(value) }

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[rel] = "dir"
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			result[rel] = "link:" + target
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = "file:" + string(content)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return result
}

func selectorFor(item app.ScanItem) app.ScanSelector {
	return app.ScanSelector{
		Key: item.Key, Origin: item.Origin, Target: item.Target,
		ExpectedHash: item.ContentHash, ExpectedObjectID: item.ObjectID,
		ExpectedRootID: item.RootObjectID, ExpectedState: item.State,
		ExpectedSkillID: item.Skill.ID, ExpectedLibraryHash: item.MatchedLibraryActualHash,
	}
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "fake" }
func (fakeFileInfo) Size() int64        { return 1 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Unix(1, 0) }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }
