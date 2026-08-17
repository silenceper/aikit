package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

func (m *Model) enterWorkspaceSkills() {
	m.captureConfirmReturn()
	m.Selected = make(map[string]bool)
	m.Mode, m.Focus, m.ActionIndex, m.Cursor, m.Scroll = ModeWorkspaceSkills, FocusList, 0, 0, 0
	m.Status = "Select Library skills for this exact workspace target"
}

func (m Model) selectedWorkspacePickerIDs() []string {
	ids := make([]string, 0)
	for _, skill := range m.Snapshot.Config.Library.Skills {
		if m.Selected["workspace-skill:"+skill.ID] {
			ids = append(ids, skill.ID)
		}
	}
	sortStrings(ids)
	return ids
}

func (m Model) previewWorkspaceSkills(operation app.BatchOperation) (Model, tea.Cmd) {
	ids := m.selectedWorkspacePickerIDs()
	if len(ids) == 0 {
		m.Err = "select at least one Library skill"
		return m, nil
	}
	request := app.BatchRequest{Operation: operation}
	for _, skillID := range ids {
		request.Bindings = append(request.Bindings, app.BindingRequest{SkillID: skillID, Agent: m.Scope.Agent, Project: m.Scope.Project})
	}
	m.pendingBatch, m.confirm, m.confirmReturnReady = request, ActionBatch, true
	m.Busy, m.Status = true, fmt.Sprintf("Building exact %s preview for %d selected skill(s)...", operation, len(ids))
	return m, batchPreviewCmd(m.ctx, m.service, request)
}
