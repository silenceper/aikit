package tui

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func (m Model) rows() []row {
	mode := m.Mode
	if mode == ModeFilter {
		mode = m.filterParent
	}
	if mode == ModeUpdates {
		var rows []row
		for _, item := range m.Snapshot.Updates.Results {
			if item.State == updatecheck.StateUpdateAvailable {
				rows = append(rows, row{Key: item.SkillID, ID: item.SkillID, Name: skillName(m.Snapshot.Config.Library.Skills, item.SkillID), State: "Update available", Detail: shortOID(item.Current) + " -> " + shortOID(item.Remote), Severity: rowSeverityWarning})
			}
		}
		return m.filtered(rows)
	}
	if mode == ModeScan {
		return m.scanRows(m.Scan.Items)
	}
	if mode == ModeAddSelect {
		rows := make([]row, 0, len(m.AddPreview.Candidates))
		for _, candidate := range m.AddPreview.Candidates {
			rows = append(rows, row{Key: candidate.RelativePath, ID: candidate.RelativePath, Name: candidate.Name, State: "Discovered", Detail: candidate.Description, Severity: rowSeverityInfo})
		}
		return m.filtered(rows)
	}
	if mode == ModeProjectAgents {
		project, projectOK := findProject(m.Snapshot.Config.Projects, m.pendingID)
		rows := make([]row, 0, len(agent.Names()))
		for _, name := range agent.Names() {
			configured := projectOK && contains(project.Agents, name)
			state := "Available"
			if configured {
				state = "Configured"
			}
			rows = append(rows, row{Key: "project-agent:" + name, ID: name, Name: name, State: state, Enabled: configured, Severity: enabledSeverity(configured)})
		}
		return m.filtered(rows)
	}
	if mode == ModeScopePicker || mode == ModePresetPicker {
		rows := make([]row, 0, len(m.Picker.Choices))
		for index, choice := range m.Picker.Choices {
			rows = append(rows, row{Key: fmt.Sprintf("picker:%d:%s", index, choice.Label), ID: choice.Label, Name: choice.Label, State: "Select", Detail: choice.Label})
		}
		return m.filtered(rows)
	}
	if mode == ModeWorkspaceSkills {
		rows := make([]row, 0, len(m.Snapshot.Config.Library.Skills))
		for _, skill := range m.Snapshot.Config.Library.Skills {
			rows = append(rows, row{Key: "workspace-skill:" + skill.ID, ID: skill.ID, Name: skill.Name, Source: skill.Source, State: "Select", Detail: skill.Description})
		}
		return m.filtered(rows)
	}

	var rows []row
	switch m.ActiveView {
	case ViewOverview:
		rows = m.overviewRows()
	case ViewLibrary:
		for _, skill := range m.Snapshot.Config.Library.Skills {
			state, severity := updatePresentation(m.Snapshot.Updates, skill.ID)
			rows = append(rows, row{Key: "library:" + skill.ID, ID: skill.ID, Name: skill.Name, Source: skill.Source, State: state, Detail: skill.Description, Severity: severity})
		}
		rows = m.filterLibraryRows(rows)
	case ViewWorkspaces:
		rows = m.workspaceRows()
	case ViewPresets:
		if m.Scope.Level == "preset-skills" {
			if preset, ok := findPreset(m.Snapshot.Config.Presets, m.Scope.Preset); ok {
				for _, skill := range m.Snapshot.Config.Library.Skills {
					enabled := m.Selected[skill.ID]
					rows = append(rows, row{Key: "preset:" + preset.Name + ":" + skill.ID, ID: skill.ID, Name: skill.Name, State: toggleState(enabled), Enabled: enabled, Severity: enabledSeverity(enabled)})
				}
			}
		} else {
			for _, preset := range m.Snapshot.Config.Presets {
				usage := presetUsage(m.Snapshot.Config, preset.Name)
				skillNoun := "skills"
				if len(preset.Skills) == 1 {
					skillNoun = "skill"
				}
				rows = append(rows, row{Key: "preset:" + preset.Name, ID: preset.Name, Name: preset.Name, State: fmt.Sprintf("%d %s · %d scopes", len(preset.Skills), skillNoun, len(usage)), Detail: strings.Join(usage, " · ")})
			}
		}
	case ViewConfiguration:
		rows = []row{
			{Key: "configuration:config", ID: "config", Name: "Config file", Source: firstNonEmpty(m.Config.Config, "Loading..."), State: "Read only", Severity: rowSeverityInfo},
			{Key: "configuration:library", ID: "library", Name: "Library", Source: firstNonEmpty(m.Config.Library, "Loading..."), State: "Managed content", Severity: rowSeverityInfo},
			{Key: "configuration:cache", ID: "cache", Name: "Cache", Source: firstNonEmpty(m.Config.Cache, "Loading..."), State: "Git metadata", Severity: rowSeverityInfo},
			{Key: "configuration:validation", ID: "validation", Name: "Validation", State: m.configurationValidationLabel(), Detail: m.ConfigValidationDisplay.Message, Severity: m.configurationValidationSeverity()},
		}
	case ViewMigration:
		rows = m.scanRows(m.Inventory.Items)
	case ViewStatus:
		for i, item := range m.Snapshot.Status.Items {
			id := item.SkillID
			if id == "" {
				id = fmt.Sprintf("%s:%d", item.Kind, i)
			}
			rows = append(rows, row{Key: statusItemKey(item), ID: id, Name: firstNonEmpty(item.Name, item.SkillID, item.Path), Source: item.Path, State: humanState(string(item.Kind)), Detail: item.Message, Severity: statusSeverity(item.Kind)})
		}
		for i, issue := range m.Inventory.Issues {
			rows = append(rows, row{Key: fmt.Sprintf("inventory-issue:%d:%s", i, issue.Origin), ID: issue.Origin, Name: firstNonEmpty(issue.Origin, issue.Path, "Inventory"), State: humanState(string(issue.State)), Detail: issue.Message, Severity: scanStateSeverity(issue.State)})
		}
		for _, failed := range m.Snapshot.Updates.Results {
			if failed.State == updatecheck.StateCheckFailed {
				rows = append(rows, row{Key: "status:update-failure:" + failed.SkillID, ID: "update-failure:" + failed.SkillID, Name: skillName(m.Snapshot.Config.Library.Skills, failed.SkillID), State: "Update check failed", Detail: failed.Error, Severity: rowSeverityError})
			}
		}
		seenWarnings := make(map[string]bool)
		for _, warning := range m.Snapshot.Updates.Warnings {
			if seenWarnings[warning] {
				continue
			}
			seenWarnings[warning] = true
			rows = append(rows, row{Key: "status:update-warning:" + base64.RawURLEncoding.EncodeToString([]byte(warning)), ID: warning, Name: "Update check warning", State: "Update check failed", Detail: warning, Severity: rowSeverityError})
		}
	}
	preserveWorkspaceOrder := m.ActiveView == ViewWorkspaces && (m.Scope.Level == "" || m.Scope.Level == "workspace-agents" || m.Scope.Level == "project-targets")
	if m.ActiveView != ViewMigration && m.ActiveView != ViewOverview && !preserveWorkspaceOrder {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].selectionKey() < rows[j].selectionKey() })
	}
	return m.filtered(rows)
}

func (m Model) configurationValidationLabel() string {
	if !m.ConfigValidationDisplay.Attempted {
		return "Not validated"
	}
	if m.ConfigValidationDisplay.Valid {
		return "Valid"
	}
	return "Invalid"
}

func (m Model) configurationValidationSeverity() rowSeverity {
	if !m.ConfigValidationDisplay.Attempted {
		return rowSeverityInfo
	}
	if m.ConfigValidationDisplay.Valid {
		return rowSeveritySuccess
	}
	return rowSeverityError
}

func (m Model) attentionRows() []row {
	rows := make([]row, 0, len(m.Inventory.Items)+len(m.Snapshot.Status.Items)+len(m.Inventory.Issues))
	inventoryTargets := m.inventoryTargetSet()
	for _, operation := range m.Snapshot.Config.PendingOperations {
		rows = append(rows, row{Key: "attention:recovery:" + operation.ID, ID: operation.ID, Name: "Recovery required", State: "Error", Detail: firstNonEmpty(operation.SkillID, operation.ID), Severity: rowSeverityRecovery, DestinationAction: ActionRecovery})
	}
	for _, item := range m.Snapshot.Status.Items {
		if item.Path != "" && inventoryTargets[filepath.Clean(item.Path)] {
			continue
		}
		severity := statusSeverity(item.Kind)
		key := statusItemKey(item)
		rows = append(rows, row{Key: "attention:" + key, ID: item.SkillID, Name: firstNonEmpty(item.Name, item.SkillID, humanState(string(item.Kind))), State: humanState(string(item.Kind)), Detail: firstNonEmpty(item.Message, "Review local status"), Severity: severity, DestinationView: ViewStatus, DestinationKey: key})
	}
	for _, item := range m.Snapshot.Updates.Results {
		switch item.State {
		case updatecheck.StateUpdateAvailable:
			rows = append(rows, row{
				Key:             "attention:update:" + base64.RawURLEncoding.EncodeToString([]byte(item.SkillID)),
				ID:              item.SkillID,
				Name:            skillName(m.Snapshot.Config.Library.Skills, item.SkillID),
				State:           "Update available",
				Detail:          shortOID(item.Current) + " -> " + shortOID(item.Remote),
				Severity:        rowSeverityWarning,
				DestinationMode: ModeUpdates,
				DestinationKey:  item.SkillID,
			})
		case updatecheck.StateCheckFailed:
			rows = append(rows, row{
				Key:             "attention:update-failure:" + base64.RawURLEncoding.EncodeToString([]byte(item.SkillID)),
				ID:              item.SkillID,
				Name:            "Update check failed · " + skillName(m.Snapshot.Config.Library.Skills, item.SkillID),
				State:           "Update check failed",
				Detail:          firstNonEmpty(item.Error, "Update check failed"),
				Severity:        rowSeverityError,
				DestinationView: ViewStatus,
				DestinationKey:  "status:update-failure:" + item.SkillID,
			})
		}
	}
	seenWarnings := make(map[string]bool)
	for _, warning := range m.Snapshot.Updates.Warnings {
		if seenWarnings[warning] {
			continue
		}
		seenWarnings[warning] = true
		rows = append(rows, row{
			Key:             "attention:update-warning:" + base64.RawURLEncoding.EncodeToString([]byte(warning)),
			ID:              warning,
			Name:            "Update check warning",
			State:           "Update check failed",
			Detail:          warning,
			Severity:        rowSeverityError,
			DestinationView: ViewStatus,
			DestinationKey:  "status:update-warning:" + base64.RawURLEncoding.EncodeToString([]byte(warning)),
		})
	}
	for index, issue := range m.Inventory.Issues {
		rows = append(rows, row{Key: "attention:issue:" + issue.Origin + "\x00" + issue.Path, ID: issue.Origin, Name: firstNonEmpty(issue.Origin, "Inventory issue"), State: "Error", Detail: firstNonEmpty(issue.Message, "Review inventory"), Severity: rowSeverityError, DestinationView: ViewStatus, DestinationKey: fmt.Sprintf("inventory-issue:%d:%s", index, issue.Origin)})
	}
	for _, item := range m.Inventory.Items {
		state := item.State
		if state == "" {
			state = item.ManagementState
		}
		severity, relevant := scanAttentionSeverity(state)
		if !relevant {
			continue
		}
		name := firstNonEmpty(item.Skill.Name, item.Discovered.Name, item.MatchedLibraryID, filepath.Base(item.Target), item.Key)
		rows = append(rows, row{Key: "attention:inventory:" + item.Key, ID: item.Key, Name: name, State: humanState(string(state)), Detail: firstNonEmpty(humanAction(item.Action), "Review in Migration"), Severity: severity, DestinationView: ViewMigration, DestinationKey: item.Key})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) < severityRank(rows[j].Severity)
		}
		return rows[i].selectionKey() < rows[j].selectionKey()
	})
	return rows
}

func scanAttentionSeverity(state app.ScanState) (rowSeverity, bool) {
	switch state {
	case app.ScanStateError, app.ScanStatePendingRecovery:
		return rowSeverityError, true
	case app.ScanStateNameConflict, app.ScanStateDrifted, app.ScanStateBrokenLink:
		return rowSeverityConflict, true
	case app.ScanStateUnmanaged, app.ScanStateUpdateAvailable:
		return rowSeverityWarning, true
	case app.ScanStateManaged, app.ScanStateSameContent, "":
		return rowSeveritySuccess, false
	default:
		return rowSeverityInfo, true
	}
}

func statusSeverity(kind status.Kind) rowSeverity {
	switch kind {
	case status.AdoptRecovery, status.PendingCleanup, status.IOError, status.Missing, status.LibraryMissing:
		return rowSeverityError
	case status.Conflict, status.ScopeConflict, status.OrphanedLink:
		return rowSeverityConflict
	case status.Unmanaged:
		return rowSeverityWarning
	default:
		return rowSeverityInfo
	}
}

func statusItemKey(item status.Item) string {
	fields := []string{string(item.Kind), item.Scope.Project, item.Scope.ProjectPath, item.Scope.Agent, item.Path, item.SkillID, item.Operation}
	encoded := make([]string, 0, len(fields))
	for _, field := range fields {
		encoded = append(encoded, base64.RawURLEncoding.EncodeToString([]byte(field)))
	}
	return "status:item:" + strings.Join(encoded, ".")
}

func (m Model) filterLibraryRows(rows []row) []row {
	filtered := make([]row, 0, len(rows))
	for _, current := range rows {
		if m.LibraryStateFilter == LibraryStateUpdateAvailable && current.State != "Update available" {
			continue
		}
		if m.LibraryStateFilter == LibraryStateManaged && current.State == "Update available" {
			continue
		}
		git := strings.Contains(current.Source, "://") || strings.HasPrefix(current.Source, "git@")
		if m.LibrarySourceFilter == LibrarySourceGit && !git {
			continue
		}
		if m.LibrarySourceFilter == LibrarySourceLocal && git {
			continue
		}
		filtered = append(filtered, current)
	}
	return filtered
}

func (m Model) scanRows(items []app.ScanItem) []row {
	rows := make([]row, 0, len(items))
	for _, item := range items {
		if m.Ignored[item.Key] {
			continue
		}
		id := firstNonEmpty(item.Skill.ID, item.Discovered.RelativePath, item.MatchedLibraryID, item.Target, item.Key)
		name := item.Skill.Name
		if name == "" {
			name = item.Discovered.Name
		}
		if name == "" && item.Target != "" {
			name = filepath.Base(filepath.Clean(item.Target))
		}
		name = firstNonEmpty(name, item.MatchedLibraryID, id)
		state := item.State
		if state == "" {
			state = item.ManagementState
		}
		rows = append(rows, row{
			Key: item.Key, ID: id, Name: name, Origin: item.Origin, Target: item.Target,
			Source: item.Origin, State: humanState(string(state)), Action: humanAction(item.Action),
			Detail: inventoryDetail(item), Enabled: item.State == app.ScanStateManaged, Severity: scanStateSeverity(state),
		})
	}
	return m.filtered(rows)
}

func (m Model) workspaceRows() []row {
	if m.Scope.Level == "workspace-global" {
		return globalWorkspaceRows(m.Snapshot.Config)
	}
	if m.Scope.Level == "agent-skills" {
		return workspaceBindingRows(m.Snapshot.Config, m.Scope)
	}
	if m.Scope.Level == "project-targets" || m.Scope.Level == "project-skills" {
		return m.projectRows()
	}
	if m.Scope.Level == "workspace-agents" {
		var rows []row
		for _, name := range agent.Names() {
			count, owners := workspaceTargetSummary(m.Snapshot.Config, Scope{Agent: name, Level: "agent-skills"})
			rows = append(rows, row{Key: "agent:" + name, ID: name, Name: name, State: fmt.Sprintf("%d available", count), Detail: owners})
		}
		return rows
	}
	if m.Scope.Level == "workspace-projects" {
		return m.projectRows()
	}
	return m.projectRows()
}

func (m Model) projectRows() []row {
	project, ok := findProject(m.Snapshot.Config.Projects, m.Scope.Project)
	switch m.Scope.Level {
	case "project-targets":
		if !ok {
			return nil
		}
		commonCount, commonOwners := workspaceTargetSummary(m.Snapshot.Config, Scope{Project: project.Name, Level: "project-skills"})
		common := row{Key: "project:" + project.Name + ":common", ID: "common", Name: "Common", State: fmt.Sprintf("%d available", commonCount), Detail: commonOwners}
		if len(project.Agents) == 0 {
			common.Detail = "No agents configured · choose Manage agents"
		}
		rows := []row{common}
		for _, name := range project.Agents {
			count, owners := workspaceTargetSummary(m.Snapshot.Config, Scope{Project: project.Name, Agent: name, Level: "project-skills"})
			rows = append(rows, row{Key: "project:" + project.Name + ":" + name, ID: name, Name: name, State: fmt.Sprintf("%d available", count), Detail: owners})
		}
		return rows
	case "project-skills":
		if !ok {
			return nil
		}
		return workspaceBindingRows(m.Snapshot.Config, m.Scope)
	default:
		var rows []row
		for _, current := range m.Snapshot.Config.Projects {
			rows = append(rows, row{Key: "project:" + current.Name, ID: current.Name, Name: current.Name, Source: current.Path, State: fmt.Sprintf("%d agents", len(current.Agents))})
		}
		return rows
	}
}

func (m Model) filtered(rows []row) []row {
	value := m.Filter
	if m.Mode == ModeFilter {
		value = m.FilterDraft
	}
	query := strings.ToLower(strings.TrimSpace(value))
	if query == "" {
		return rows
	}
	filtered := make([]row, 0, len(rows))
	for _, current := range rows {
		if strings.Contains(strings.ToLower(current.ID+" "+current.Name+" "+current.Source+" "+current.State+" "+current.Action), query) {
			filtered = append(filtered, current)
		}
	}
	return filtered
}

type attentionSummary struct{ migration, status, updates int }

func (m Model) attentionCounts() attentionSummary {
	summary := attentionSummary{status: len(m.Snapshot.Status.Items) + len(m.Inventory.Issues)}
	for _, item := range m.Inventory.Items {
		if item.State != app.ScanStateManaged && item.State != app.ScanStateSameContent {
			summary.migration++
		}
	}
	for _, item := range m.Snapshot.Updates.Results {
		if item.State == updatecheck.StateUpdateAvailable {
			summary.updates++
		} else if item.State == updatecheck.StateCheckFailed {
			summary.status++
		}
	}
	seenWarnings := make(map[string]bool)
	for _, warning := range m.Snapshot.Updates.Warnings {
		if !seenWarnings[warning] {
			seenWarnings[warning] = true
			summary.status++
		}
	}
	return summary
}

func (m Model) inventoryTargetSet() map[string]bool {
	targets := make(map[string]bool, len(m.Inventory.Items))
	for _, item := range m.Inventory.Items {
		if item.Target != "" {
			targets[filepath.Clean(item.Target)] = true
		}
	}
	return targets
}

func humanState(value string) string {
	if value == "" {
		return "Ready"
	}
	words := strings.Fields(strings.ReplaceAll(value, "-", " "))
	for i := range words {
		if i == 0 {
			words[i] = title(words[i])
		}
	}
	return strings.Join(words, " ")
}

func humanAction(action app.ScanAction) string {
	switch action {
	case app.ScanActionImport:
		return "Import"
	case app.ScanActionAdopt:
		return "Adopt"
	case app.ScanActionLinkExisting:
		return "Link existing"
	case app.ScanActionConflict:
		return "Conflict"
	default:
		return "Review"
	}
}

func inventoryDetail(item app.ScanItem) string {
	scope := "Global"
	if item.Project != "" || item.Scope.Project != "" {
		scope = "Project"
	}
	parts := []string{scope}
	agent := item.Agent
	if agent == "" {
		agent = item.Scope.Agent
	}
	if agent != "" {
		parts = append(parts, agent)
	}
	project := item.Project
	if project == "" {
		project = item.Scope.Project
	}
	if project != "" {
		parts = append(parts, project)
	}
	return strings.Join(parts, " · ")
}

func updatePresentation(report updatecheck.CheckReport, id string) (string, rowSeverity) {
	for _, item := range report.Results {
		if item.SkillID == id {
			switch item.State {
			case updatecheck.StateCurrent:
				return humanState(string(item.State)), rowSeveritySuccess
			case updatecheck.StateUpdateAvailable:
				return humanState(string(item.State)), rowSeverityWarning
			case updatecheck.StateCheckFailed:
				return humanState(string(item.State)), rowSeverityError
			default:
				return humanState(string(item.State)), rowSeverityInfo
			}
		}
	}
	return "Managed", rowSeveritySuccess
}

func scanStateSeverity(state app.ScanState) rowSeverity {
	switch state {
	case app.ScanStatePendingRecovery:
		return rowSeverityRecovery
	case app.ScanStateError:
		return rowSeverityError
	case app.ScanStateNameConflict, app.ScanStateDrifted, app.ScanStateBrokenLink:
		return rowSeverityConflict
	case app.ScanStateUnmanaged, app.ScanStateUpdateAvailable:
		return rowSeverityWarning
	case app.ScanStateManaged, app.ScanStateSameContent:
		return rowSeveritySuccess
	default:
		return rowSeverityInfo
	}
}

func skillName(skills []config.Skill, id string) string {
	for _, skill := range skills {
		if skill.ID == id {
			return skill.Name
		}
	}
	return id
}

func findProject(projects []config.Project, name string) (config.Project, bool) {
	for _, project := range projects {
		if project.Name == name {
			return project, true
		}
	}
	return config.Project{}, false
}

func findPreset(presets []config.Preset, name string) (config.Preset, bool) {
	for _, preset := range presets {
		if preset.Name == name {
			return preset, true
		}
	}
	return config.Preset{}, false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "-"
}

func toggleState(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Available"
}

func enabledSeverity(enabled bool) rowSeverity {
	if enabled {
		return rowSeveritySuccess
	}
	return rowSeverityInfo
}

func shortOID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func isUnmanaged(item status.Item) bool { return item.Kind == status.Unmanaged }

func (m Model) selectedStatusItem() (status.Item, bool) {
	rows := m.rows()
	if m.ActiveView != ViewStatus || m.Cursor < 0 || m.Cursor >= len(rows) {
		return status.Item{}, false
	}
	for _, item := range m.Snapshot.Status.Items {
		if rows[m.Cursor].Key == statusItemKey(item) {
			return item, true
		}
	}
	return status.Item{}, false
}

func (m Model) selectedStatusCanSync() bool {
	item, ok := m.selectedStatusItem()
	if !ok {
		return false
	}
	return statusItemCanSync(item)
}

func statusItemCanSync(item status.Item) bool {
	switch item.Kind {
	case status.Missing, status.LibraryMissing, status.Conflict, status.ScopeConflict, status.OrphanedLink, status.PendingCleanup:
		return true
	default:
		return false
	}
}
