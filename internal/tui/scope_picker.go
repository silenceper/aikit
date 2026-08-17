package tui

import (
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

type pickerPurpose string

const (
	pickerBatchScope             pickerPurpose = "batch-scope"
	pickerPresetApplyScope       pickerPurpose = "preset-apply-scope"
	pickerProjectPreset          pickerPurpose = "project-preset"
	pickerProjectTargetPreset    pickerPurpose = "project-target-preset"
	pickerGlobalWorkspacePreset  pickerPurpose = "global-workspace-preset"
	pickerGlobalPresetApplyScope pickerPurpose = "global-preset-apply-scope"
	pickerAgentPreset            pickerPurpose = "agent-preset"
)

type pickerChoice struct {
	Label    string
	Bindings []app.BindingRequest
	Preset   string
}

type pickerState struct {
	Purpose  pickerPurpose
	Choices  []pickerChoice
	Selected int
	Project  string
	Agent    string
	Preset   string
}

func (m *Model) enterScopePicker(purpose pickerPurpose, project, preset string, preserveReturn bool) {
	if !preserveReturn {
		m.captureConfirmReturn()
	}
	choices := m.scopeChoices(project)
	if purpose == pickerGlobalPresetApplyScope {
		choices = globalScopeChoices()
	}
	m.Picker = pickerState{Purpose: purpose, Project: project, Preset: preset, Choices: choices, Selected: -1}
	m.Mode, m.Focus, m.ActionIndex, m.Cursor, m.Scroll, m.OverlayScroll = ModeScopePicker, FocusList, 0, 0, 0, 0
	m.Status = "Choose an exact target scope"
}

func (m *Model) enterPresetPicker(purpose pickerPurpose, project string) {
	m.captureConfirmReturn()
	m.enterPresetPickerWithReturn(purpose, project, m.Scope.Agent)
}

func (m *Model) enterPresetPickerForAgent(agentName string) {
	m.captureConfirmReturn()
	m.enterPresetPickerWithReturn(pickerAgentPreset, "", agentName)
}

func (m *Model) enterPresetPickerForProjectTarget(project, agentName string) {
	m.captureConfirmReturn()
	m.enterPresetPickerWithReturn(pickerProjectTargetPreset, project, agentName)
}

func (m *Model) enterPresetPickerWithReturn(purpose pickerPurpose, project, agentName string) {
	names := make([]string, 0, len(m.Snapshot.Config.Presets))
	for _, preset := range m.Snapshot.Config.Presets {
		names = append(names, preset.Name)
	}
	sort.Strings(names)
	choices := make([]pickerChoice, 0, len(names))
	for _, name := range names {
		choices = append(choices, pickerChoice{Label: name, Preset: name})
	}
	m.Picker = pickerState{Purpose: purpose, Project: project, Agent: agentName, Choices: choices, Selected: -1}
	m.Mode, m.Focus, m.ActionIndex, m.Cursor, m.Scroll, m.OverlayScroll = ModePresetPicker, FocusList, 0, 0, 0, 0
	m.Status = "Choose a preset"
}

func (m Model) scopeChoices(projectName string) []pickerChoice {
	if projectName != "" {
		project, ok := findProject(m.Snapshot.Config.Projects, projectName)
		if !ok {
			return nil
		}
		choices := []pickerChoice{{Label: "Project / " + project.Name + " / Common", Bindings: []app.BindingRequest{{Project: project.Name}}}}
		for _, name := range project.Agents {
			choices = append(choices, pickerChoice{Label: "Project / " + project.Name + " / " + name, Bindings: []app.BindingRequest{{Project: project.Name, Agent: name}}})
		}
		return choices
	}
	choices := globalScopeChoices()
	projects := append([]configProject(nil), projectNames(m.Snapshot.Config.Projects)...)
	sort.SliceStable(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	for _, project := range projects {
		choices = append(choices, pickerChoice{Label: "Project / " + project.Name + " / Common", Bindings: []app.BindingRequest{{Project: project.Name}}})
		for _, name := range project.Agents {
			choices = append(choices, pickerChoice{Label: "Project / " + project.Name + " / " + name, Bindings: []app.BindingRequest{{Project: project.Name, Agent: name}}})
		}
	}
	return choices
}

func globalScopeChoices() []pickerChoice {
	all := make([]app.BindingRequest, 0, len(agent.Names()))
	for _, name := range agent.Names() {
		all = append(all, app.BindingRequest{Agent: name})
	}
	choices := []pickerChoice{{Label: "All agents", Bindings: all}}
	for _, name := range agent.Names() {
		choices = append(choices, pickerChoice{Label: "Global / " + name, Bindings: []app.BindingRequest{{Agent: name}}})
	}
	return choices
}

type configProject struct {
	Name   string
	Agents []string
}

func projectNames(projects []config.Project) []configProject {
	result := make([]configProject, 0, len(projects))
	for _, project := range projects {
		result = append(result, configProject{Name: project.Name, Agents: append([]string(nil), project.Agents...)})
	}
	return result
}

func (m Model) chooseHighlightedPicker() Model {
	if m.Cursor < 0 || m.Cursor >= len(m.Picker.Choices) {
		return m
	}
	m.Picker.Selected = m.Cursor
	m.Status = "Selected " + m.Picker.Choices[m.Cursor].Label + "; choose Apply to review"
	return m
}

func (m Model) applyPicker() (tea.Model, tea.Cmd) {
	if m.Picker.Selected < 0 || m.Picker.Selected >= len(m.Picker.Choices) {
		m.Status = "Choose a target before applying"
		return m, nil
	}
	choice := m.Picker.Choices[m.Picker.Selected]
	if m.Mode == ModePresetPicker {
		switch m.Picker.Purpose {
		case pickerProjectPreset:
			m.enterScopePicker(pickerPresetApplyScope, m.Picker.Project, choice.Preset, true)
			return m, nil
		case pickerGlobalWorkspacePreset:
			m.enterScopePicker(pickerGlobalPresetApplyScope, "", choice.Preset, true)
			return m, nil
		case pickerAgentPreset:
			binding := app.BindingRequest{Preset: choice.Preset, Agent: m.Picker.Agent}
			m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetApply, Name: choice.Preset, Binding: binding}
			m.confirm, m.confirmReturnReady = ActionPreset, true
			m.Busy, m.Status = true, "Building exact agent preset preview..."
			return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
		case pickerProjectTargetPreset:
			binding := app.BindingRequest{Preset: choice.Preset, Project: m.Picker.Project, Agent: m.Picker.Agent}
			m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetApply, Name: choice.Preset, Binding: binding}
			m.confirm, m.confirmReturnReady = ActionPreset, true
			m.Busy, m.Status = true, "Building exact project preset preview..."
			return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
		}
	}
	switch m.Picker.Purpose {
	case pickerBatchScope:
		ids := m.selectedBatchSkillIDs()
		m.pendingBatch.Bindings = nil
		for _, binding := range choice.Bindings {
			for _, skillID := range ids {
				item := binding
				item.SkillID = skillID
				m.pendingBatch.Bindings = append(m.pendingBatch.Bindings, item)
			}
		}
		m.confirm = ActionBatch
		m.confirmReturnReady = true
		m.Busy, m.Status = true, "Building atomic batch preview..."
		return m, batchPreviewCmd(m.ctx, m.service, m.pendingBatch)
	case pickerPresetApplyScope, pickerGlobalPresetApplyScope:
		if len(choice.Bindings) == 1 {
			binding := choice.Bindings[0]
			binding.Preset = m.Picker.Preset
			m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetApply, Name: m.Picker.Preset, Binding: binding}
			m.confirm, m.confirmReturnReady = ActionPreset, true
			m.Busy, m.Status = true, "Building exact preset apply preview..."
			return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
		}
		m.pendingBatch = app.BatchRequest{Operation: app.BatchEnable}
		for _, binding := range choice.Bindings {
			binding.Preset = m.Picker.Preset
			m.pendingBatch.Bindings = append(m.pendingBatch.Bindings, binding)
		}
		m.confirm, m.confirmReturnReady = ActionBatch, true
		m.Busy, m.Status = true, fmt.Sprintf("Building atomic preset preview for %d agents...", len(choice.Bindings))
		return m, batchPreviewCmd(m.ctx, m.service, m.pendingBatch)
	}
	return m, nil
}
