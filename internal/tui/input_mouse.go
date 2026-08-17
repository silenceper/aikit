package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

type HitRegions struct {
	Layout        Layout
	Tabs          map[View]Rect
	Rows          []Rect
	Checkboxes    []Rect
	Actions       []Rect
	ActionIndexes []int
	ActionBar     Rect
	Confirm       Rect
	Cancel        Rect
	Back          Rect
	ActionPrev    Rect
	ActionNext    Rect
	actionPrev    int
	actionNext    int
}

func (m Model) hitRegions() HitRegions {
	layout := ComputeLayout(m.Width, m.Height)
	regions := HitRegions{Layout: layout, Tabs: make(map[View]Rect), actionPrev: -1, actionNext: -1}
	if layout.TooShort {
		return regions
	}
	for _, item := range layoutNavigation(layout, topViews, m.ActiveView) {
		regions.Tabs[item.View] = item.Rect
	}
	if layout.Narrow && !layout.Breadcrumb.Empty() {
		regions.Back = Rect{X: layout.Breadcrumb.X, Y: layout.Breadcrumb.Y, Width: min(2, layout.Breadcrumb.Width), Height: 1}
	}
	if !m.Detail {
		rowGeometry := m.visibleRowsLayout(layout)
		for _, rowRect := range rowGeometry.Rects {
			regions.Rows = append(regions.Rows, rowRect)
			if m.selectionRendered() {
				regions.Checkboxes = append(regions.Checkboxes, Rect{X: rowRect.X + min(2, rowRect.Width), Y: rowRect.Y, Width: min(3, max(0, rowRect.Width-2)), Height: 1})
			}
		}
	}
	if m.Mode == ModeMore {
		visible := layoutMoreActions(layout, m.primaryActions(), m.ActionIndex, m.OverlayScroll)
		regions.Actions = append(regions.Actions, visible.Rects...)
		regions.ActionIndexes = append(regions.ActionIndexes, visible.Indexes...)
	} else if actions := m.overlayPanelActions(); len(actions) > 0 {
		panel := layoutOverlayPanel(layout, actions, true, m.ActionIndex)
		regions.ActionBar = panel.Actions
		regions.ActionPrev = translateRect(panel.ActionBar.Previous, panel.Actions.X, panel.Actions.Y)
		regions.ActionNext = translateRect(panel.ActionBar.Next, panel.Actions.X, panel.Actions.Y)
		regions.actionPrev, regions.actionNext = panel.ActionBar.PreviousIndex, panel.ActionBar.NextIndex
		buttons := make([]Rect, 0, len(panel.ActionBar.Buttons))
		for _, button := range panel.ActionBar.Buttons {
			button.X += panel.Actions.X
			button.Y += panel.Actions.Y
			buttons = append(buttons, button)
		}
		if m.Mode == ModeConfirm || m.Mode == ModeInput {
			if len(buttons) > 0 {
				regions.Confirm = buttons[0]
			}
			if len(buttons) > 1 {
				regions.Cancel = buttons[1]
			}
		} else {
			regions.Actions = append(regions.Actions, buttons...)
			regions.ActionIndexes = append(regions.ActionIndexes, panel.ActionBar.ButtonIndexes...)
		}
	} else if layout.List.Height > 1 && len(m.primaryActions()) > 0 {
		actions := m.primaryActions()
		actionArea := layout.CollectionPanel.Actions
		if !layout.DetailPanel.Actions.Empty() {
			actionArea = layout.DetailPanel.Actions
		}
		if actionArea.Empty() {
			actionArea = layout.List
			if !layout.Detail.Empty() {
				actionArea = layout.Detail
			}
			actionArea.Y = actionArea.Bottom() - 1
			actionArea.Height = 1
		}
		actionX, actionY, actionRight := actionArea.X, actionArea.Y, actionArea.Right()
		bar := layoutActionBar(actions, m.Focus == FocusActions, m.ActionIndex, actionRight-actionX)
		regions.ActionBar = Rect{X: actionX, Y: actionY, Width: bar.Bar.Width, Height: bar.Bar.Height}
		regions.ActionPrev = translateRect(bar.Previous, actionX, actionY)
		regions.ActionNext = translateRect(bar.Next, actionX, actionY)
		regions.actionPrev, regions.actionNext = bar.PreviousIndex, bar.NextIndex
		for _, button := range bar.Buttons {
			button.X += actionX
			button.Y += actionY
			regions.Actions = append(regions.Actions, button)
		}
		regions.ActionIndexes = append(regions.ActionIndexes, bar.ButtonIndexes...)
	}
	return regions
}

func translateRect(rect Rect, x, y int) Rect {
	if rect.Empty() {
		return Rect{}
	}
	rect.X += x
	rect.Y += y
	return rect
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.MutationBusy || m.Busy {
		return m, nil
	}
	regions := m.hitRegions()
	if m.hasOverlay() {
		if (m.Mode == ModeConfiguration || m.Mode == ModeErrorDetail) && regions.ActionBar.Contains(msg.X, msg.Y) {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.moveActionViewport(-1)
				return m, nil
			case tea.MouseButtonWheelDown:
				m.moveActionViewport(1)
				return m, nil
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if regions.ActionPrev.Contains(msg.X, msg.Y) && regions.actionPrev >= 0 {
				m.Focus, m.ActionIndex = FocusActions, regions.actionPrev
				return m, nil
			}
			if regions.ActionNext.Contains(msg.X, msg.Y) && regions.actionNext >= 0 {
				m.Focus, m.ActionIndex = FocusActions, regions.actionNext
				return m, nil
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && (m.Mode == ModeConfiguration || m.Mode == ModeErrorDetail) {
			for i, rect := range regions.Actions {
				if rect.Contains(msg.X, msg.Y) {
					m.Focus, m.ActionIndex = FocusActions, regions.ActionIndexes[i]
					return m.performPrimaryAction(m.ActionIndex)
				}
			}
		}
		if (m.Mode == ModeConfirm || m.Mode == ModeErrorDetail || m.Help) && msg.Button == tea.MouseButtonWheelUp {
			m.moveOverlayScroll(-1)
			return m, nil
		}
		if (m.Mode == ModeConfirm || m.Mode == ModeErrorDetail || m.Help) && msg.Button == tea.MouseButtonWheelDown {
			m.moveOverlayScroll(1)
			return m, nil
		}
		if m.Mode == ModeMore && msg.Button == tea.MouseButtonWheelUp {
			m.moveMoreAction(-1)
			return m, nil
		}
		if m.Mode == ModeMore && msg.Button == tea.MouseButtonWheelDown {
			m.moveMoreAction(1)
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.Mode == ModeConfirm {
			if regions.Confirm.Contains(msg.X, msg.Y) {
				m.Focus, m.ActionIndex = FocusActions, 0
				return m.performPrimaryAction(m.ActionIndex)
			}
			if regions.Cancel.Contains(msg.X, msg.Y) {
				m.Focus, m.ActionIndex = FocusActions, 1
				return m.performPrimaryAction(m.ActionIndex)
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.Mode == ModeInput {
			if regions.Confirm.Contains(msg.X, msg.Y) {
				m.Focus, m.ActionIndex = FocusActions, 0
				return m.performPrimaryAction(m.ActionIndex)
			}
			if regions.Cancel.Contains(msg.X, msg.Y) {
				m.Focus, m.ActionIndex = FocusActions, 1
				return m.performPrimaryAction(m.ActionIndex)
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.Mode == ModeMore {
			for i, rect := range regions.Actions {
				if rect.Contains(msg.X, msg.Y) {
					m.Focus, m.ActionIndex = FocusActions, regions.ActionIndexes[i]
					return m.performPrimaryAction(m.ActionIndex)
				}
			}
		}
		return m, nil
	}
	if regions.ActionBar.Contains(msg.X, msg.Y) {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.moveActionViewport(-1)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.moveActionViewport(1)
			return m, nil
		}
	}
	if m.Detail && msg.Button == tea.MouseButtonWheelUp && regions.Layout.Main.Contains(msg.X, msg.Y) {
		m.Focus = FocusDetail
		m.moveDetailScroll(-1)
		return m, nil
	}
	if m.Detail && msg.Button == tea.MouseButtonWheelDown && regions.Layout.Main.Contains(msg.X, msg.Y) {
		m.Focus = FocusDetail
		m.moveDetailScroll(1)
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp && regions.Layout.List.Contains(msg.X, msg.Y) {
		return m.perform(uiMoveUp)
	}
	if msg.Button == tea.MouseButtonWheelDown && regions.Layout.List.Contains(msg.X, msg.Y) {
		return m.perform(uiMoveDown)
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if regions.ActionPrev.Contains(msg.X, msg.Y) && regions.actionPrev >= 0 {
		m.Focus, m.ActionIndex = FocusActions, regions.actionPrev
		return m, nil
	}
	if regions.ActionNext.Contains(msg.X, msg.Y) && regions.actionNext >= 0 {
		m.Focus, m.ActionIndex = FocusActions, regions.actionNext
		return m, nil
	}
	if regions.Back.Contains(msg.X, msg.Y) {
		return m.perform(uiBack)
	}
	for _, view := range topViews {
		if regions.Tabs[view].Contains(msg.X, msg.Y) {
			m.switchView(view)
			return m, nil
		}
	}
	start := m.visibleRowsLayout(regions.Layout).Start
	for visibleIndex, rect := range regions.Checkboxes {
		if rect.Contains(msg.X, msg.Y) {
			m.Cursor = start + visibleIndex
			m.Focus, m.ActionIndex = FocusList, 0
			return m.perform(uiToggle)
		}
	}
	for visibleIndex, rect := range regions.Rows {
		if rect.Contains(msg.X, msg.Y) {
			nextCursor := start + visibleIndex
			m.Cursor = nextCursor
			m.Focus, m.ActionIndex = FocusList, 0
			m.ensureVisible()
			if m.Mode == ModeScopePicker || m.Mode == ModePresetPicker {
				return m.chooseHighlightedPicker(), nil
			}
			if m.ActiveView == ViewLibrary {
				return m.perform(uiToggle)
			}
			if m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills" {
				return m.perform(uiToggle)
			}
			return m, nil
		}
	}
	for i, rect := range regions.Actions {
		if rect.Contains(msg.X, msg.Y) {
			actionIndex := i
			if i < len(regions.ActionIndexes) {
				actionIndex = regions.ActionIndexes[i]
			}
			m.Focus, m.ActionIndex = FocusActions, actionIndex
			return m.performPrimaryAction(actionIndex)
		}
	}
	if regions.ActionBar.Contains(msg.X, msg.Y) {
		return m, nil
	}
	if m.Detail && regions.Layout.Main.Contains(msg.X, msg.Y) {
		m.Focus = FocusDetail
		return m, nil
	}
	if !regions.Layout.Detail.Empty() && regions.Layout.Detail.Contains(msg.X, msg.Y) {
		m.Focus = FocusDetail
		return m, nil
	}
	return m, nil
}

func (m Model) hasOverlay() bool {
	return m.Help || m.Mode == ModeConfiguration || m.Mode == ModeFilter || m.Mode == ModeConfirm || m.Mode == ModeInput || m.Mode == ModeMore || m.Mode == ModeErrorDetail
}

func (m Model) performPrimaryAction(index int) (tea.Model, tea.Cmd) {
	actions := m.currentActions()
	if index < 0 || index >= len(actions) {
		return m, nil
	}
	if actions[index] == "Cancel" {
		return m.perform(uiCancel)
	}
	if m.Mode == ModeConfirm && actions[index] == "Confirm" {
		return m.perform(uiConfirm)
	}
	if m.Mode == ModeInput && actions[index] == "Apply" {
		return m.submitInput()
	}
	if (m.Mode == ModeScopePicker || m.Mode == ModePresetPicker) && actions[index] == "Apply" {
		return m.applyPicker()
	}
	switch actions[index] {
	case "Validate":
		m.Busy, m.Status = true, "Validating configuration without changes..."
		return m, validateConfigurationCmd(m.ctx, m.service)
	case "Reload":
		m.Busy, m.Status = true, "Reloading configuration paths..."
		return m, reloadConfigurationCmd(m.ctx, m.service)
	case "Show paths":
		m.enterErrorDetail("Config: " + m.Config.Config + "\nLibrary: " + m.Config.Library + "\nCache: " + m.Config.Cache)
		m.Status = "Configuration paths shown; no clipboard operation was performed"
		return m, nil
	case "View SKILL.md":
		if m.SkillDetail.SkillMD == "" {
			m.Err = "SKILL.md content is not loaded"
			return m, nil
		}
		content := m.SkillDetail.SkillMD
		if m.SkillDetail.SkillMDTruncated {
			content += "\n\n(preview truncated at 64 KiB)"
		}
		m.enterTextDetail("SKILL.md · "+m.SkillDetail.Skill.Name, content)
		m.Status = "Viewing the loaded SKILL.md preview"
		return m, nil
	case "State filter":
		switch m.LibraryStateFilter {
		case LibraryStateAll:
			m.LibraryStateFilter = LibraryStateUpdateAvailable
		case LibraryStateUpdateAvailable:
			m.LibraryStateFilter = LibraryStateManaged
		default:
			m.LibraryStateFilter = LibraryStateAll
		}
		m.Cursor, m.Scroll = 0, 0
		m.Status = "Library state filter: " + string(m.LibraryStateFilter)
		return m, nil
	case "Source filter":
		switch m.LibrarySourceFilter {
		case LibrarySourceAll:
			m.LibrarySourceFilter = LibrarySourceGit
		case LibrarySourceGit:
			m.LibrarySourceFilter = LibrarySourceLocal
		default:
			m.LibrarySourceFilter = LibrarySourceAll
		}
		m.Cursor, m.Scroll = 0, 0
		m.Status = "Library source filter: " + string(m.LibrarySourceFilter)
		return m, nil
	case "Clear selection":
		m.Selected = make(map[string]bool)
		m.Status = "Library selection cleared"
		return m, nil
	case "Check updates":
		m.Busy, m.Status = true, "Checking remote updates..."
		return m, updateCheckCmd(m.ctx, m.service)
	case "Change ref":
		if len(m.selectedLibraryIDs()) != 1 {
			m.Err = "change ref requires exactly one selected library skill"
			return m, nil
		}
		m.enterInput(inputState{Kind: inputRefChange, Prompt: "New ref (branch:name, tag:name, or commit:object-id)"})
		m.Status = "Enter the explicit replacement ref"
		return m, nil
	case "Enable selected", "Disable selected", "Update selected", "Remove selected":
		operation := app.BatchEnable
		switch actions[index] {
		case "Disable selected":
			operation = app.BatchDisable
		case "Update selected":
			operation = app.BatchUpdate
		case "Remove selected":
			operation = app.BatchRemove
		}
		if m.Mode == ModeWorkspaceSkills {
			return m.previewWorkspaceSkills(operation)
		}
		if operation == app.BatchEnable || operation == app.BatchDisable {
			if len(m.selectedBatchSkillIDs()) == 0 {
				m.Err = "select at least one skill"
				return m, nil
			}
			m.pendingBatch = app.BatchRequest{Operation: operation}
			m.enterScopePicker(pickerBatchScope, "", "", false)
			return m, nil
		}
		request, err := m.libraryBatchRequest(operation)
		if err != nil {
			m.Err = err.Error()
			return m, nil
		}
		m.pendingBatch = request
		m.confirm, m.Busy, m.Status = ActionBatch, true, "Building exact atomic batch preview..."
		return m, batchPreviewCmd(m.ctx, m.service, request)
	case "Open", "Review", "Import", "Link existing":
		return m.perform(uiActivate)
	case "Adopt":
		if m.ActiveView == ViewStatus {
			return m.previewSelectedStatusAdopt()
		}
		return m.perform(uiActivate)
	case "Compare":
		rows := m.rows()
		if m.Cursor < 0 || m.Cursor >= len(rows) {
			return m, nil
		}
		for _, item := range m.migrationItems() {
			if item.Key != rows[m.Cursor].Key {
				continue
			}
			if item.MatchedLibraryID == "" || item.Skill.ID == "" {
				m.Err = "Comparison requires a matched library item"
				return m, nil
			}
			m.Busy, m.Status = true, "Comparing library items..."
			return m, compareCmd(m.ctx, m.service, app.CompareRequest{LeftSkillID: item.MatchedLibraryID, RightSkillID: item.Skill.ID})
		}
		return m, nil
	case "Add source":
		m.enterInput(inputState{Kind: inputAddSource, Prompt: "Local path, owner/repo, Git URL, or skills.sh URL"})
		m.Status = "Enter a local path or remote Git source"
		return m, nil
	case "Create project":
		m.pendingProjectPath = ""
		m.ProjectRegistration = app.ProjectRegistrationPreview{}
		m.enterInput(inputState{Kind: inputProjectCreate, Prompt: "Project directory"})
		m.Status = "Enter an existing project directory; agents will be detected automatically"
		return m, nil
	case "Rename project":
		m.pendingID = m.currentProjectName()
		m.enterInput(inputState{Kind: inputProjectRename, Prompt: "Project name"})
		m.Status = "Enter the new project name"
		return m, nil
	case "Change project directory":
		m.pendingID = m.currentProjectName()
		m.enterInput(inputState{Kind: inputProjectPath, Prompt: "Project directory"})
		m.Status = "Enter the new existing project directory"
		return m, nil
	case "Manage agents":
		return m.openProjectAgents()
	case "Select skills":
		m.enterWorkspaceSkills()
		return m, nil
	case "Manage source":
		m.Detail = true
		m.Focus = FocusDetail
		m.Status = "Review the owning preset or inherited scope in Details"
		return m, nil
	case "Save agents":
		return m.saveProjectAgents()
	case "Remove project":
		return m.previewCurrentProjectRemove()
	case "Apply preset":
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-global" {
			m.enterPresetPicker(pickerGlobalWorkspacePreset, "")
			return m, nil
		}
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-agents" {
			rows := m.rows()
			if m.Cursor < 0 || m.Cursor >= len(rows) {
				m.Err = "select an agent before applying a preset"
				return m, nil
			}
			m.enterPresetPickerForAgent(rows[m.Cursor].ID)
			return m, nil
		}
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "agent-skills" {
			m.enterPresetPicker(pickerAgentPreset, "")
			return m, nil
		}
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "project-targets" {
			rows := m.rows()
			if m.Cursor < 0 || m.Cursor >= len(rows) {
				m.Err = "select a project target before applying a preset"
				return m, nil
			}
			agentName := rows[m.Cursor].ID
			if agentName == "common" {
				agentName = ""
			}
			m.enterPresetPickerForProjectTarget(m.Scope.Project, agentName)
			return m, nil
		}
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "project-skills" {
			m.enterPresetPickerForProjectTarget(m.Scope.Project, m.Scope.Agent)
			return m, nil
		}
		project := m.currentProjectName()
		if project == "" {
			m.Err = "select a project before applying a preset"
			return m, nil
		}
		m.enterPresetPicker(pickerProjectPreset, project)
		return m, nil
	case "More":
		m.enterMore()
		return m, nil
	case "Enable", "Disable":
		return m.perform(uiToggle)
	case "Save":
		m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetEditMembers, Name: m.Scope.Preset, Skills: m.selectedIDs()}
		m.confirm, m.Busy, m.Status = ActionPreset, true, "Building preset member preview..."
		return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
	case "Create":
		m.enterInput(inputState{Kind: inputPresetCreate, Prompt: "Preset name"})
		return m, nil
	case "Create preset":
		m.enterInput(inputState{Kind: inputPresetCreate, Prompt: "Preset name"})
		m.Status = "Create a preset, then apply it to this workspace"
		return m, nil
	case "Duplicate", "Rename", "Apply":
		rows := m.rows()
		if m.Cursor < 0 || m.Cursor >= len(rows) {
			return m, nil
		}
		m.pendingID = rows[m.Cursor].ID
		switch actions[index] {
		case "Duplicate":
			m.enterInput(inputState{Kind: inputPresetDuplicate, Prompt: "Duplicate preset as (new name)"})
		case "Rename":
			m.enterInput(inputState{Kind: inputPresetRename, Prompt: "Rename preset to (new name)"})
		case "Apply":
			m.enterScopePicker(pickerPresetApplyScope, "", m.pendingID, false)
		}
		return m, nil
	case "Delete":
		rows := m.rows()
		if m.Cursor < 0 || m.Cursor >= len(rows) {
			return m, nil
		}
		m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetDelete, Name: rows[m.Cursor].ID}
		m.confirm, m.Busy, m.Status = ActionPreset, true, "Building preset deletion preview..."
		return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
	case "Sync preview":
		m.pendingSync = app.SyncRequest{}
		m.Busy, m.Status = true, "Building sync preview..."
		return m, syncPreviewCmd(m.ctx, m.service, app.SyncRequest{DryRun: true})
	case "Remove":
		m.Mode = ModeTable
		return m.confirmCurrent(ActionRemoveSkill)
	case "Review updates":
		m.Mode = ModeTable
		m.openUpdates()
		return m, nil
	case "Retry":
		m.pendingSync = app.SyncRequest{}
		m.Busy, m.Status = true, "Building retry sync preview..."
		return m, syncPreviewCmd(m.ctx, m.service, app.SyncRequest{DryRun: true})
	case "Error details":
		m.enterErrorDetail(m.selectedErrorDetail())
		m.Status = "Full error shown; no clipboard operation was performed"
		return m, nil
	case "Refresh":
		if m.ActiveView == ViewStatus {
			return m.refreshStatus()
		}
		return m.perform(uiRefresh)
	case "Ignore":
		rows := m.rows()
		if m.Cursor >= 0 && m.Cursor < len(rows) {
			m.Ignored[rows[m.Cursor].Key] = true
			delete(m.Selected, rows[m.Cursor].selectionKey())
			m.clampCursor()
			m.Status = "Ignored for this session"
		}
		return m, nil
	case "Close":
		return m.perform(uiCancel)
	}
	return m, nil
}
