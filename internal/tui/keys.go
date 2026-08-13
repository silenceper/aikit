package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.Busy {
		return m, nil
	}
	if m.Help {
		if key == "esc" || key == "?" || key == "q" {
			m.Help = false
		}
		return m, nil
	}
	if m.Mode == ModeFilter {
		switch key {
		case "esc":
			m.Mode = m.filterParent
		case "enter":
			m.Mode = m.filterParent
		case "backspace":
			chars := []rune(m.Filter)
			if len(chars) > 0 {
				m.Filter = string(chars[:len(chars)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.Filter += string(msg.Runes)
			}
		}
		m.Cursor = 0
		m.clampCursor()
		return m, nil
	}
	if m.Mode == ModeConfirm {
		switch key {
		case "esc", "q":
			if m.confirm == ActionUpdate {
				m.Mode = ModeUpdates
			} else if m.confirm == ActionScan {
				m.Mode = ModeScan
			} else {
				m.Mode = ModeTable
			}
			m.confirm = ActionNone
			m.pendingID = ""
			m.Status = "cancelled; no changes made"
			return m, nil
		case "enter", "y":
			m.Busy = true
			switch m.confirm {
			case ActionUpdate:
				m.Status = "updating…"
				return m, updateCmd(m.ctx, m.service, m.updateRequest())
			case ActionScan:
				m.Status = "adopting…"
				return m, adoptCmd(m.ctx, m.migration, app.ScanRequest{All: true, Adopt: true, Targets: m.selectedTargets()})
			case ActionRemoveSkill:
				m.Status = "removing skill…"
				return m, removeSkillCmd(m.ctx, m.service, app.RemoveRequest{SkillID: m.pendingID})
			case ActionRemoveProject:
				m.Status = "removing project…"
				return m, removeProjectCmd(m.ctx, m.service, app.ProjectRemoveRequest{Project: m.pendingID, Confirmed: true})
			}
		}
		return m, nil
	}

	if view, ok := viewKey(key); ok {
		m.switchView(view)
		return m, nil
	}
	switch key {
	case "?":
		m.Help = true
		return m, nil
	case "/":
		m.filterParent = m.Mode
		m.Mode = ModeFilter
		m.Filter = ""
		return m, nil
	case "j", "down":
		if m.Cursor+1 < len(m.rows()) {
			m.Cursor++
		}
		return m, nil
	case "k", "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
		return m, nil
	case " ":
		return m.toggleSelected()
	case "enter":
		return m.enter()
	case "u":
		m.openUpdates()
		return m, nil
	case "d":
		if m.ActiveView == ViewLibrary {
			return m.confirmCurrent(ActionRemoveSkill)
		}
	case "x":
		if m.ActiveView == ViewProjects && m.Scope.Level == "" {
			return m.confirmCurrent(ActionRemoveProject)
		}
	case "S":
		if m.ActiveView == ViewLibrary {
			m.Busy = true
			m.Status = "scanning…"
			return m, scanCmd(m.ctx, m.migration, app.ScanRequest{All: true, DryRun: true})
		}
	case "A":
		if m.ActiveView == ViewStatus {
			rows := m.rows()
			if m.Cursor < len(rows) {
				for _, item := range m.Snapshot.Status.Items {
					if isUnmanaged(item) && (item.Name == rows[m.Cursor].Name || item.Path == rows[m.Cursor].Source) {
						m.Busy = true
						return m, scanCmd(m.ctx, m.migration, app.ScanRequest{
							Agent: item.Scope.Agent, Project: item.Scope.Project, Targets: []string{item.Path}, DryRun: true,
						})
					}
				}
			}
		}
	case "s":
		if m.ActiveView == ViewStatus {
			m.Busy = true
			m.Status = "syncing…"
			return m, syncCmd(m.ctx, m.service)
		}
	case "esc", "q":
		if m.Detail {
			m.Detail = false
			return m, nil
		}
		if m.Mode == ModeUpdates || m.Mode == ModeScan {
			m.Mode = ModeTable
			m.Selected = make(map[string]bool)
			m.Status = "cancelled; no changes made"
			return m, nil
		}
		if m.Scope.Level != "" {
			m.back()
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func viewKey(key string) (View, bool) {
	switch key {
	case "1":
		return ViewLibrary, true
	case "2":
		return ViewAgents, true
	case "3":
		return ViewProjects, true
	case "4":
		return ViewPresets, true
	case "5":
		return ViewStatus, true
	default:
		return "", false
	}
}

func (m Model) toggleSelected() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if m.Cursor >= len(rows) {
		return m, nil
	}
	row := rows[m.Cursor]
	if m.Mode == ModeUpdates || m.Mode == ModeScan {
		key := row.selectionKey()
		m.Selected[key] = !m.Selected[key]
		return m, nil
	}
	if m.ActiveView == ViewAgents && m.Scope.Level == "agent-skills" {
		m.Busy = true
		request := app.BindingRequest{SkillID: row.ID, Agent: m.Scope.Agent}
		return m, bindingCmd(m.ctx, m.service, request, !row.Enabled)
	}
	if m.ActiveView == ViewProjects && m.Scope.Level == "project-skills" {
		m.Busy = true
		request := app.BindingRequest{SkillID: row.ID, Project: m.Scope.Project, Agent: m.Scope.Agent}
		return m, bindingCmd(m.ctx, m.service, request, !row.Enabled)
	}
	if m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills" {
		m.Busy = true
		return m, presetCmd(m.ctx, m.service, app.PresetRequest{Name: m.Scope.Preset, Skills: []string{row.ID}, Remove: row.Enabled})
	}
	return m, nil
}

func (m Model) confirmCurrent(action Action) (tea.Model, tea.Cmd) {
	rows := m.rows()
	if m.Cursor >= len(rows) {
		return m, nil
	}
	m.pendingID = rows[m.Cursor].ID
	m.confirm = action
	m.Mode = ModeConfirm
	m.Status = "press enter to confirm or esc to cancel"
	return m, nil
}

func (m Model) enter() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if m.Mode == ModeUpdates || m.Mode == ModeScan {
		sourceMode := m.Mode
		if len(m.selectedIDs()) == 0 && m.Cursor < len(rows) {
			m.Selected[rows[m.Cursor].selectionKey()] = true
		}
		if len(m.selectedIDs()) == 0 {
			m.Status = "nothing selected"
			return m, nil
		}
		m.Mode = ModeConfirm
		if sourceMode == ModeUpdates {
			m.confirm = ActionUpdate
		} else {
			m.confirm = ActionScan
		}
		m.Status = "press enter to confirm or esc to cancel"
		return m, nil
	}
	if m.Cursor >= len(rows) {
		return m, nil
	}
	row := rows[m.Cursor]
	switch m.ActiveView {
	case ViewLibrary, ViewStatus:
		m.Detail = true
	case ViewAgents:
		if m.Scope.Level == "" {
			m.Scope = Scope{Agent: row.ID, Level: "agent-skills"}
			m.Cursor = 0
		} else {
			m.Detail = true
		}
	case ViewProjects:
		switch m.Scope.Level {
		case "":
			m.Scope = Scope{Project: row.ID, Level: "project-targets"}
		case "project-targets":
			m.Scope.Level = "project-skills"
			if row.ID == "common" {
				m.Scope.Agent = ""
			} else {
				m.Scope.Agent = row.ID
			}
		default:
			m.Detail = true
		}
		m.Cursor = 0
	case ViewPresets:
		if m.Scope.Level == "" {
			m.Scope = Scope{Preset: row.ID, Level: "preset-skills"}
			m.Cursor = 0
		} else {
			m.Detail = true
		}
	}
	return m, nil
}

func (m *Model) back() {
	switch m.Scope.Level {
	case "project-skills":
		m.Scope.Agent = ""
		m.Scope.Level = "project-targets"
	default:
		m.Scope = Scope{}
	}
	m.Cursor = 0
	m.Detail = false
}

func (m Model) selectedIDs() []string {
	if m.confirm == ActionScan || m.Mode == ModeScan {
		return m.selectedTargets()
	}
	ids := make([]string, 0, len(m.Selected))
	for id, selected := range m.Selected {
		if selected {
			ids = append(ids, id)
		}
	}
	sortStrings(ids)
	return ids
}

func (m Model) selectedTargets() []string {
	targets := make([]string, 0, len(m.Selected))
	for _, item := range m.Scan.Items {
		row := row{ID: item.Skill.ID, Origin: item.Origin, Target: item.Target}
		if m.Selected[row.selectionKey()] {
			targets = append(targets, item.Target)
		}
	}
	sortStrings(targets)
	return targets
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && strings.Compare(values[j], values[j-1]) < 0; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (m Model) updateRequest() app.UpdateRequest {
	expected := make(map[string]app.ExpectedUpdate)
	for _, item := range m.Snapshot.Updates.Results {
		if !m.Selected[item.SkillID] {
			continue
		}
		var ref = findSkillRef(m.Snapshot.Config, item.SkillID)
		expected[item.SkillID] = app.ExpectedUpdate{Ref: ref, Resolved: item.Current, Remote: item.Remote}
	}
	return app.UpdateRequest{SkillIDs: m.selectedIDs(), Expected: expected, Confirmed: true, Refresh: true}
}

func findSkillRef(cfg config.Config, id string) *config.Ref {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == id {
			return skill.Ref
		}
	}
	return nil
}
