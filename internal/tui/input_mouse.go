package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/silenceper/aikit/internal/app"
)

type HitRegions struct {
	Layout                  Layout
	Tabs                    map[View]Rect
	Navigation              []navigationEntryItem
	Commands                []Rect
	CommandIndexes          []int
	Rows                    []Rect
	Checkboxes              []Rect
	Actions                 []Rect
	ActionIndexes           []int
	ActionBar               Rect
	Confirm                 Rect
	Cancel                  Rect
	Back                    Rect
	ActionPrev              Rect
	ActionNext              Rect
	actionPrev              int
	actionNext              int
	CollectionActions       PaneActionRegions
	DetailActions           PaneActionRegions
	OverviewSections        map[overviewSectionID]Rect
	OverviewRows            map[overviewSectionID][]Rect
	OverviewCheckboxes      map[overviewSectionID][]Rect
	OverviewCheckboxIndexes map[overviewSectionID][]int
	OverviewActions         map[overviewSectionID]PaneActionRegions
	OverviewQuick           PaneActionRegions
	OverviewPrevious        Rect
	OverviewNext            Rect
	ActivityStatus          Rect
}

type PaneActionRegions struct {
	Bar                      Rect
	Previous                 Rect
	Next                     Rect
	Buttons                  []Rect
	Indexes                  []int
	PreviousIndex, NextIndex int
}

func (m Model) hitRegions() HitRegions {
	layout := ComputeLayout(m.Width, m.Height)
	regions := HitRegions{
		Layout: layout, Tabs: make(map[View]Rect), actionPrev: -1, actionNext: -1,
		OverviewSections: make(map[overviewSectionID]Rect), OverviewRows: make(map[overviewSectionID][]Rect),
		OverviewCheckboxes: make(map[overviewSectionID][]Rect), OverviewCheckboxIndexes: make(map[overviewSectionID][]int), OverviewActions: make(map[overviewSectionID]PaneActionRegions),
	}
	if layout.TooShort {
		return regions
	}
	for _, item := range layoutNavigation(layout, topViews, m.ActiveView) {
		regions.Tabs[item.View] = item.Rect
	}
	regions.Navigation = layoutNavigationEntries(layout, m)
	if activity := m.displayedActivity(); activity.Kind != ActivityIdle && !layout.FooterPanel.Body.Empty() {
		text := m.renderFooterActivity(activity, layout.FooterPanel.Body.Width)
		regions.ActivityStatus = Rect{X: layout.FooterPanel.Body.X, Y: layout.FooterPanel.Body.Y, Width: min(layout.FooterPanel.Body.Width, lipgloss.Width(text)), Height: 1}
	}
	if layout.Narrow && !layout.Breadcrumb.Empty() {
		regions.Back = Rect{X: layout.Breadcrumb.X, Y: layout.Breadcrumb.Y, Width: min(2, layout.Breadcrumb.Width), Height: 1}
	}
	if m.ActiveView == ViewOverview && m.Mode == ModeTable && !m.hasOverlay() {
		geometry := m.overviewLayout(layout)
		regions.OverviewPrevious, regions.OverviewNext = geometry.Previous, geometry.Next
		regions.OverviewQuick = paneActionHitRegions(geometry.Quick.Actions, []string{"Add skill", "Add project", "Create preset"}, m.OverviewSection == overviewQuick, m.ActionIndex)
		dashboard := m.overviewDashboard()
		for _, section := range []overviewSectionID{overviewUpdates, overviewLocal, overviewHealth} {
			panel := geometry.panel(section)
			if panel.Outer.Empty() {
				continue
			}
			regions.OverviewSections[section] = panel.Outer
			regions.OverviewRows[section] = append(regions.OverviewRows[section], geometry.Rows[section].Rects...)
			tasks := dashboard.tasks(section)
			for visible, rect := range geometry.Rows[section].Rects {
				index := geometry.Rows[section].Start + visible
				if index < len(tasks) && tasks[index].Selectable {
					regions.OverviewCheckboxes[section] = append(regions.OverviewCheckboxes[section], Rect{X: rect.X, Y: rect.Y, Width: min(3, rect.Width), Height: rect.Height})
					regions.OverviewCheckboxIndexes[section] = append(regions.OverviewCheckboxIndexes[section], index)
				}
			}
			regions.OverviewActions[section] = paneActionHitRegions(panel.Actions, m.overviewSectionActions(section), section == geometry.Active && m.Focus == FocusCollectionActions, m.ActionIndex)
		}
		return regions
	}
	if m.Mode != ModeTable || !m.Detail {
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
	} else if m.Mode == ModeCommand {
		panel := layoutOverlayPanel(layout, nil, false, 0)
		entries := m.commandEntries()
		for index := range entries {
			y := panel.Body.Y + 1 + index
			if y >= panel.Body.Bottom() {
				break
			}
			regions.Commands = append(regions.Commands, Rect{X: panel.Body.X, Y: y, Width: panel.Body.Width, Height: 1})
			regions.CommandIndexes = append(regions.CommandIndexes, index)
		}
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
	} else if layout.List.Height > 1 && m.Mode != ModeTable {
		actionArea := layout.CollectionPanel.Actions
		if actionArea.Empty() {
			actionArea = layout.List
			actionArea.Y = actionArea.Bottom() - 1
			actionArea.Height = 1
		}
		regions.CollectionActions = paneActionHitRegions(actionArea, m.primaryActions(), m.Focus == FocusActions, m.ActionIndex)
		regions.Actions = append(regions.Actions, regions.CollectionActions.Buttons...)
		regions.ActionIndexes = append(regions.ActionIndexes, regions.CollectionActions.Indexes...)
		regions.ActionBar = regions.CollectionActions.Bar
		regions.ActionPrev, regions.ActionNext = regions.CollectionActions.Previous, regions.CollectionActions.Next
		regions.actionPrev, regions.actionNext = regions.CollectionActions.PreviousIndex, regions.CollectionActions.NextIndex
	} else if layout.List.Height > 1 {
		collectionArea, detailArea := layout.CollectionPanel.Actions, layout.DetailPanel.Actions
		collectionActions, detailActions := m.collectionActions(), m.detailActions()
		if detailArea.Empty() {
			if (m.Detail || m.hasPinnedDetail()) && !m.librarySelectionBarActive() {
				detailArea, collectionArea = collectionArea, Rect{}
			} else {
				detailActions = nil
			}
		}
		regions.CollectionActions = m.collectionActionHitRegions(collectionArea, collectionActions, m.Focus == FocusCollectionActions, m.ActionIndex)
		regions.DetailActions = paneActionHitRegions(detailArea, detailActions, m.Focus == FocusDetailActions, m.ActionIndex)
		regions.populateLegacyActions(m.primaryActions(), collectionActions, detailActions)
	}
	return regions
}

func (m Model) collectionActionHitRegions(area Rect, actions []string, focused bool, selected int) PaneActionRegions {
	if !m.librarySelectionBarActive() {
		return paneActionHitRegions(area, actions, focused, selected)
	}
	result := PaneActionRegions{PreviousIndex: -1, NextIndex: -1}
	if area.Empty() {
		return result
	}
	bar := layoutLibrarySelectionBar(m.librarySelectionCount(), m.librarySelectionActionLabels(), focused, selected, area.Width).Actions
	result.Bar = Rect{X: area.X, Y: area.Y, Width: area.Width, Height: 1}
	result.Previous = translateRect(bar.Previous, area.X, area.Y)
	result.Next = translateRect(bar.Next, area.X, area.Y)
	result.PreviousIndex, result.NextIndex = bar.PreviousIndex, bar.NextIndex
	result.Indexes = append(result.Indexes, bar.ButtonIndexes...)
	for _, button := range bar.Buttons {
		result.Buttons = append(result.Buttons, translateRect(button, area.X, area.Y))
	}
	return result
}

func paneActionHitRegions(area Rect, actions []string, focused bool, selected int) PaneActionRegions {
	result := PaneActionRegions{PreviousIndex: -1, NextIndex: -1}
	if area.Empty() || len(actions) == 0 {
		return result
	}
	bar := layoutActionBar(actions, focused, selected, area.Width)
	result.Bar = Rect{X: area.X, Y: area.Y, Width: bar.Bar.Width, Height: bar.Bar.Height}
	result.Previous = translateRect(bar.Previous, area.X, area.Y)
	result.Next = translateRect(bar.Next, area.X, area.Y)
	result.PreviousIndex, result.NextIndex = bar.PreviousIndex, bar.NextIndex
	result.Indexes = append(result.Indexes, bar.ButtonIndexes...)
	for _, button := range bar.Buttons {
		result.Buttons = append(result.Buttons, translateRect(button, area.X, area.Y))
	}
	return result
}

func (regions *HitRegions) populateLegacyActions(actions, collectionActions, detailActions []string) {
	for index, label := range actions {
		if rect, ok := paneActionButton(detailActions, regions.DetailActions, label); ok {
			regions.Actions = append(regions.Actions, rect)
			regions.ActionIndexes = append(regions.ActionIndexes, index)
			continue
		}
		if rect, ok := paneActionButton(collectionActions, regions.CollectionActions, label); ok {
			regions.Actions = append(regions.Actions, rect)
			regions.ActionIndexes = append(regions.ActionIndexes, index)
		}
	}
	active := regions.DetailActions
	if active.Bar.Empty() {
		active = regions.CollectionActions
	}
	regions.ActionBar = active.Bar
	regions.ActionPrev, regions.ActionNext = active.Previous, active.Next
	regions.actionPrev, regions.actionNext = active.PreviousIndex, active.NextIndex
}

func paneActionButton(actions []string, regions PaneActionRegions, label string) (Rect, bool) {
	for visible, index := range regions.Indexes {
		if visible < len(regions.Buttons) && index < len(actions) && actions[index] == label {
			return regions.Buttons[visible], true
		}
	}
	return Rect{}, false
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
	if m.MutationBusy {
		return m, nil
	}
	regions := m.hitRegions()
	if m.readingActivityActive() {
		return m.updateReadingMouse(msg, regions)
	}
	if m.hasOverlay() {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.Mode == ModeCommand {
			entries := m.commandEntries()
			for visible, rect := range regions.Commands {
				if !rect.Contains(msg.X, msg.Y) {
					continue
				}
				index := regions.CommandIndexes[visible]
				m.CommandIndex = index
				if index < len(entries) {
					return m.activateCommandEntry(entries[index])
				}
			}
		}
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
	if m.ActiveView == ViewOverview && m.Mode == ModeTable {
		return m.updateOverviewMouse(msg, regions)
	}
	if m.Mode == ModeTable && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		if regions.CollectionActions.Bar.Contains(msg.X, msg.Y) {
			return m.movePaneActionWithMouse(actionPaneCollection, regions.CollectionActions, msg)
		}
		if regions.DetailActions.Bar.Contains(msg.X, msg.Y) {
			return m.movePaneActionWithMouse(actionPaneDetail, regions.DetailActions, msg)
		}
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
	if regions.ActivityStatus.Contains(msg.X, msg.Y) && !m.displayedActivity().Review.Empty() {
		m.Focus = FocusStatus
		return m, nil
	}
	if m.Mode == ModeTable {
		if next, cmd, ok := m.clickPaneAction(actionPaneCollection, regions.CollectionActions, msg.X, msg.Y); ok {
			return next, cmd
		}
		if next, cmd, ok := m.clickPaneAction(actionPaneDetail, regions.DetailActions, msg.X, msg.Y); ok {
			return next, cmd
		}
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
	for _, item := range regions.Navigation {
		if item.Rect.Contains(msg.X, msg.Y) {
			return m.activateCommandEntry(item.Entry)
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
			selectableRow := m.ActiveView == ViewLibrary || (m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills")
			if selectableRow {
				current := m.rows()[m.Cursor]
				if regions.Layout.Detail.Empty() && m.rowSelected(current) && len(m.detailActions()) > 0 {
					m.Detail, m.Focus, m.ActionPane = true, FocusDetail, actionPaneDetail
					return m, nil
				}
				return m.perform(uiToggle)
			}
			if regions.Layout.Detail.Empty() && len(m.detailActions()) > 0 {
				m.Detail, m.Focus, m.ActionPane = true, FocusDetail, actionPaneDetail
				return m, nil
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

func (m Model) updateReadingMouse(msg tea.MouseMsg, regions HitRegions) (tea.Model, tea.Cmd) {
	if m.hasOverlay() {
		if msg.Button == tea.MouseButtonWheelUp {
			m.moveOverlayScroll(-1)
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.moveOverlayScroll(1)
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if m.Detail && regions.Layout.Main.Contains(msg.X, msg.Y) {
			m.Focus = FocusDetail
			if msg.Button == tea.MouseButtonWheelUp {
				m.moveDetailScroll(-1)
			} else {
				m.moveDetailScroll(1)
			}
			return m, nil
		}
		if regions.Layout.List.Contains(msg.X, msg.Y) {
			if msg.Button == tea.MouseButtonWheelUp {
				return m.perform(uiMoveUp)
			}
			return m.perform(uiMoveDown)
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if regions.Back.Contains(msg.X, msg.Y) {
		return m.perform(uiBack)
	}
	for _, item := range regions.Navigation {
		if item.Rect.Contains(msg.X, msg.Y) && item.Entry.Kind == navigationView {
			return m.activateCommandEntry(item.Entry)
		}
	}
	start := m.visibleRowsLayout(regions.Layout).Start
	for visible, rect := range regions.Rows {
		if rect.Contains(msg.X, msg.Y) {
			m.Cursor = start + visible
			m.Focus, m.ActionIndex = FocusList, 0
			m.ensureVisible()
			return m, nil
		}
	}
	if (m.Detail && regions.Layout.Main.Contains(msg.X, msg.Y)) || (!regions.Layout.Detail.Empty() && regions.Layout.Detail.Contains(msg.X, msg.Y)) {
		m.Focus = FocusDetail
	}
	return m, nil
}

func (m Model) updateOverviewMouse(msg tea.MouseMsg, regions HitRegions) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		for _, section := range []overviewSectionID{overviewUpdates, overviewLocal, overviewHealth} {
			if !regions.OverviewSections[section].Contains(msg.X, msg.Y) {
				continue
			}
			m.switchOverviewSection(section)
			if msg.Button == tea.MouseButtonWheelUp {
				return m.perform(uiMoveUp)
			}
			return m.perform(uiMoveDown)
		}
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if regions.Back.Contains(msg.X, msg.Y) {
		return m.perform(uiBack)
	}
	if regions.OverviewPrevious.Contains(msg.X, msg.Y) {
		m.switchOverviewSection(previousOverviewSection(m.OverviewSection))
		return m, nil
	}
	if regions.OverviewNext.Contains(msg.X, msg.Y) {
		m.switchOverviewSection(nextOverviewSection(m.OverviewSection))
		return m, nil
	}
	for _, item := range regions.Navigation {
		if item.Rect.Contains(msg.X, msg.Y) {
			return m.activateCommandEntry(item.Entry)
		}
	}
	for visible, rect := range regions.OverviewQuick.Buttons {
		if !rect.Contains(msg.X, msg.Y) {
			continue
		}
		m.OverviewSection, m.ActionIndex = overviewQuick, regions.OverviewQuick.Indexes[visible]
		return m.perform([]uiAction{uiAddSource, uiCreateProject, uiCreatePreset}[regions.OverviewQuick.Indexes[visible]])
	}
	geometry := m.overviewLayout(regions.Layout)
	for _, section := range []overviewSectionID{overviewUpdates, overviewLocal, overviewHealth} {
		actions := regions.OverviewActions[section]
		for visible, rect := range actions.Buttons {
			if !rect.Contains(msg.X, msg.Y) || visible >= len(actions.Indexes) {
				continue
			}
			m.switchOverviewSection(section)
			m.Focus, m.ActionIndex = FocusCollectionActions, actions.Indexes[visible]
			return m.performPrimaryAction(m.ActionIndex)
		}
		for visible, rect := range regions.OverviewCheckboxes[section] {
			if !rect.Contains(msg.X, msg.Y) || visible >= len(regions.OverviewCheckboxIndexes[section]) {
				continue
			}
			m.switchOverviewSection(section)
			m.Cursor = regions.OverviewCheckboxIndexes[section][visible]
			m.Focus = FocusList
			return m.toggleSelected()
		}
		rows := regions.OverviewRows[section]
		for visible, rect := range rows {
			if !rect.Contains(msg.X, msg.Y) {
				continue
			}
			m.switchOverviewSection(section)
			m.Cursor = geometry.Rows[section].Start + visible
			m.Focus = FocusList
			m.ensureOverviewVisible()
			return m, nil
		}
	}
	return m, nil
}

func (m Model) clickPaneAction(pane actionPane, regions PaneActionRegions, x, y int) (tea.Model, tea.Cmd, bool) {
	if regions.Previous.Contains(x, y) && regions.PreviousIndex >= 0 {
		m.ActionPane, m.Focus, m.ActionIndex = pane, pane.focus(), regions.PreviousIndex
		return m, nil, true
	}
	if regions.Next.Contains(x, y) && regions.NextIndex >= 0 {
		m.ActionPane, m.Focus, m.ActionIndex = pane, pane.focus(), regions.NextIndex
		return m, nil, true
	}
	for visible, rect := range regions.Buttons {
		if !rect.Contains(x, y) || visible >= len(regions.Indexes) {
			continue
		}
		m.ActionPane, m.Focus, m.ActionIndex = pane, pane.focus(), regions.Indexes[visible]
		next, cmd := m.performPrimaryAction(m.ActionIndex)
		return next, cmd, true
	}
	if regions.Bar.Contains(x, y) {
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) movePaneActionWithMouse(pane actionPane, regions PaneActionRegions, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}
	actions := m.collectionActions()
	if pane == actionPaneDetail {
		actions = m.detailActions()
	}
	if len(actions) == 0 {
		return m, nil
	}
	delta := 1
	if msg.Button == tea.MouseButtonWheelUp {
		delta = -1
	}
	m.ActionPane, m.Focus = pane, pane.focus()
	m.ActionIndex = min(max(0, m.ActionIndex+delta), len(actions)-1)
	return m, nil
}

func (pane actionPane) focus() Focus {
	if pane == actionPaneDetail {
		return FocusDetailActions
	}
	return FocusCollectionActions
}

func (m Model) hasOverlay() bool {
	return m.Help || m.Mode == ModeConfiguration || m.Mode == ModeFilter || m.Mode == ModeCommand || m.Mode == ModeConfirm || m.Mode == ModeInput || m.Mode == ModeMore || m.Mode == ModeErrorDetail
}

func (m Model) performPrimaryAction(index int) (tea.Model, tea.Cmd) {
	actions := m.currentActions()
	if index < 0 || index >= len(actions) {
		return m, nil
	}
	if m.librarySelectionBarActive() && m.Mode == ModeTable && (m.Focus == FocusCollectionActions || m.ActionPane == actionPaneCollection) {
		selectionActions := m.librarySelectionActions()
		if index < len(selectionActions) {
			return m.performLibrarySelectionAction(selectionActions[index])
		}
	}
	if m.ActiveView == ViewOverview && m.OverviewSection == overviewHealth && (actions[index] == "Open" || actions[index] == "Adopt" || actions[index] == "Sync preview" || actions[index] == "Review recovery") {
		return m.beginOverviewHealthAction(actions[index])
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
	case "Add project":
		return m.perform(uiCreateProject)
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
	case "Import selected", "Adopt selected", "Apply selected":
		if m.ActiveView == ViewOverview {
			return m.beginOverviewLocalPreview()
		}
	case "Review all":
		if m.ActiveView == ViewOverview {
			if m.OverviewSection == overviewLocal {
				m.switchView(ViewMigration)
			} else {
				m.switchView(ViewStatus)
			}
			return m, nil
		}
	case "Change ref":
		if len(m.selectedLibraryIDs()) != 1 {
			m.Err = "change ref requires exactly one selected library skill"
			return m, nil
		}
		m.enterInput(inputState{Kind: inputRefChange, Prompt: "New ref (branch:name, tag:name, or commit:object-id)"})
		m.Status = "Enter the explicit replacement ref"
		return m, nil
	case "Enable selected", "Disable selected", "Update selected", "Remove selected":
		if m.ActiveView == ViewOverview && actions[index] == "Update selected" {
			return m.beginOverviewUpdatePreview()
		}
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
	case "Edit members":
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
	case "Add skill":
		if m.ActiveView == ViewOverview {
			return m.perform(uiAddSource)
		}
		if m.ActiveView != ViewWorkspaces || m.Scope.Level != "workspace-projects" {
			return m, nil
		}
		rows := m.rows()
		if m.Cursor < 0 || m.Cursor >= len(rows) {
			return m, nil
		}
		m.Scope = Scope{Project: rows[m.Cursor].ID, Level: "project-targets"}
		m.workspaceIntent = "add-skill"
		m.Cursor, m.Scroll = 0, 0
		m.Status = "Choose Common or an agent for the new skill binding"
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
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-projects" {
			rows := m.rows()
			if m.Cursor < 0 || m.Cursor >= len(rows) {
				return m, nil
			}
			m.Scope = Scope{Project: rows[m.Cursor].ID, Level: "project-targets"}
			m.workspaceIntent = "apply-preset"
			m.Cursor, m.Scroll = 0, 0
			m.Status = "Choose the exact Common or agent target for this preset"
			return m, nil
		}
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
