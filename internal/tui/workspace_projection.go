package tui

import (
	"fmt"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	domainscope "github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

func globalWorkspaceRows(cfg config.Config) []row {
	agents := agent.Names()
	rows := make([]row, 0, len(cfg.Library.Skills))
	for _, skill := range cfg.Library.Skills {
		enabled := make([]string, 0, len(agents))
		owners := make([]string, 0, len(agents))
		for _, agentName := range agents {
			projection := projectBinding(cfg, cfg.Agents[agentName], skill.ID, "Direct", "Preset: ")
			if projection.enabled {
				enabled = append(enabled, agentName)
				owners = append(owners, agentName+": "+strings.Join(projection.owners, " + "))
			}
		}
		detail := "Not enabled"
		if len(enabled) > 0 {
			detail = strings.Join(owners, " · ")
		}
		rows = append(rows, row{
			Key:      "workspace-global:" + skill.ID,
			ID:       skill.ID,
			Name:     skill.Name,
			State:    fmt.Sprintf("%d/%d agents", len(enabled), len(agents)),
			Detail:   detail,
			Enabled:  len(enabled) > 0,
			Severity: enabledSeverity(len(enabled) > 0),
		})
	}
	return rows
}

type workspaceProjection struct {
	direct   bool
	enabled  bool
	conflict bool
	owners   []string
}

func workspaceBindingRows(cfg config.Config, scope Scope) []row {
	rows := make([]row, 0, len(cfg.Library.Skills))
	project, projectOK := findProject(cfg.Projects, scope.Project)
	for _, skill := range cfg.Library.Skills {
		projection := workspaceSkillProjection(cfg, project, projectOK, scope, skill)
		state := "Available"
		severity := rowSeverityInfo
		switch {
		case projection.conflict:
			state, severity = "Conflict", rowSeverityConflict
		case projection.direct:
			state, severity = "Direct", rowSeveritySuccess
		case projection.enabled && len(projection.owners) > 0:
			state, severity = projection.owners[0], rowSeveritySuccess
		}
		rows = append(rows, row{
			Key:      workspaceRowKey(scope, skill.ID),
			ID:       skill.ID,
			Name:     skill.Name,
			State:    state,
			Detail:   strings.Join(projection.owners, " · "),
			Enabled:  projection.enabled,
			Direct:   projection.direct,
			Severity: severity,
		})
	}
	return rows
}

func workspaceTargetSummary(cfg config.Config, scope Scope) (int, string) {
	count := 0
	owners := make([]string, 0, 4)
	for _, current := range workspaceBindingRows(cfg, scope) {
		if !current.Enabled {
			continue
		}
		count++
		for _, owner := range strings.Split(current.Detail, " · ") {
			category := owner
			switch {
			case strings.HasPrefix(owner, "Project common"):
				category = "Project common"
			case strings.HasPrefix(owner, "Global inherited"):
				category = "Global inherited"
			case strings.HasPrefix(owner, "Preset:"):
				category = owner
			}
			owners = appendUnique(owners, category)
		}
	}
	return count, strings.Join(owners, " · ")
}

func presetUsage(cfg config.Config, presetName string) []string {
	usage := make([]string, 0)
	for _, agentName := range agent.Names() {
		if contains(cfg.Agents[agentName].Presets, presetName) {
			usage = append(usage, "Global / "+agentName)
		}
	}
	for _, project := range cfg.Projects {
		if contains(project.Presets, presetName) {
			usage = append(usage, "Project / "+project.Name+" / Common")
		}
		for _, agentName := range project.Agents {
			if contains(project.AgentBindings[agentName].Presets, presetName) {
				usage = append(usage, "Project / "+project.Name+" / "+agentName)
			}
		}
	}
	return usage
}

func workspaceSkillProjection(cfg config.Config, project config.Project, projectOK bool, scope Scope, skill config.Skill) workspaceProjection {
	projection := workspaceProjection{}
	if scope.Project == "" {
		projection = projectBinding(cfg, cfg.Agents[scope.Agent], skill.ID, "Direct", "Preset: ")
		view := domainscope.Global(&cfg, scope.Agent)
		for _, issue := range view.Issues {
			if issue.Name == skill.Name || contains(issue.IDs, skill.ID) {
				projection.conflict = true
				break
			}
		}
		return projection
	}
	if !projectOK {
		return projection
	}
	if scope.Agent == "" {
		return projectBinding(cfg, project.Binding, skill.ID, "Project common", "Preset: ")
	}
	exact := projectBinding(cfg, project.AgentBindings[scope.Agent], skill.ID, "Direct", "Preset: ")
	projection.direct = exact.direct
	projection.owners = append(projection.owners, exact.owners...)
	common := projectBinding(cfg, project.Binding, skill.ID, "Project common", "Project common · Preset: ")
	projection.owners = appendUnique(projection.owners, common.owners...)
	global := projectBinding(cfg, cfg.Agents[scope.Agent], skill.ID, "Global inherited", "Global inherited · Preset: ")
	projection.owners = appendUnique(projection.owners, global.owners...)
	projection.enabled = len(projection.owners) > 0
	view := domainscope.Project(&cfg, project, scope.Agent)
	for _, issue := range view.Issues {
		if issue.Name == skill.Name || contains(issue.IDs, skill.ID) {
			projection.conflict = true
			break
		}
	}
	return projection
}

func projectBinding(cfg config.Config, binding config.Binding, skillID, directLabel, presetPrefix string) workspaceProjection {
	projection := workspaceProjection{}
	if contains(binding.Skills, skillID) {
		projection.direct = true
		projection.owners = append(projection.owners, directLabel)
	}
	for _, presetName := range binding.Presets {
		preset, ok := findPreset(cfg.Presets, presetName)
		if ok && contains(preset.Skills, skillID) {
			projection.owners = appendUnique(projection.owners, presetPrefix+presetName)
		}
	}
	projection.enabled = len(projection.owners) > 0
	return projection
}

func workspaceRowKey(scope Scope, skillID string) string {
	return "workspace-scope:" + scope.Project + ":" + scope.Agent + ":" + skillID
}

func appendUnique(values []string, additional ...string) []string {
	seen := make(map[string]bool, len(values)+len(additional))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additional {
		if value != "" && !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	return values
}
