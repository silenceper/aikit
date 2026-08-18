package tui

import "github.com/silenceper/aikit/internal/app"

// actionPane identifies which stable pane owns an action. Overlay actions use
// their existing modal registry and are intentionally not represented here.
type actionPane int

const (
	actionPaneCollection actionPane = iota
	actionPaneDetail
)

type paneActionSet struct {
	Collection []string
	Detail     []string
}

func (m Model) paneActions() paneActionSet {
	return paneActionSet{
		Collection: m.collectionActions(),
		Detail:     m.detailActions(),
	}
}

func (m Model) collectionActions() []string {
	switch m.ActiveView {
	case ViewLibrary:
		return []string{"Add source", "More"}
	case ViewWorkspaces:
		switch m.Scope.Level {
		case "workspace-global":
			actions := []string{"Sync preview"}
			if len(m.selectedWorkspaceSkillIDs()) > 0 {
				actions = append(actions, "More")
			}
			return actions
		case "workspace-agents":
			return []string{"Sync preview"}
		case "workspace-projects":
			return []string{"Create project"}
		case "agent-skills", "project-skills":
			return []string{"Select skills", "More"}
		}
	case ViewPresets:
		if m.Scope.Level == "preset-skills" {
			return []string{"Save"}
		}
		return []string{"Create preset"}
	case ViewStatus:
		return []string{"Refresh"}
	case ViewMigration:
		return []string{"Refresh"}
	}
	return nil
}

func (m Model) detailActions() []string {
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return nil
	}
	current := rows[m.Cursor]
	switch m.ActiveView {
	case ViewOverview:
		return []string{"Open"}
	case ViewMigration:
		actions := make([]string, 0, 3)
		if current.Action == "Adopt" || current.Action == "Import" || current.Action == "Link existing" {
			actions = append(actions, current.Action)
		}
		if m.migrationCanCompare(current.Key) {
			actions = append(actions, "Compare")
		}
		return append(actions, "Ignore")
	case ViewLibrary:
		if m.Detail && m.SkillDetail.Skill.ID == current.ID && m.SkillDetail.SkillMD != "" {
			return []string{"View SKILL.md", "More"}
		}
		return []string{"Open", "More"}
	case ViewWorkspaces:
		switch m.Scope.Level {
		case "workspace-projects":
			return []string{"Add skill", m.workspacePresetAction(), "More"}
		case "workspace-agents":
			return []string{"Open", m.workspacePresetAction()}
		case "agent-skills", "project-skills":
			if current.Direct {
				return []string{"Disable", "More"}
			}
			if current.Enabled {
				return []string{"Manage source", "More"}
			}
			return []string{"Enable", "More"}
		case "project-targets":
			if project, ok := findProject(m.Snapshot.Config.Projects, m.Scope.Project); ok && len(project.Agents) > 0 {
				return []string{"Open", m.workspacePresetAction(), "More"}
			}
			return []string{"Open", "Manage agents", "More"}
		case "workspace-global":
			return []string{"Open", m.workspacePresetAction()}
		}
	case ViewPresets:
		if m.Scope.Level == "preset-skills" {
			return []string{"Open", "More"}
		}
		return []string{"Edit members", "Apply", "More"}
	case ViewStatus:
		actions := []string{"Open"}
		if item, ok := m.selectedStatusItem(); ok && isUnmanaged(item) {
			actions = append(actions, "Adopt")
		} else if m.selectedStatusCanSync() {
			actions = append(actions, "Sync preview")
		}
		return append(actions, "More")
	}
	return nil
}

func (m Model) moreActions(pane actionPane) []string {
	if pane == actionPaneCollection {
		switch m.ActiveView {
		case ViewLibrary:
			actions := []string{"State filter", "Source filter", "Check updates"}
			selected := len(m.selectedLibraryIDs())
			if selected > 0 {
				actions = append(actions, "Enable selected", "Disable selected")
				if _, err := m.libraryBatchRequest(app.BatchUpdate); err == nil {
					actions = append(actions, "Update selected")
				}
				actions = append(actions, "Remove selected", "Clear selection")
			}
			return append(actions, "Close")
		case ViewWorkspaces:
			if m.Scope.Level == "workspace-global" && len(m.selectedWorkspaceSkillIDs()) > 0 {
				return []string{"Enable selected", "Disable selected", "Clear selection", "Close"}
			}
			if m.Scope.Level == "agent-skills" || m.Scope.Level == "project-skills" {
				return []string{"Sync preview", "Close"}
			}
		}
		return []string{"Close"}
	}

	switch m.ActiveView {
	case ViewLibrary:
		actions := make([]string, 0, 3)
		rows := m.rows()
		if m.Cursor >= 0 && m.Cursor < len(rows) {
			actions = append(actions, "Change ref", "Remove")
		}
		return append(actions, "Close")
	case ViewWorkspaces:
		switch m.Scope.Level {
		case "workspace-projects", "project-targets":
			return []string{"Rename project", "Manage agents", "Change project directory", "Remove project", "Close"}
		case "agent-skills", "project-skills":
			return []string{"Sync preview", "Close"}
		}
	case ViewPresets:
		if m.Scope.Level == "" {
			return []string{"Duplicate", "Rename", "Delete", "Close"}
		}
	case ViewStatus:
		actions := make([]string, 0, 3)
		if m.selectedStatusCanSync() {
			actions = append(actions, "Retry")
		}
		return append(actions, "Error details", "Close")
	}
	return []string{"Close"}
}
