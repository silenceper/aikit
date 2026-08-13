// Package scope expands ledger bindings into the effective skill links for an
// agent or a project/agent pair. It performs no filesystem operations.
package scope

import (
	"fmt"
	"sort"

	"github.com/silenceper/aikit/pkg/config"
)

type IssueKind string

const (
	IssueMissingReference IssueKind = "missing-reference"
	IssueNameConflict     IssueKind = "name-conflict"
	IssueScopeConflict    IssueKind = "scope-conflict"
)

type Issue struct {
	Kind    IssueKind
	Scope   config.Scope
	Name    string
	IDs     []string
	Message string
}

type View struct {
	Scope      config.Scope
	Skills     map[string]config.Skill
	Suppressed map[string]config.Skill
	Issues     []Issue
}

func (v View) Desired() map[string]string {
	result := make(map[string]string, len(v.Skills))
	for name, skill := range v.Skills {
		result[name] = skill.ID
	}
	return result
}

// ValidateEffective checks every configured global and project target. It is
// intended for mutation preflight before a new ledger is checkpointed.
func ValidateEffective(cfg *config.Config) []Issue {
	if cfg == nil {
		return []Issue{{Kind: IssueMissingReference, Message: "nil config"}}
	}
	var issues []Issue
	agents := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		agents = append(agents, name)
	}
	sort.Strings(agents)
	for _, name := range agents {
		issues = append(issues, Global(cfg, name).Issues...)
	}
	projects := append([]config.Project(nil), cfg.Projects...)
	sort.SliceStable(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	for _, project := range projects {
		for _, name := range project.Agents {
			issues = append(issues, Project(cfg, project, name).Issues...)
		}
	}
	return issues
}

func Global(cfg *config.Config, agent string) View {
	s := config.Scope{Agent: agent}
	v := View{Scope: s, Skills: map[string]config.Skill{}, Suppressed: map[string]config.Skill{}}
	if cfg == nil {
		v.Issues = append(v.Issues, Issue{Kind: IssueMissingReference, Scope: s, Message: "nil config"})
		return v
	}
	addBinding(cfg, s, cfg.Agents[agent], v.Skills, map[string]struct{}{}, &v.Issues)
	return v
}

func Project(cfg *config.Config, project config.Project, agent string) View {
	s := config.Scope{Project: project.Name, ProjectPath: project.Path, Agent: agent}
	v := View{Scope: s, Skills: map[string]config.Skill{}, Suppressed: map[string]config.Skill{}}
	if cfg == nil {
		v.Issues = append(v.Issues, Issue{Kind: IssueMissingReference, Scope: s, Message: "nil config"})
		return v
	}

	projectSkills := map[string]config.Skill{}
	blockedNames := map[string]struct{}{}
	addBinding(cfg, s, project.Binding, projectSkills, blockedNames, &v.Issues)
	addBinding(cfg, s, project.AgentBindings[agent], projectSkills, blockedNames, &v.Issues)
	global := Global(cfg, agent)
	// Global same-layer errors are attached to the global target only. The
	// project view only needs them when comparing a concrete, valid entry.
	for name, skill := range projectSkills {
		if inherited, ok := global.Skills[name]; ok {
			if inherited.ID == skill.ID {
				v.Suppressed[name] = skill
				continue
			}
			v.Issues = append(v.Issues, Issue{
				Kind: IssueScopeConflict, Scope: s, Name: name,
				IDs:     []string{inherited.ID, skill.ID},
				Message: fmt.Sprintf("global and project bindings use different ids for %q", name),
			})
			continue
		}
		v.Skills[name] = skill
	}
	return v
}

func addBinding(cfg *config.Config, s config.Scope, binding config.Binding, dst map[string]config.Skill, blockedNames map[string]struct{}, issues *[]Issue) {
	ids := make([]string, 0, len(binding.Skills))
	for _, presetName := range binding.Presets {
		preset, ok := findPreset(cfg, presetName)
		if !ok {
			*issues = append(*issues, Issue{Kind: IssueMissingReference, Scope: s, Name: presetName, Message: "preset is not present in the ledger"})
			continue
		}
		ids = append(ids, preset.Skills...)
	}
	ids = append(ids, binding.Skills...)
	seenID := map[string]struct{}{}
	for _, id := range ids {
		if _, ok := seenID[id]; ok {
			continue
		}
		seenID[id] = struct{}{}
		skill, ok := findSkill(cfg, id)
		if !ok {
			*issues = append(*issues, Issue{Kind: IssueMissingReference, Scope: s, IDs: []string{id}, Message: "skill is not present in the library ledger"})
			continue
		}
		if _, blocked := blockedNames[skill.Name]; blocked {
			continue
		}
		if previous, ok := dst[skill.Name]; ok && previous.ID != skill.ID {
			ids := []string{previous.ID, skill.ID}
			sort.Strings(ids)
			*issues = append(*issues, Issue{Kind: IssueNameConflict, Scope: s, Name: skill.Name, IDs: ids, Message: "two ids use the same target name"})
			delete(dst, skill.Name)
			blockedNames[skill.Name] = struct{}{}
			continue
		}
		dst[skill.Name] = skill
	}
}

func findSkill(cfg *config.Config, id string) (config.Skill, bool) {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == id {
			return skill, true
		}
	}
	return config.Skill{}, false
}

func findPreset(cfg *config.Config, name string) (config.Preset, bool) {
	for _, preset := range cfg.Presets {
		if preset.Name == name {
			return preset, true
		}
	}
	return config.Preset{}, false
}
