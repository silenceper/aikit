package agent

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRegistryOrderAndPaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "user")
	project := filepath.Join(string(filepath.Separator), "work", "repo")
	want := []struct {
		name, global, project string
	}{
		{"cursor", ".cursor/skills", ".cursor/skills"},
		{"claude-code", ".claude/skills", ".claude/skills"},
		{"codex", ".codex/skills", ".codex/skills"},
		{"copilot", ".copilot/skills", ".agents/skills"},
		{"windsurf", ".codeium/windsurf/skills", ".windsurf/skills"},
	}

	all := All()
	if len(all) != len(want) {
		t.Fatalf("All() length = %d", len(all))
	}
	for i, tt := range want {
		if all[i].Name() != tt.name {
			t.Fatalf("All()[%d].Name() = %q", i, all[i].Name())
		}
		if all[i].GlobalSkillDir(home) != filepath.Join(home, filepath.FromSlash(tt.global)) {
			t.Errorf("%s global dir = %q", tt.name, all[i].GlobalSkillDir(home))
		}
		if all[i].ProjectSkillDir(project) != filepath.Join(project, filepath.FromSlash(tt.project)) {
			t.Errorf("%s project dir = %q", tt.name, all[i].ProjectSkillDir(project))
		}
	}
}

func TestByNameAndLegacyNormalization(t *testing.T) {
	if got := NormalizeLegacyName("github-copilot"); got != "copilot" {
		t.Fatalf("NormalizeLegacyName() = %q", got)
	}
	if got := NormalizeLegacyName("cursor"); got != "cursor" {
		t.Fatalf("NormalizeLegacyName() changed canonical name: %q", got)
	}
	a, ok := ByName("github-copilot")
	if !ok || a.Name() != "copilot" {
		t.Fatalf("ByName legacy lookup = %#v, %v", a, ok)
	}
	if _, ok := ByName("unknown"); ok {
		t.Fatal("ByName accepted unknown agent")
	}

	names := make([]string, 0, len(All()))
	for _, a := range All() {
		names = append(names, a.Name())
	}
	if !reflect.DeepEqual(names, Names()) {
		t.Fatalf("Names() = %#v, registry = %#v", Names(), names)
	}
}
