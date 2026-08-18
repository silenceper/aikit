package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.cancelInventory()
		return m, tea.Quit
	}
	if m.MutationBusy {
		if key == "ctrl+q" || key == "q" {
			m.Status = "Operation in progress; wait for completion or use Ctrl+C for emergency exit"
		}
		return m, nil
	}
	if key == "ctrl+q" {
		m.cancelInventory()
		return m, tea.Quit
	}
	if key == "q" && m.Mode != ModeFilter && m.Mode != ModeInput {
		m.Status = "Press Ctrl+Q to quit"
		return m, nil
	}
	if m.readingActivityActive() {
		return m.updateReadingKey(msg)
	}
	if m.Help {
		if key == "esc" || key == "?" {
			return m.perform(uiCancel)
		}
		switch key {
		case "up", "k":
			m.moveOverlayScroll(-1)
		case "down", "j":
			m.moveOverlayScroll(1)
		case "pgup":
			m.moveOverlayScroll(-m.overlayPageSize())
		case "pgdown":
			m.moveOverlayScroll(m.overlayPageSize())
		}
		return m, nil
	}
	if ComputeLayout(m.Width, m.Height).TooShort {
		switch key {
		case "?":
			m.Help = true
		case "esc":
			return m.perform(uiCancel)
		default:
			m.Status = "terminal too short; need 8 rows"
		}
		return m, nil
	}
	if m.Mode == ModeConfiguration {
		if key == "esc" {
			return m.perform(uiCancel)
		}
		switch key {
		case "left", "shift+tab":
			m.Focus = FocusActions
			m.moveAction(-1)
			return m, nil
		case "right", "tab":
			m.Focus = FocusActions
			m.moveAction(1)
			return m, nil
		case "enter":
			return m.performPrimaryAction(m.ActionIndex)
		}
		return m, nil
	}
	if m.Mode == ModeErrorDetail {
		if key == "esc" || key == "enter" {
			return m.perform(uiCancel)
		}
		switch key {
		case "up", "k":
			m.moveOverlayScroll(-1)
			return m, nil
		case "down", "j":
			m.moveOverlayScroll(1)
			return m, nil
		case "pgup":
			m.moveOverlayScroll(-m.overlayPageSize())
			return m, nil
		case "pgdown":
			m.moveOverlayScroll(m.overlayPageSize())
			return m, nil
		}
		return m, nil
	}
	if m.Mode == ModeCommand {
		entries := m.commandEntries()
		switch key {
		case "esc":
			m.Mode, m.CommandDraft, m.CommandIndex = ModeTable, "", 0
			return m, nil
		case "up", "k":
			m.CommandIndex = max(0, m.CommandIndex-1)
			return m, nil
		case "down", "j":
			m.CommandIndex = min(max(0, len(entries)-1), m.CommandIndex+1)
			return m, nil
		case "backspace":
			chars := []rune(m.CommandDraft)
			if len(chars) > 0 {
				m.CommandDraft = string(chars[:len(chars)-1])
			}
			m.CommandIndex = 0
			return m, nil
		case "enter":
			if len(entries) > 0 {
				return m.activateCommandEntry(entries[min(m.CommandIndex, len(entries)-1)])
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.CommandDraft += string(msg.Runes)
				m.CommandIndex = 0
			}
			return m, nil
		}
	}
	if m.Mode == ModeFilter {
		switch key {
		case "esc":
			m.cancelFilterDraft()
			return m, nil
		case "enter":
			m.applyFilterDraft()
			return m, nil
		case "backspace":
			chars := []rune(m.FilterDraft)
			if len(chars) > 0 {
				m.FilterDraft = string(chars[:len(chars)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.FilterDraft += string(msg.Runes)
			}
		}
		m.Cursor, m.Scroll = 0, 0
		m.clampCursor()
		return m, nil
	}
	if m.Mode == ModeInput {
		switch key {
		case "esc":
			return m.perform(uiCancel)
		case "enter":
			return m.performPrimaryAction(m.ActionIndex)
		case "left", "shift+tab":
			m.moveAction(-1)
			return m, nil
		case "right", "tab":
			m.moveAction(1)
			return m, nil
		case "backspace":
			chars := []rune(m.Input.Value)
			if len(chars) > 0 {
				m.Input.Value = string(chars[:len(chars)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.Input.Value += string(msg.Runes)
			}
		}
		return m, nil
	}
	if m.Mode == ModeMore {
		if key == "esc" {
			return m.perform(uiCancel)
		}
		switch key {
		case "left", "shift+tab", "up", "k":
			m.moveMoreAction(-1)
			return m, nil
		case "right", "tab", "down", "j":
			m.moveMoreAction(1)
			return m, nil
		case "pgup":
			m.moveMoreAction(-max(1, ComputeLayout(m.Width, m.Height).Overlay.Height-1))
			return m, nil
		case "pgdown":
			m.moveMoreAction(max(1, ComputeLayout(m.Width, m.Height).Overlay.Height-1))
			return m, nil
		case "enter":
			return m.performPrimaryAction(m.ActionIndex)
		}
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			visible := layoutMoreActions(ComputeLayout(m.Width, m.Height), m.primaryActions(), m.ActionIndex, m.OverlayScroll)
			visibleIndex := int(key[0] - '1')
			if visibleIndex < len(visible.Indexes) {
				m.ActionIndex = visible.Indexes[visibleIndex]
				return m.performPrimaryAction(m.ActionIndex)
			}
		}
		return m, nil
	}
	if m.Mode == ModeConfirm {
		if key == "esc" {
			return m.perform(uiCancel)
		}
		switch key {
		case "left", "shift+tab":
			m.moveAction(-1)
			return m, nil
		case "right", "tab":
			m.moveAction(1)
			return m, nil
		case "up", "k":
			m.moveOverlayScroll(-1)
			return m, nil
		case "down", "j":
			m.moveOverlayScroll(1)
			return m, nil
		case "pgup":
			m.moveOverlayScroll(-m.overlayPageSize())
			return m, nil
		case "pgdown":
			m.moveOverlayScroll(m.overlayPageSize())
			return m, nil
		}
		if key == "enter" {
			return m.performPrimaryAction(m.ActionIndex)
		}
		if key == "y" {
			return m.perform(uiConfirm)
		}
		return m, nil
	}
	if key == "ctrl+p" || key == ":" {
		m.enterCommandPalette()
		return m, nil
	}
	if view, ok := viewKey(key); ok {
		m.switchView(view)
		return m, nil
	}
	if key == "tab" {
		return m.advanceFocus()
	}
	if key == "shift+tab" {
		return m.reverseFocus()
	}
	if m.Focus == FocusNavigation {
		entries := layoutNavigationEntries(ComputeLayout(m.Width, m.Height), m)
		switch key {
		case "up", "k", "left":
			m.NavigationIndex = max(0, m.NavigationIndex-1)
			return m, nil
		case "down", "j", "right":
			m.NavigationIndex = min(max(0, len(entries)-1), m.NavigationIndex+1)
			return m, nil
		case "enter":
			if len(entries) > 0 {
				return m.activateCommandEntry(entries[min(m.NavigationIndex, len(entries)-1)].Entry)
			}
			return m, nil
		case "esc":
			m.Focus = FocusList
			return m, nil
		}
	}
	if m.Focus == FocusStatus {
		switch key {
		case "enter":
			return m.openActivityReview()
		case "esc":
			m.Focus = FocusList
			return m, nil
		}
	}
	if m.ActiveView == ViewOverview && m.Mode == ModeTable && m.Focus == FocusList {
		switch key {
		case "left":
			m.switchOverviewSection(previousOverviewSection(m.OverviewSection))
			return m, nil
		case "right":
			m.switchOverviewSection(nextOverviewSection(m.OverviewSection))
			return m, nil
		}
	}
	if m.Focus == FocusCollectionActions || m.Focus == FocusDetailActions {
		switch key {
		case "left":
			m.moveAction(-1)
			return m, nil
		case "right":
			m.moveAction(1)
			return m, nil
		case "enter":
			return m.performPrimaryAction(m.ActionIndex)
		case "esc":
			if m.Focus == FocusDetailActions {
				m.Focus = FocusDetail
			} else {
				m.Focus = FocusList
			}
			m.ActionIndex = 0
			return m, nil
		}
	}
	if m.Focus == FocusActions {
		switch key {
		case "left":
			m.moveAction(-1)
			return m, nil
		case "right":
			m.moveAction(1)
			return m, nil
		case "enter":
			return m.performPrimaryAction(m.ActionIndex)
		case "esc":
			m.Focus = FocusList
			if ComputeLayout(m.Width, m.Height).Detail.Empty() {
				m.Detail = false
			}
			return m, nil
		}
	}
	if m.Focus == FocusList && key == "right" && len(m.detailActions()) > 0 {
		if ComputeLayout(m.Width, m.Height).Detail.Empty() {
			m.Detail = true
		}
		m.Focus, m.ActionPane, m.ActionIndex = FocusDetail, actionPaneDetail, 0
		return m, nil
	}
	if m.Focus == FocusDetail {
		if key == "esc" {
			m.Focus = FocusList
			if ComputeLayout(m.Width, m.Height).Detail.Empty() {
				m.Detail = false
			}
			return m, nil
		}
		if key == "left" {
			m.Focus = FocusList
			return m, nil
		}
		if key == "right" && len(m.detailActions()) > 0 {
			m.Focus, m.ActionPane, m.ActionIndex = FocusDetailActions, actionPaneDetail, 0
			return m, nil
		}
		switch key {
		case "j", "down":
			m.moveDetailScroll(1)
			return m, nil
		case "k", "up":
			m.moveDetailScroll(-1)
			return m, nil
		case "pgdown":
			m.moveDetailScroll(m.detailPageSize())
			return m, nil
		case "pgup":
			m.moveDetailScroll(-m.detailPageSize())
			return m, nil
		}
	}
	switch key {
	case "?":
		m.Help = true
	case "/":
		m.beginFilter()
	case "j", "down":
		return m.perform(uiMoveDown)
	case "k", "up":
		return m.perform(uiMoveUp)
	case " ":
		return m.perform(uiToggle)
	case "enter":
		return m.perform(uiActivate)
	case "r":
		if m.ActiveView == ViewStatus {
			return m.refreshStatus()
		}
		return m.perform(uiRefresh)
	case "ctrl+k":
		m.enterConfiguration()
		return m, configurationCmd(m.ctx, m.service)
	case "u":
		m.openUpdates()
	case "a":
		if m.ActiveView == ViewLibrary {
			m.enterInput(inputState{Kind: inputAddSource, Prompt: "Local path, owner/repo, Git URL, or skills.sh URL"})
			m.Status = "Enter a local path or remote Git source"
		}
	case "n":
		if m.ActiveView == ViewPresets && m.Scope.Level == "" {
			m.enterInput(inputState{Kind: inputPresetCreate, Prompt: "Preset name"})
			m.Status = "Enter the new preset name"
		}
	case "w":
		if m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills" {
			m.pendingPreset = app.PresetMutationRequest{Operation: app.PresetEditMembers, Name: m.Scope.Preset, Skills: m.selectedIDs()}
			m.confirm, m.Busy, m.Status = ActionPreset, true, "Building preset member preview..."
			return m, presetMutationPreviewCmd(m.ctx, m.service, m.pendingPreset)
		}
	case "d":
		if m.ActiveView == ViewLibrary {
			return m.confirmCurrent(ActionRemoveSkill)
		}
	case "x":
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-projects" {
			return m.previewCurrentProjectRemove()
		}
	case "S":
		if m.ActiveView == ViewLibrary {
			m.Busy, m.Status = true, "Building migration preview..."
			return m, scanCmd(m.ctx, m.migration, app.ScanRequest{All: true, AllProjects: true, DryRun: true})
		}
	case "A":
		if m.ActiveView == ViewStatus {
			return m.previewSelectedStatusAdopt()
		}
	case "s":
		if m.ActiveView == ViewStatus && m.selectedStatusCanSync() {
			m.pendingSync = app.SyncRequest{}
			m.Busy, m.Status = true, "Building sync preview..."
			return m, syncPreviewCmd(m.ctx, m.service, app.SyncRequest{DryRun: true})
		}
	case "esc":
		return m.perform(uiCancel)
	}
	return m, nil
}

func (m Model) updateReadingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.Help {
		switch key {
		case "esc", "?":
			return m.perform(uiCancel)
		case "up", "k":
			m.moveOverlayScroll(-1)
		case "down", "j":
			m.moveOverlayScroll(1)
		case "pgup":
			m.moveOverlayScroll(-m.overlayPageSize())
		case "pgdown":
			m.moveOverlayScroll(m.overlayPageSize())
		}
		return m, nil
	}
	if m.Mode == ModeFilter {
		switch key {
		case "esc":
			m.cancelFilterDraft()
		case "enter":
			m.applyFilterDraft()
		case "backspace":
			chars := []rune(m.FilterDraft)
			if len(chars) > 0 {
				m.FilterDraft = string(chars[:len(chars)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.FilterDraft += string(msg.Runes)
			}
		}
		m.Cursor, m.Scroll = 0, 0
		m.clampCursor()
		return m, nil
	}
	if m.hasOverlay() {
		switch key {
		case "esc":
			return m.perform(uiCancel)
		case "up", "k":
			m.moveOverlayScroll(-1)
		case "down", "j":
			m.moveOverlayScroll(1)
		case "pgup":
			m.moveOverlayScroll(-m.overlayPageSize())
		case "pgdown":
			m.moveOverlayScroll(m.overlayPageSize())
		}
		return m, nil
	}
	if view, ok := viewKey(key); ok {
		m.switchView(view)
		return m, nil
	}
	switch key {
	case "tab":
		return m.advanceFocus()
	case "shift+tab":
		return m.reverseFocus()
	case "?":
		m.Help = true
		return m, nil
	case "/":
		m.beginFilter()
		return m, nil
	case "esc":
		return m.perform(uiCancel)
	}
	if m.Focus == FocusNavigation {
		entries := layoutNavigationEntries(ComputeLayout(m.Width, m.Height), m)
		switch key {
		case "up", "k", "left":
			m.NavigationIndex = max(0, m.NavigationIndex-1)
		case "down", "j", "right":
			m.NavigationIndex = min(max(0, len(entries)-1), m.NavigationIndex+1)
		case "enter":
			if len(entries) > 0 {
				entry := entries[min(m.NavigationIndex, len(entries)-1)].Entry
				if entry.Kind == navigationView {
					return m.activateCommandEntry(entry)
				}
			}
		}
		return m, nil
	}
	if m.Focus == FocusDetail {
		switch key {
		case "down", "j":
			m.moveDetailScroll(1)
		case "up", "k":
			m.moveDetailScroll(-1)
		case "pgdown":
			m.moveDetailScroll(m.detailPageSize())
		case "pgup":
			m.moveDetailScroll(-m.detailPageSize())
		case "left":
			m.Focus = FocusList
		}
		return m, nil
	}
	switch key {
	case "down", "j":
		return m.perform(uiMoveDown)
	case "up", "k":
		return m.perform(uiMoveUp)
	case "left":
		if m.ActiveView == ViewOverview {
			m.switchOverviewSection(previousOverviewSection(m.OverviewSection))
		}
	case "right":
		if m.ActiveView == ViewOverview {
			m.switchOverviewSection(nextOverviewSection(m.OverviewSection))
		} else if len(m.detailActions()) > 0 {
			m.Detail, m.Focus = true, FocusDetail
		}
	}
	return m, nil
}

func (m *Model) moveMoreAction(delta int) {
	actions := m.primaryActions()
	if len(actions) == 0 {
		m.ActionIndex, m.OverlayScroll = 0, 0
		return
	}
	m.ActionIndex = min(max(0, m.ActionIndex+delta), len(actions)-1)
	m.OverlayScroll = layoutMoreActions(ComputeLayout(m.Width, m.Height), actions, m.ActionIndex, m.OverlayScroll).Start
}

func (m *Model) moveOverlayScroll(delta int) {
	layout := ComputeLayout(m.Width, m.Height)
	panel := layoutOverlayPanel(layout, m.overlayPanelActions(), m.Focus == FocusActions, m.ActionIndex)
	overlay := m.overlayLines()
	body := []string(nil)
	if len(overlay) > 1 {
		body = wrapOverlayBody(overlay[1:], panel.Body.Width)
	}
	bodyLength := len(body)
	capacity := overlayBodyCapacity(layout, bodyLength)
	maximum := max(0, bodyLength-capacity)
	m.OverlayScroll = min(max(0, m.OverlayScroll+delta), maximum)
}

func (m Model) overlayPageSize() int {
	layout := ComputeLayout(m.Width, m.Height)
	panel := layoutOverlayPanel(layout, m.overlayPanelActions(), m.Focus == FocusActions, m.ActionIndex)
	overlay := m.overlayLines()
	bodyLength := 0
	if len(overlay) > 1 {
		bodyLength = len(wrapOverlayBody(overlay[1:], panel.Body.Width))
	}
	return overlayBodyCapacity(layout, bodyLength)
}

func (m Model) advanceFocus() (tea.Model, tea.Cmd) {
	m.moveFocus(1)
	return m, nil
}

func (m Model) reverseFocus() (tea.Model, tea.Cmd) {
	m.moveFocus(-1)
	return m, nil
}

func (m *Model) moveFocus(delta int) {
	order := m.visibleFocusOrder()
	if len(order) == 0 {
		m.Focus = FocusList
		return
	}
	index := 0
	for current, focus := range order {
		if focus == m.Focus {
			index = current
			break
		}
	}
	index = (index + delta + len(order)) % len(order)
	m.setPaneFocus(order[index])
}

func (m Model) visibleFocusOrder() []Focus {
	layout := ComputeLayout(m.Width, m.Height)
	order := make([]Focus, 0, 5)
	if !layout.NavigationPanel.Outer.Empty() {
		order = append(order, FocusNavigation)
	}
	if !m.displayedActivity().Review.Empty() {
		order = append(order, FocusStatus)
	}
	compactDetail := layout.DetailPanel.Outer.Empty() && (m.Detail || m.hasPinnedDetail())
	if compactDetail {
		order = append(order, FocusDetail)
		if len(m.detailActions()) > 0 {
			order = append(order, FocusDetailActions)
		}
		return order
	}
	order = append(order, FocusList)
	if len(m.collectionActions()) > 0 {
		order = append(order, FocusCollectionActions)
	}
	if !layout.DetailPanel.Outer.Empty() && len(m.rows()) > 0 {
		order = append(order, FocusDetail)
		if len(m.detailActions()) > 0 {
			order = append(order, FocusDetailActions)
		}
	}
	return order
}

func (m *Model) setPaneFocus(focus Focus) {
	m.Focus, m.ActionIndex = focus, 0
	switch focus {
	case FocusNavigation:
		m.ActionPane = actionPaneNone
		m.syncNavigationIndex()
	case FocusStatus:
		m.ActionPane = actionPaneNone
	case FocusCollectionActions:
		m.ActionPane = actionPaneCollection
	case FocusDetailActions:
		m.ActionPane = actionPaneDetail
	default:
		m.ActionPane = actionPaneNone
	}
}

func (m Model) detailFocusVisible() bool {
	layout := ComputeLayout(m.Width, m.Height)
	return m.Detail || layout.Narrow || !layout.Detail.Empty()
}

func (m Model) detailPageSize() int {
	return max(1, ComputeLayout(m.Width, m.Height).Main.Height-2)
}

func (m *Model) moveDetailScroll(delta int) {
	layout := ComputeLayout(m.Width, m.Height)
	width := layout.List.Width
	if !layout.Detail.Empty() {
		width = layout.Detail.Width
	}
	pageSize := m.detailPageSize()
	maximum := max(0, len(m.detailLinesForArea(width, pageSize))-pageSize)
	m.DetailScroll = min(max(0, m.DetailScroll+delta), maximum)
}

func (m *Model) moveAction(delta int) {
	actions := m.currentActions()
	if len(actions) == 0 {
		m.ActionIndex = 0
		return
	}
	m.ActionIndex = (m.ActionIndex + delta + len(actions)) % len(actions)
}

func (m *Model) moveActionViewport(delta int) {
	actions := m.currentActions()
	if len(actions) == 0 {
		m.ActionIndex = 0
		return
	}
	m.Focus = FocusActions
	m.ActionIndex = min(max(0, m.ActionIndex+delta), len(actions)-1)
}

func viewKey(key string) (View, bool) {
	switch key {
	case "1":
		return ViewOverview, true
	case "2":
		return ViewLibrary, true
	case "3":
		return ViewPresets, true
	case "4":
		return ViewStatus, true
	default:
		return "", false
	}
}
