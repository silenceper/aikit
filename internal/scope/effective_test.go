package scope_test

import (
	"testing"

	"github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Library: config.Library{Skills: []config.Skill{
			{ID: "one/repo/shared", Name: "shared"},
			{ID: "two/repo/shared", Name: "shared"},
			{ID: "one/repo/direct", Name: "direct"},
			{ID: "one/repo/project", Name: "project"},
		}},
		Presets: []config.Preset{{Name: "base", Skills: []string{"one/repo/shared", "one/repo/direct"}}},
		Agents: map[string]config.Binding{
			"cursor": {Presets: []string{"base"}, Skills: []string{"one/repo/direct"}},
		},
		Projects: []config.Project{{
			Name: "app", Path: "/tmp/app", Agents: []string{"cursor"},
			Binding:       config.Binding{Skills: []string{"one/repo/shared"}},
			AgentBindings: map[string]config.Binding{"cursor": {Skills: []string{"one/repo/project"}}},
		}},
	}
}

func TestGlobalExpandsPresetsAndDeduplicates(t *testing.T) {
	v := scope.Global(testConfig(), "cursor")
	if len(v.Skills) != 2 || v.Skills["shared"].ID != "one/repo/shared" || v.Skills["direct"].ID != "one/repo/direct" {
		t.Fatalf("unexpected global view: %#v", v)
	}
	if len(v.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", v.Issues)
	}
}

func TestProjectCombinesCommonAndAgentAndSuppressesSameGlobalID(t *testing.T) {
	cfg := testConfig()
	v := scope.Project(cfg, cfg.Projects[0], "cursor")
	if _, ok := v.Skills["shared"]; ok {
		t.Fatalf("same id should be suppressed from project links: %#v", v.Skills)
	}
	if v.Suppressed["shared"].ID != "one/repo/shared" || v.Skills["project"].ID != "one/repo/project" {
		t.Fatalf("unexpected project view: %#v", v)
	}

	delete(cfg.Agents, "cursor")
	v = scope.Project(cfg, cfg.Projects[0], "cursor")
	if v.Skills["shared"].ID != "one/repo/shared" {
		t.Fatalf("global disable must restore project link: %#v", v)
	}
}

func TestProjectReportsCrossLayerConflict(t *testing.T) {
	cfg := testConfig()
	cfg.Projects[0].Binding.Skills = []string{"two/repo/shared"}
	v := scope.Project(cfg, cfg.Projects[0], "cursor")
	if len(v.Issues) != 1 || v.Issues[0].Kind != scope.IssueScopeConflict {
		t.Fatalf("expected scope conflict, got %#v", v.Issues)
	}
	if len(v.Issues[0].IDs) != 2 || v.Issues[0].IDs[0] != "one/repo/shared" || v.Issues[0].IDs[1] != "two/repo/shared" {
		t.Fatalf("full ids missing: %#v", v.Issues[0])
	}
}

func TestSameLayerShortNameConflictAndCorruptReferences(t *testing.T) {
	cfg := testConfig()
	cfg.Agents["cursor"] = config.Binding{Skills: []string{"one/repo/shared", "two/repo/shared", "missing/id"}}
	v := scope.Global(cfg, "cursor")
	if len(v.Issues) != 2 {
		t.Fatalf("expected name and missing-reference issues, got %#v", v.Issues)
	}
}
