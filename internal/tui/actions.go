package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type uiAction int

const (
	uiNone uiAction = iota
	uiMoveUp
	uiMoveDown
	uiToggle
	uiActivate
	uiCancel
	uiConfirm
	uiRefresh
	uiBack
	uiAddSource
	uiCreateProject
	uiCreatePreset
	uiReviewRecovery
)

func (m Model) perform(action uiAction) (tea.Model, tea.Cmd) {
	if m.MutationBusy || m.Busy {
		return m, nil
	}
	switch action {
	case uiMoveUp:
		if m.Cursor > 0 {
			m.Cursor--
		}
		m.ensureVisible()
	case uiMoveDown:
		if m.Cursor+1 < len(m.rows()) {
			m.Cursor++
		}
		m.ensureVisible()
	case uiToggle:
		return m.toggleSelected()
	case uiActivate:
		return m.activate()
	case uiCancel:
		return m.cancel()
	case uiConfirm:
		return m.confirmAction()
	case uiRefresh:
		return m.startInventory()
	case uiBack:
		if m.Detail || m.Scope.Level != "" {
			m.back()
		} else if m.ActiveView != ViewOverview {
			m.switchView(ViewOverview)
		}
	case uiAddSource:
		m.switchView(ViewLibrary)
		m.enterInput(inputState{Kind: inputAddSource, Prompt: "Local path, owner/repo, Git URL, or skills.sh URL"})
		m.Status = "Enter a local path or remote Git source"
	case uiCreateProject:
		m.switchView(ViewWorkspaces)
		m.enterInput(inputState{Kind: inputProjectCreate, Prompt: "Project directory"})
		m.Status = "Enter the project directory"
	case uiCreatePreset:
		m.switchView(ViewPresets)
		m.enterInput(inputState{Kind: inputPresetCreate, Prompt: "Preset name"})
		m.Status = "Enter the new preset name"
	case uiReviewRecovery:
		operations := make([]app.RecoveryOperation, 0, len(m.Snapshot.Config.PendingOperations))
		for _, operation := range m.Snapshot.Config.PendingOperations {
			operations = append(operations, app.RecoveryOperation{Operation: operation})
		}
		if len(operations) > 0 {
			return m.openRecoveryPreview(operations)
		}
	}
	return m, nil
}

func (m Model) refreshStatus() (tea.Model, tea.Cmd) {
	m.Mode, m.Busy, m.Status = ModeTable, true, "Refreshing status and update checks..."
	return m, refreshSnapshotCmd(m.ctx, m.service)
}

func (m Model) toggleSelected() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return m, nil
	}
	current := rows[m.Cursor]
	if m.Mode == ModeProjectAgents {
		key := current.selectionKey()
		m.Selected[key] = !m.Selected[key]
		return m, nil
	}
	if m.Mode == ModeWorkspaceSkills {
		key := current.selectionKey()
		m.Selected[key] = !m.Selected[key]
		m.Status = fmt.Sprintf("%d Library skill(s) selected", len(m.selectedWorkspacePickerIDs()))
		return m, nil
	}
	if m.Mode == ModeScopePicker || m.Mode == ModePresetPicker {
		return m.chooseHighlightedPicker(), nil
	}
	if m.ActiveView == ViewMigration || m.Mode == ModeScan || m.Mode == ModeUpdates || m.Mode == ModeAddSelect {
		key := current.selectionKey()
		m.Selected[key] = !m.Selected[key]
		return m, nil
	}
	if m.ActiveView == ViewLibrary {
		key := current.selectionKey()
		m.Selected[key] = !m.Selected[key]
		m.Status = fmt.Sprintf("%d library skill(s) selected", m.librarySelectionCount())
		return m, nil
	}
	if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-global" {
		key := current.selectionKey()
		m.Selected[key] = !m.Selected[key]
		m.Status = fmt.Sprintf("%d global workspace skill(s) selected", len(m.selectedWorkspaceSkillIDs()))
		return m, nil
	}
	if m.ActiveView == ViewWorkspaces && m.Scope.Level == "agent-skills" {
		if current.Enabled && !current.Direct {
			m.Detail = true
			m.Status = "This skill is provided by " + firstNonEmpty(current.Detail, current.State) + "; manage that source to remove it"
			return m, nil
		}
		request := app.BindingPreviewRequest{Binding: app.BindingRequest{SkillID: current.ID, Agent: m.Scope.Agent}, Enable: !current.Direct}
		m.pendingBinding, m.confirm = request, ActionBinding
		m.Busy, m.Status = true, "Building binding preview..."
		return m, bindingPreviewCmd(m.ctx, m.service, request)
	}
	if m.ActiveView == ViewWorkspaces && m.Scope.Level == "project-skills" {
		if current.Enabled && !current.Direct {
			m.Detail = true
			m.Status = "This skill is inherited from " + firstNonEmpty(current.Detail, current.State) + "; manage that source to remove it"
			return m, nil
		}
		request := app.BindingPreviewRequest{Binding: app.BindingRequest{SkillID: current.ID, Project: m.Scope.Project, Agent: m.Scope.Agent}, Enable: !current.Direct}
		m.pendingBinding, m.confirm = request, ActionBinding
		m.Busy, m.Status = true, "Building binding preview..."
		return m, bindingPreviewCmd(m.ctx, m.service, request)
	}
	if m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills" {
		m.Selected[current.ID] = !m.Selected[current.ID]
		m.Status = "Preset members changed locally; choose Save to preview"
		return m, nil
	}
	return m, nil
}

func (m Model) activate() (tea.Model, tea.Cmd) {
	if m.Mode == ModeConfirm {
		return m.confirmAction()
	}
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return m, nil
	}
	current := rows[m.Cursor]
	if m.Mode == ModeProjectAgents {
		return m.toggleSelected()
	}
	if m.Mode == ModeWorkspaceSkills {
		return m.toggleSelected()
	}
	if m.Mode == ModeUpdates || m.Mode == ModeScan {
		sourceMode := m.Mode
		if !m.Selected[current.selectionKey()] {
			m.Selected[current.selectionKey()] = true
		}
		m.enterConfirm(ActionScan)
		if m.filterParent == ModeUpdates || sourceMode == ModeUpdates {
			m.confirm = ActionUpdate
		}
		m.Status = "Review the exact change, then confirm"
		return m, nil
	}
	if m.Mode == ModeScopePicker || m.Mode == ModePresetPicker {
		m = m.chooseHighlightedPicker()
		m.Focus, m.ActionIndex = FocusActions, 0
		return m, nil
	}
	if m.Mode == ModeAddSelect {
		if len(m.selectedIDs()) == 0 {
			m.Selected[current.selectionKey()] = true
		}
		m.pendingAdd.Skills = m.selectedIDs()
		m.pendingAdd.ExpectedCandidates = m.selectedAddCandidates()
		m.enterConfirm(ActionAdd)
		m.Preview = app.MutationPreview{Title: "Add selected skills", Summary: fmt.Sprintf("Add %d selected skill(s) from %s", len(m.pendingAdd.Skills), m.pendingAdd.Source), RequiresConfirmation: true}
		m.Status = "Review selected skills, then confirm"
		return m, nil
	}
	if m.Mode == ModeProjectAgents {
		if m.restoreConfirmReturn() {
			m.Status = "Cancelled; no changes made"
		} else {
			m.Mode = ModeTable
		}
		m.Selected = make(map[string]bool)
		return m, nil
	}
	if m.ActiveView == ViewMigration {
		m.prepareConfirmReturn()
		for _, item := range m.migrationItems() {
			if item.Action != m.scanActionForKey(current.Key) {
				delete(m.Selected, item.Key)
			}
		}
		m.Selected[current.selectionKey()] = true
		request := app.ScanRequest{Selectors: m.selectedSelectors(), Targets: m.selectedTargets(), DryRun: true}
		request.Adopt = m.migrationActionAdopts(current)
		m.pendingScan = request
		m.pendingScan.DryRun = false
		m.confirm = ActionScan
		m.Busy, m.Status = true, "Building exact migration preview..."
		return m, migrationPreviewCmd(m.ctx, m.migration, request)
	}
	switch m.ActiveView {
	case ViewOverview:
		return m.openAttention(current)
	case ViewLibrary:
		m.Detail, m.DetailScroll, m.pendingDetailID, m.Status = true, 0, current.ID, "Loading skill detail..."
		return m, skillDetailCmd(m.ctx, m.service, current.ID)
	case ViewStatus:
		m.Detail = true
	case ViewWorkspaces:
		switch m.Scope.Level {
		case "":
			if current.ID == "agents" {
				m.Scope.Level = "workspace-agents"
			} else if current.ID == "projects" {
				m.Scope.Level = "workspace-projects"
			} else if current.ID == "global" {
				m.Scope = Scope{Level: "workspace-global"}
			} else {
				m.Detail = true
			}
		case "workspace-agents":
			m.Scope = Scope{Agent: current.ID, Level: "agent-skills"}
		case "workspace-projects":
			m.Scope = Scope{Project: current.ID, Level: "project-targets"}
		case "project-targets":
			m.Scope.Level = "project-skills"
			if current.ID == "common" {
				m.Scope.Agent = ""
			} else {
				m.Scope.Agent = current.ID
			}
		default:
			m.Detail = true
		}
		m.Cursor, m.Scroll = 0, 0
	case ViewPresets:
		if m.Scope.Level == "" {
			m.Scope = Scope{Preset: current.ID, Level: "preset-skills"}
			m.Cursor, m.Scroll = 0, 0
			m.Selected = make(map[string]bool)
			if preset, ok := findPreset(m.Snapshot.Config.Presets, current.ID); ok {
				for _, skillID := range preset.Skills {
					m.Selected[skillID] = true
				}
			}
		} else {
			m.Detail = true
		}
	}
	return m, nil
}

func (m Model) openAttention(current row) (tea.Model, tea.Cmd) {
	if current.DestinationAction == ActionRecovery {
		for _, operation := range m.Snapshot.Config.PendingOperations {
			if operation.ID == current.ID {
				return m.openRecoveryPreview([]app.RecoveryOperation{{Operation: operation}})
			}
		}
	}
	if current.DestinationMode == ModeUpdates {
		m.openUpdates()
		m.restoreActiveKey(current.DestinationKey)
		return m, nil
	}
	if current.DestinationView != "" {
		m.switchView(current.DestinationView)
		m.restoreActiveKey(current.DestinationKey)
	}
	return m, nil
}

func (m Model) scanActionForKey(key string) app.ScanAction {
	for _, item := range m.migrationItems() {
		if item.Key == key {
			return item.Action
		}
	}
	return app.ScanActionNone
}

func (m Model) migrationCanCompare(key string) bool {
	for _, item := range m.migrationItems() {
		if item.Key == key {
			return item.MatchedLibraryID != "" && item.Skill.ID != ""
		}
	}
	return false
}

func (m Model) previewSelectedStatusAdopt() (tea.Model, tea.Cmd) {
	item, ok := m.selectedStatusItem()
	if !ok || !isUnmanaged(item) {
		return m, nil
	}
	m.Busy, m.Status = true, "Building exact adoption preview..."
	return m, scanCmd(m.ctx, m.migration, app.ScanRequest{Agent: item.Scope.Agent, Project: item.Scope.Project, Targets: []string{item.Path}, DryRun: true})
}

func (m Model) cancel() (tea.Model, tea.Cmd) {
	if m.Mode == ModeWorkspaceSkills {
		if !m.restoreConfirmReturn() {
			m.Mode, m.Focus = ModeTable, FocusList
		}
		m.Status = "Cancelled; no changes made"
		return m, nil
	}
	if m.Mode == ModeScopePicker || m.Mode == ModePresetPicker {
		if !m.restoreConfirmReturn() {
			m.Mode, m.Focus = ModeTable, FocusList
		}
		m.Picker = pickerState{}
		m.pendingBatch = app.BatchRequest{}
		m.pendingPreset = app.PresetMutationRequest{}
		m.Status = "Cancelled; no changes made"
		return m, nil
	}
	if m.Help {
		m.Help = false
		return m, nil
	}
	if m.Mode == ModeConfiguration {
		m.Mode = ModeTable
		return m, nil
	}
	if m.Mode == ModeInput {
		m.Mode = ModeTable
		m.Input = inputState{}
		m.pendingBatch = app.BatchRequest{}
		m.pendingProject = app.ProjectEditRequest{}
		m.pendingProjectPath = ""
		m.ProjectRegistration = app.ProjectRegistrationPreview{}
		m.pendingPreset = app.PresetMutationRequest{}
		m.Status = "Cancelled; no changes made"
		return m, nil
	}
	if m.Mode == ModeAddSelect {
		m.Mode = ModeTable
		m.Input = inputState{}
		m.AddPreview = app.AddPreview{}
		m.Selected = make(map[string]bool)
		m.Status = "Cancelled; no changes made"
		return m, nil
	}
	if m.Mode == ModeMore {
		m.Mode, m.ActionIndex = ModeTable, 0
		if m.Detail {
			m.Focus = FocusDetail
		} else {
			m.Focus = FocusList
		}
		return m, nil
	}
	if m.Mode == ModeErrorDetail {
		if state := m.errorDetailReturn; state.Valid {
			m.ActiveView, m.Mode, m.Focus = state.ActiveView, state.Mode, state.Focus
			m.Status = state.Status
			m.ActionIndex, m.Cursor, m.Scroll = state.ActionIndex, state.Cursor, state.Scroll
			m.DetailScroll, m.OverlayScroll = state.DetailScroll, state.OverlayScroll
			m.FullError, m.FullDetailTitle, m.errorDetailParent = "", "", ""
			m.errorDetailReturn = confirmReturnState{}
			return m, nil
		}
		parent := m.errorDetailParent
		if parent == "" || parent == ModeErrorDetail {
			parent = ModeTable
		}
		m.Mode, m.FullError, m.FullDetailTitle, m.errorDetailParent = parent, "", "", ""
		return m, nil
	}
	if m.Mode == ModeFilter {
		m.cancelFilterDraft()
		return m, nil
	}
	if m.Mode == ModeConfirm {
		restored := m.restoreConfirmReturn()
		if !restored && m.confirm == ActionUpdate {
			m.Mode = ModeUpdates
		} else if !restored && m.confirm == ActionScan && len(m.Scan.Items) > 0 {
			m.Mode = ModeScan
		} else if !restored {
			m.Mode = ModeTable
		}
		m.confirm, m.pendingID = ActionNone, ""
		m.Preview = app.MutationPreview{}
		m.PlanPreview = app.Result{}
		if !restored {
			m.OverlayScroll = 0
		}
		m.forceAcknowledged = false
		m.pendingBatch = app.BatchRequest{}
		m.pendingUpdate = app.UpdateRequest{}
		m.pendingProject = app.ProjectEditRequest{}
		m.ProjectPreview = app.ProjectEditPreview{}
		m.pendingProjectPath = ""
		m.ProjectRegistration = app.ProjectRegistrationPreview{}
		m.pendingPreset = app.PresetMutationRequest{}
		m.Status = "Cancelled; no changes made"
		return m, nil
	}
	if m.Detail {
		m.Detail, m.DetailScroll = false, 0
		m.pendingDetailID = ""
		return m, nil
	}
	if m.Mode == ModeUpdates || m.Mode == ModeScan {
		m.Mode = ModeTable
		m.Selected = make(map[string]bool)
		m.Status = "Cancelled; no changes made"
		return m, nil
	}
	if m.Scope.Level != "" {
		m.back()
		return m, nil
	}
	return m, nil
}

func (m Model) confirmAction() (tea.Model, tea.Cmd) {
	if m.Mode != ModeConfirm {
		return m, nil
	}
	m.Busy, m.MutationBusy = true, true
	switch m.confirm {
	case ActionUpdate:
		m.Status = "Updating selected skills..."
		return m, updateCmd(m.ctx, m.service, m.updateRequest())
	case ActionScan:
		m.Status = "Applying selected migration action..."
		request := m.pendingScan
		if len(request.Selectors) == 0 {
			request = app.ScanRequest{Adopt: true, Selectors: m.selectedSelectors(), Targets: m.selectedTargets()}
		}
		request.DryRun = false
		return m, adoptCmd(m.ctx, m.migration, request)
	case ActionBinding:
		m.Status = "Applying binding change..."
		return m, bindingCmd(m.ctx, m.service, m.pendingBinding.Binding, m.pendingBinding.Enable)
	case ActionSync:
		m.Status = "Applying sync plan..."
		return m, syncCmd(m.ctx, m.service, m.pendingSync)
	case ActionDiscoverAdd:
		m.MutationBusy = false
		m.Status = "Discovering remote skills in a temporary checkout..."
		return m, addPreviewCmd(m.ctx, m.service, app.AddPreviewRequest{
			Source: m.pendingAdd.Source, SourcePath: m.pendingAdd.SourcePath,
			Ref: m.pendingAdd.Ref, AllowNetwork: true,
		})
	case ActionAdd:
		m.Status = "Adding selected skills..."
		return m, addCmd(m.ctx, m.service, m.pendingAdd)
	case ActionPreset:
		if m.pendingPreset.Operation == app.PresetDelete && m.Preview.RequiresForce && !m.pendingPreset.Force {
			m.Busy, m.MutationBusy = false, false
			m.enterConfirm(ActionForcePresetDelete)
			m.Status = "Force deletion will detach every listed preset reference; confirm again"
			return m, nil
		}
		m.Status = "Applying preset mutation..."
		m.pendingPreset.Confirmed = true
		return m, mutatePresetCmd(m.ctx, m.service, m.pendingPreset)
	case ActionForcePresetDelete:
		m.Status = "Force deleting preset..."
		m.pendingPreset.Force, m.pendingPreset.Confirmed = true, true
		return m, mutatePresetCmd(m.ctx, m.service, m.pendingPreset)
	case ActionBatch:
		if m.pendingBatch.Operation == app.BatchRemove && m.Preview.RequiresForce && !m.pendingBatch.Force {
			m.Busy, m.MutationBusy = false, false
			m.enterConfirm(ActionForceBatchRemove)
			m.Status = "Force removal will detach every listed reference; confirm again"
			return m, nil
		}
		m.Status = "Applying library batch..."
		m.pendingBatch.Confirmed = true
		return m, batchCmd(m.ctx, m.service, m.pendingBatch)
	case ActionForceBatchRemove:
		m.Status = "Force removing selected skills..."
		m.pendingBatch.Force, m.pendingBatch.Confirmed = true, true
		return m, batchCmd(m.ctx, m.service, m.pendingBatch)
	case ActionRef:
		m.Status = "Changing skill ref..."
		m.pendingUpdate.Confirmed = true
		return m, updateCmd(m.ctx, m.service, m.pendingUpdate)
	case ActionProjectEdit:
		m.Status = "Applying project edit..."
		m.pendingProject.Confirmed = true
		return m, projectEditCmd(m.ctx, m.service, m.pendingProject)
	case ActionRemoveSkill:
		if m.Preview.RequiresForce && !m.forceAcknowledged {
			m.forceAcknowledged = true
			m.Busy, m.MutationBusy = false, false
			m.enterConfirm(ActionForceRemove)
			m.Status = "Force removal will detach references; confirm again"
			return m, nil
		}
		m.Status = "Removing skill..."
		m.pendingRemove.Force = m.forceAcknowledged
		return m, removeSkillCmd(m.ctx, m.service, m.pendingRemove)
	case ActionForceRemove:
		m.Status = "Force removing skill..."
		m.pendingRemove.Force = true
		return m, removeSkillCmd(m.ctx, m.service, m.pendingRemove)
	case ActionRemoveProject:
		m.Status = "Removing project..."
		return m, removeProjectCmd(m.ctx, m.service, app.ProjectRemoveRequest{Project: m.pendingID, Confirmed: true})
	case ActionRecovery:
		for _, operation := range m.RecoveryPreview.Operations {
			if !operation.CanResume {
				m.Busy, m.MutationBusy = false, false
				m.Err, m.Status = "selected recovery group cannot resume", "Recovery cannot resume; review issues"
				return m, nil
			}
		}
		m.Status = "Resuming exact recovery operations..."
		m.pendingRecovery.Confirmed = true
		return m, resumeRecoveryCmd(m.ctx, m.service, m.pendingRecovery)
	default:
		m.Busy, m.MutationBusy = false, false
	}
	return m, nil
}

func (m Model) selectedAddCandidates() []app.ExpectedAddCandidate {
	selected := make([]app.ExpectedAddCandidate, 0, len(m.Selected))
	for _, candidate := range m.AddPreview.Candidates {
		if !m.Selected[candidate.RelativePath] {
			continue
		}
		selected = append(selected, app.ExpectedAddCandidate{
			Name: candidate.Name, RelativePath: candidate.RelativePath, Hash: candidate.Hash,
		})
	}
	return selected
}

func (m Model) confirmCurrent(action Action) (tea.Model, tea.Cmd) {
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return m, nil
	}
	m.pendingID = rows[m.Cursor].ID
	if action == ActionRemoveSkill {
		m.pendingRemove = app.RemoveRequest{SkillID: m.pendingID}
		m.confirm, m.Busy = action, true
		m.Status = "Building remove preview..."
		return m, removePreviewCmd(m.ctx, m.service, m.pendingRemove)
	}
	m.enterConfirm(action)
	m.Status = "Review the exact change, then confirm"
	return m, nil
}

func (m Model) previewCurrentProjectRemove() (tea.Model, tea.Cmd) {
	project := m.currentProjectName()
	if project == "" {
		return m, nil
	}
	m.pendingID, m.confirm = project, ActionRemoveProject
	m.Busy, m.Status = true, "Building exact project removal preview..."
	return m, projectRemovePreviewCmd(m.ctx, m.service, app.ProjectRemoveRequest{Project: m.pendingID})
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.Input.Value)
	if value == "" {
		m.Err = m.Input.Prompt + " is required"
		return m, nil
	}
	switch m.Input.Kind {
	case inputAddSource:
		m.pendingAdd = app.AddRequest{Source: value}
		m.Busy, m.Status = true, "Discovering source without mutation..."
		return m, addPreviewCmd(m.ctx, m.service, app.AddPreviewRequest{Source: value})
	case inputPresetCreate:
		m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetCreate, Name: value}
		m.confirm = ActionPreset
		m.Busy, m.Status = true, "Building preset preview..."
		return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
	case inputPresetDuplicate:
		m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetDuplicate, Name: m.pendingID, NewName: value}
		m.confirm, m.Busy, m.Status = ActionPreset, true, "Building duplicate preset preview..."
		return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
	case inputPresetRename:
		m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetRename, Name: m.pendingID, NewName: value}
		m.confirm, m.Busy, m.Status = ActionPreset, true, "Building rename preset preview..."
		return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
	case inputRefChange:
		kind, refValue, ok := strings.Cut(value, ":")
		if !ok || refValue == "" || (kind != "branch" && kind != "tag" && kind != "commit") {
			m.Err = "ref must be branch:<name>, tag:<name>, or commit:<full-object-id>"
			return m, nil
		}
		ids := m.selectedLibraryIDs()
		if len(ids) != 1 {
			m.Err = "change ref requires exactly one selected library skill"
			return m, nil
		}
		skillID := ids[0]
		m.pendingUpdate = app.UpdateRequest{
			SkillIDs: []string{skillID}, Ref: &config.Ref{Kind: kind, Value: refValue}, Force: true,
			Expected: map[string]app.ExpectedUpdate{skillID: {Ref: findSkillRef(m.Snapshot.Config, skillID), Resolved: findSkillResolved(m.Snapshot.Config, skillID)}},
		}
		m.enterConfirm(ActionRef)
		m.Preview = app.MutationPreview{
			Title: "Change skill ref", Summary: fmt.Sprintf("Change %s to %s:%s; the application will roll back the previous ref and content if the update fails", skillID, kind, refValue), RequiresConfirmation: true,
		}
		m.Status = "Review the ref change and rollback guarantee, then confirm"
		return m, nil
	case inputProjectCreate:
		m.pendingProjectPath = value
		m.Busy, m.Status = true, "Inspecting project directory without mutation..."
		return m, projectRegistrationPreviewCmd(m.ctx, m.service, app.ProjectRegistrationRequest{Path: value})
	case inputProjectName:
		if m.pendingProjectPath == "" {
			m.Err = "project directory is required before choosing a name"
			return m, nil
		}
		m.Busy, m.Status = true, "Rebuilding project registration preview..."
		return m, projectRegistrationPreviewCmd(m.ctx, m.service, app.ProjectRegistrationRequest{Path: m.pendingProjectPath, Name: value})
	case inputProjectRename:
		request := app.ProjectEditRequest{Project: m.pendingID, Name: value}
		m.pendingProject = request
		m.Busy, m.Status = true, "Building project rename preview..."
		return m, projectPreviewCmd(m.ctx, m.service, request)
	case inputProjectPath:
		request := app.ProjectEditRequest{Project: m.pendingID, Path: value}
		m.pendingProject = request
		m.Busy, m.Status = true, "Building project directory change preview..."
		return m, projectPreviewCmd(m.ctx, m.service, request)
	default:
		m.Err = "Unsupported input action"
		return m, nil
	}
}

func (m Model) currentProjectName() string {
	if m.Scope.Project != "" {
		return m.Scope.Project
	}
	rows := m.rows()
	if m.Cursor >= 0 && m.Cursor < len(rows) && m.Scope.Level == "workspace-projects" {
		return rows[m.Cursor].ID
	}
	return ""
}

func (m Model) openProjectAgents() (tea.Model, tea.Cmd) {
	projectName := m.currentProjectName()
	project, ok := findProject(m.Snapshot.Config.Projects, projectName)
	if !ok {
		m.Err = "select a registered project"
		return m, nil
	}
	m.captureConfirmReturn()
	m.pendingID = project.Name
	m.Mode, m.Focus, m.Cursor, m.Scroll = ModeProjectAgents, FocusList, 0, 0
	m.Selected = make(map[string]bool)
	for _, name := range project.Agents {
		m.Selected["project-agent:"+name] = true
	}
	m.Status = "Select the agents enabled for this project"
	return m, nil
}

func (m Model) saveProjectAgents() (tea.Model, tea.Cmd) {
	project, ok := findProject(m.Snapshot.Config.Projects, m.pendingID)
	if !ok {
		m.Err = "selected project no longer exists"
		return m, nil
	}
	selected := make([]string, 0, len(agent.Names()))
	for _, name := range agent.Names() {
		if m.Selected["project-agent:"+name] {
			selected = append(selected, name)
		}
	}
	request := app.ProjectEditRequest{Project: project.Name}
	for _, name := range selected {
		if !contains(project.Agents, name) {
			request.AddAgents = append(request.AddAgents, name)
		}
	}
	for _, name := range project.Agents {
		if !contains(selected, name) {
			request.RemoveAgents = append(request.RemoveAgents, name)
		}
	}
	if len(request.AddAgents)+len(request.RemoveAgents) == 0 {
		m.Status = "Agent selection is unchanged"
		return m, nil
	}
	m.pendingProject = request
	m.confirmReturnReady = true
	m.Busy, m.Status = true, "Building exact project agent preview..."
	return m, projectPreviewCmd(m.ctx, m.service, request)
}

func (m Model) selectedErrorDetail() string {
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return m.Err
	}
	current := rows[m.Cursor]
	if strings.HasPrefix(current.Key, "status:update-failure:") {
		skillID := strings.TrimPrefix(current.Key, "status:update-failure:")
		for _, result := range m.Snapshot.Updates.Results {
			if result.SkillID == skillID && result.State == updatecheck.StateCheckFailed {
				return firstNonEmpty(result.Error, current.Detail, m.Err)
			}
		}
	}
	if strings.HasPrefix(current.Key, "status:item:") {
		for _, item := range m.Snapshot.Status.Items {
			if current.Key == statusItemKey(item) {
				return firstNonEmpty(item.Message, item.Path, m.Err)
			}
		}
	}
	if strings.HasPrefix(current.Key, "inventory-issue:") {
		return firstNonEmpty(current.Detail, current.Source, m.Err)
	}
	return firstNonEmpty(current.Detail, m.Err)
}

func (m Model) migrationActionAdopts(current row) bool {
	for _, item := range m.migrationItems() {
		if item.Key != current.Key {
			continue
		}
		return item.Action == app.ScanActionAdopt || item.Action == app.ScanActionLinkExisting
	}
	return true
}

func (m *Model) back() {
	switch m.Scope.Level {
	case "project-skills":
		m.Scope.Agent, m.Scope.Level = "", "project-targets"
	case "agent-skills":
		m.Scope = Scope{Level: "workspace-agents"}
	case "project-targets":
		m.Scope = Scope{Level: "workspace-projects"}
	default:
		m.Scope = Scope{}
	}
	m.Cursor, m.Scroll = 0, 0
	m.Detail = false
	m.DetailScroll = 0
}

func (m Model) selectedIDs() []string {
	if m.confirm == ActionScan || m.Mode == ModeScan || m.ActiveView == ViewMigration {
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

func (m Model) librarySelectionCount() int {
	count := 0
	for _, skill := range m.Snapshot.Config.Library.Skills {
		if m.Selected["library:"+skill.ID] {
			count++
		}
	}
	return count
}

func (m Model) selectedLibraryIDs() []string {
	ids := make([]string, 0, m.librarySelectionCount())
	for _, skill := range m.Snapshot.Config.Library.Skills {
		if m.Selected["library:"+skill.ID] {
			ids = append(ids, skill.ID)
		}
	}
	sortStrings(ids)
	return ids
}

func (m Model) selectedWorkspaceSkillIDs() []string {
	ids := make([]string, 0)
	for _, skill := range m.Snapshot.Config.Library.Skills {
		if m.Selected["workspace-global:"+skill.ID] {
			ids = append(ids, skill.ID)
		}
	}
	sortStrings(ids)
	return ids
}

func (m Model) selectedBatchSkillIDs() []string {
	if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-global" {
		return m.selectedWorkspaceSkillIDs()
	}
	return m.selectedLibraryIDs()
}

func (m Model) libraryBatchRequest(operation app.BatchOperation) (app.BatchRequest, error) {
	ids := m.selectedLibraryIDs()
	if len(ids) == 0 {
		return app.BatchRequest{}, fmt.Errorf("select at least one library skill")
	}
	request := app.BatchRequest{Operation: operation}
	switch operation {
	case app.BatchEnable, app.BatchDisable:
		return app.BatchRequest{}, fmt.Errorf("enable or disable requires an explicit binding scope")
	case app.BatchUpdate:
		request.SkillIDs = ids
		request.Expected = make(map[string]app.ExpectedUpdate, len(ids))
		for _, skillID := range ids {
			skill, ok := snapshotSkill(m.Snapshot.Config, skillID)
			if !ok || skill.Source == "" || skill.Ref == nil || skill.Ref.Kind != "branch" || skill.Ref.Value == "" {
				return app.BatchRequest{}, fmt.Errorf("skill %q has no updateable source ref", skillID)
			}
			checked, ok := snapshotUpdate(m.Snapshot, skillID)
			if !ok || checked.State != updatecheck.StateUpdateAvailable || checked.Current == "" || checked.Remote == "" || checked.Current != skill.Resolved {
				return app.BatchRequest{}, fmt.Errorf("skill %q does not have a complete current update-available result", skillID)
			}
			request.Expected[skillID] = app.ExpectedUpdate{
				Ref: &config.Ref{Kind: skill.Ref.Kind, Value: skill.Ref.Value}, Resolved: checked.Current, Remote: checked.Remote,
			}
		}
	case app.BatchRemove:
		request.SkillIDs = ids
	default:
		return app.BatchRequest{}, fmt.Errorf("unsupported library batch %q", operation)
	}
	return request, nil
}

func snapshotSkill(cfg config.Config, id string) (config.Skill, bool) {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == id {
			return skill, true
		}
	}
	return config.Skill{}, false
}

func snapshotUpdate(snapshot app.Snapshot, id string) (updatecheck.Result, bool) {
	for _, item := range snapshot.Updates.Results {
		if item.SkillID == id {
			return item, true
		}
	}
	return updatecheck.Result{}, false
}

func findSkillResolved(cfg config.Config, id string) string {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == id {
			return skill.Resolved
		}
	}
	return ""
}

func findRemoteUpdate(snapshot app.Snapshot, id string) string {
	for _, item := range snapshot.Updates.Results {
		if item.SkillID == id {
			return item.Remote
		}
	}
	return ""
}

func (m Model) migrationItems() []app.ScanItem {
	if len(m.Scan.Items) > 0 && (m.Mode == ModeScan || m.filterParent == ModeScan || (m.Mode == ModeConfirm && m.confirm == ActionScan)) {
		return m.Scan.Items
	}
	return m.Inventory.Items
}

func (m Model) selectedTargets() []string {
	var targets []string
	for _, item := range m.migrationItems() {
		key := item.Key
		if key == "" {
			key = row{Origin: item.Origin, Target: item.Target}.selectionKey()
		}
		if m.Selected[key] {
			targets = append(targets, item.Target)
		}
	}
	sortStrings(targets)
	return targets
}

func (m Model) selectedSelectors() []app.ScanSelector {
	var selectors []app.ScanSelector
	for _, item := range m.migrationItems() {
		if !m.Selected[item.Key] || item.Key == "" {
			continue
		}
		selectors = append(selectors, app.ScanSelector{
			Key: item.Key, Origin: item.Origin, Target: item.Target,
			ExpectedHash: item.ContentHash, ExpectedObjectID: item.ObjectID, ExpectedRootID: item.RootObjectID,
			ExpectedState: item.State, ExpectedSkillID: item.Skill.ID, ExpectedLibraryHash: item.MatchedLibraryHash,
		})
	}
	return selectors
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
		expected[item.SkillID] = app.ExpectedUpdate{Ref: findSkillRef(m.Snapshot.Config, item.SkillID), Resolved: item.Current, Remote: item.Remote}
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
