package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
)

func (m Model) ViewString() string { return m.render() }
func (m Model) View() string       { return m.render() }

func (m Model) render() string {
	layout := ComputeLayout(max(20, m.Width), max(1, m.Height))
	if layout.TooShort {
		lines := make([]string, layout.Height)
		message := clipPlain("terminal too short; need 8 rows", layout.Width)
		if len(lines) > 0 {
			lines[len(lines)/2] = message
		}
		return strings.Join(lines, "\n")
	}
	if layout.LowHeight {
		return m.renderLowHeightShell(layout)
	}
	return m.renderFramedShell(layout)
}

func (m Model) frameGlyphs() FrameGlyphs {
	if uiTheme.mode == themeNoColor || uiTheme.mode == themeReduced {
		return asciiFrameGlyphs()
	}
	return unicodeFrameGlyphs()
}

func (m Model) renderFramedShell(layout Layout) string {
	glyphs := m.frameGlyphs()
	groups := []positionedLines{
		{X: layout.AppBar.Outer.X, Y: layout.AppBar.Outer.Y, Lines: renderPanel(layout.AppBar, "aikit", false, []string{m.renderAppContext(layout.AppBar.Body.Width)}, "", glyphs)},
		{X: layout.FooterPanel.Outer.X, Y: layout.FooterPanel.Outer.Y, Lines: renderPanel(layout.FooterPanel, "Shortcuts", false, []string{m.renderFooterBody(layout.FooterPanel.Body.Width)}, "", glyphs)},
	}
	if !layout.NavigationPanel.Outer.Empty() {
		groups = append(groups, positionedLines{X: layout.NavigationPanel.Outer.X, Y: layout.NavigationPanel.Outer.Y, Lines: renderPanel(layout.NavigationPanel, "Navigation", false, m.renderNavigationBody(layout.NavigationPanel.Body.Width), "", glyphs)})
	}
	if m.hasOverlay() {
		panel, lines := m.renderFramedOverlay(layout, glyphs)
		groups = append(groups, positionedLines{X: panel.Outer.X, Y: panel.Outer.Y, Lines: lines})
	} else {
		collectionTitle, collectionBody := m.renderFramedCollection(layout)
		collectionAction := ""
		if layout.DetailPanel.Outer.Empty() {
			collectionAction = m.renderActionBar(layout.CollectionPanel.Actions.Width)
		}
		groups = append(groups, positionedLines{X: layout.CollectionPanel.Outer.X, Y: layout.CollectionPanel.Outer.Y, Lines: renderPanel(layout.CollectionPanel, collectionTitle, m.Focus == FocusList, collectionBody, collectionAction, glyphs)})
		if !layout.DetailPanel.Outer.Empty() {
			groups = append(groups, positionedLines{X: layout.DetailPanel.Outer.X, Y: layout.DetailPanel.Outer.Y, Lines: renderPanel(layout.DetailPanel, m.detailTitle(), m.Focus == FocusDetail || m.Focus == FocusActions, m.renderFramedDetail(layout.DetailPanel.Body), m.renderActionBar(layout.DetailPanel.Actions.Width), glyphs)})
		}
	}
	return strings.Join(composePositioned(layout.Width, layout.Height, groups...), "\n")
}

func (m Model) renderLowHeightShell(layout Layout) string {
	glyphs := m.frameGlyphs()
	title, body := m.renderFramedCollection(layout)
	if m.Detail || m.hasPinnedDetail() {
		title, body = m.detailTitle(), m.renderFramedDetail(layout.CollectionPanel.Body)
	}
	panel := layout.CollectionPanel
	panelLines := renderPanel(panel, title, true, body, m.renderActionBar(panel.Actions.Width), glyphs)
	groups := []positionedLines{{X: panel.Outer.X, Y: panel.Outer.Y, Lines: panelLines}}
	if m.hasOverlay() {
		overlay, lines := m.renderFramedOverlay(layout, glyphs)
		groups = []positionedLines{{X: overlay.Outer.X, Y: overlay.Outer.Y, Lines: lines}}
	}
	lines := composePositioned(layout.Width, layout.Height, groups...)
	if len(lines) > 0 {
		lines[0] = clip(m.renderAppBar(layout.Width), layout.Width)
	}
	if len(lines) > 1 {
		lines[len(lines)-1] = clip(m.footer(), layout.Width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderAppContext(width int) string {
	value := viewLabel(m.ActiveView) + m.scopeBreadcrumb()
	if m.Detail {
		rows := m.rows()
		if m.Cursor >= 0 && m.Cursor < len(rows) {
			value += " / " + rows[m.Cursor].Name
		}
	}
	if ComputeLayout(m.Width, m.Height).Narrow && (m.Detail || m.Scope.Level != "") {
		value = "‹ " + value
	}
	if m.Inventory.Loading {
		value += " · scanning"
	} else if m.Busy || m.MutationBusy {
		value += " · busy"
	}
	return clip(value, width)
}

func (m Model) renderFooterBody(width int) string {
	status := stripANSI(m.renderStatus(width))
	shortcut := m.footer()
	if strings.TrimSpace(status) == "" {
		return clip(shortcut, width)
	}
	if width < 60 {
		if m.Detail || m.Scope.Level != "" || m.Mode != ModeTable {
			return clip(shortcut, width)
		}
		return clip(status+" │ Ctrl+Q Quit", width)
	}
	return clip(status+" │ "+shortcut, width)
}

func (m Model) renderNavigationBody(width int) []string {
	layout := ComputeLayout(m.Width, m.Height)
	items := layoutNavigationEntries(layout, m)
	lines := make([]string, 0, layout.NavigationPanel.Body.Height)
	previousY := layout.NavigationPanel.Body.Y - 1
	mainIndex := 0
	for _, item := range items {
		for previousY+1 < item.Rect.Y {
			lines = append(lines, uiTheme.muted.Render("  Tools"))
			previousY++
		}
		label := item.Entry.Label
		if item.Entry.Kind == navigationView {
			label = m.navigationLabel(navigationItem{View: item.Entry.View, Label: item.Entry.Label, Active: item.Entry.View == m.ActiveView})
		}
		if item.Entry.Section == "Main" {
			mainIndex++
			label = fmt.Sprintf("%d %s", mainIndex, label)
		} else {
			label = "· " + label
		}
		active := item.Entry.Kind == navigationView && item.Entry.View == m.ActiveView
		if active {
			lines = append(lines, uiTheme.focused(label))
		} else {
			lines = append(lines, uiTheme.navigation.Render("  "+label))
		}
		previousY = item.Rect.Y
	}
	return lines
}

func (m Model) renderFramedCollection(layout Layout) (string, []string) {
	panel := layout.CollectionPanel
	if m.ActiveView == ViewOverview && m.Mode == ModeTable {
		return m.collectionTitle(), m.renderOverview(panel.Body.Width, panel.Body.Height)
	}
	if panel.Outer.Empty() {
		return m.collectionTitle(), nil
	}
	if (m.Detail || m.hasPinnedDetail()) && layout.DetailPanel.Outer.Empty() {
		return m.detailTitle(), m.renderFramedDetail(panel.Body)
	}
	rows := m.rows()
	geometry := m.visibleRowsLayout(layout)
	lines := make([]string, panel.Body.Height)
	if len(rows) == 0 && len(lines) > 0 {
		lines[0] = mutedStyle.Render(m.emptyState())
	}
	for visibleIndex, rect := range geometry.Rects {
		rowIndex := geometry.Start + visibleIndex
		if rowIndex < 0 || rowIndex >= len(rows) {
			continue
		}
		current := rows[rowIndex]
		if m.ActiveView == ViewMigration && current.Origin != "" {
			current.Name = current.Origin + " · " + current.Name
		}
		rendered := []string{m.renderRow(current, rowIndex == m.Cursor && m.Focus == FocusList, panel.Body.Width)}
		relativeY := rect.Y - panel.Body.Y
		for lineIndex := 0; lineIndex < len(rendered) && lineIndex < rect.Height; lineIndex++ {
			if y := relativeY + lineIndex; y >= 0 && y < len(lines) {
				lines[y] = rendered[lineIndex]
			}
		}
	}
	return rangeTitle(m.collectionTitle(), geometry.Start, geometry.End, len(rows)), lines
}

func (m Model) renderFramedDetail(area Rect) []string {
	if area.Empty() {
		return nil
	}
	detail := m.detailLinesForArea(area.Width, area.Height)
	start := min(max(0, m.DetailScroll), max(0, len(detail)-area.Height))
	end := min(len(detail), start+area.Height)
	if start >= end {
		return []string{"Select an item to see details."}
	}
	return detail[start:end]
}

func (m Model) renderFramedOverlay(layout Layout, glyphs FrameGlyphs) (PanelLayout, []string) {
	actions := m.overlayPanelActions()
	panel := framedOverlayPanel(layout, len(actions) > 0)
	overlay := m.overlayLines()
	if panel.Outer.Empty() || len(overlay) == 0 {
		return panel, nil
	}
	title, body := overlay[0], wrapOverlayBody(overlay[1:], panel.Body.Width)
	capacity := panel.Body.Height
	if len(body) > capacity {
		if capacity > 1 {
			capacity--
		}
		start := min(max(0, m.OverlayScroll), max(0, len(body)-capacity))
		end := min(len(body), start+capacity)
		if panel.Body.Height > 1 {
			body = append(append([]string(nil), body[start:end]...), fmt.Sprintf("View %d-%d/%d · ↑/↓", start+1, end, len(body)))
		} else {
			title = fmt.Sprintf("%s · %d/%d", title, start+1, len(body))
			body = append([]string(nil), body[start:end]...)
		}
	}
	action := ""
	if !panel.Actions.Empty() {
		action = layoutActionBar(actions, m.Focus == FocusActions, m.ActionIndex, panel.Actions.Width).Text
	}
	return panel, renderPanel(panel, title, true, body, action, glyphs)
}

func (m Model) navigationLabel(item navigationItem) string {
	attention := m.attentionCounts()
	switch item.View {
	case ViewLibrary:
		return fmt.Sprintf("%s  %d", item.Label, len(m.Snapshot.Config.Library.Skills))
	case ViewMigration:
		if attention.migration > 0 {
			return item.Label + "  !"
		}
	case ViewStatus:
		return fmt.Sprintf("%s  %d", item.Label, attention.status)
	}
	return item.Label
}

func (m Model) renderAppBar(width int) string {
	context := viewLabel(m.ActiveView) + m.scopeBreadcrumb()
	if m.Detail {
		rows := m.rows()
		if m.Cursor >= 0 && m.Cursor < len(rows) {
			context += " / " + rows[m.Cursor].Name
		}
	}
	if m.Detail || m.Scope.Level != "" {
		context = "‹ " + context
	}
	bar := uiTheme.appTitle.Render("aikit") + uiTheme.muted.Render("  ·  "+context)
	if m.Inventory.Loading {
		bar += uiTheme.muted.Render("  ·  scanning")
	} else if m.Busy || m.MutationBusy {
		bar += uiTheme.warning.Render("  ·  busy")
	}
	return clip(bar, width)
}

func (m Model) renderMain(layout Layout) []string {
	if overlay := m.overlayLines(); len(overlay) > 0 {
		panelLayout := layoutOverlayPanel(layout, m.overlayPanelActions(), m.Focus == FocusActions, m.ActionIndex)
		panel := panelLayout.Panel
		if panel.Empty() {
			return nil
		}
		title, body := overlay[0], overlay[1:]
		body = wrapOverlayBody(body, panelLayout.Body.Width)
		bodyTotal := len(body)
		bodyCapacity := panelLayout.Body.Height
		if m.scrollableOverlayBody() && len(body) > bodyCapacity {
			bodyCapacity = overlayBodyCapacity(layout, len(body))
			start := min(max(0, m.OverlayScroll), max(0, len(body)-bodyCapacity))
			end := min(len(body), start+bodyCapacity)
			body = append(append([]string(nil), body[start:end]...), fmt.Sprintf("View %d-%d/%d · ↑/↓", start+1, end, bodyTotal))
		} else if len(body) > bodyCapacity {
			body = body[:bodyCapacity]
		}
		result := make([]string, layout.Main.Height)
		relativeX := max(0, panel.X-layout.Main.X)
		relativeY := max(0, panel.Y-layout.Main.Y)
		if relativeY < len(result) {
			result[relativeY] = strings.Repeat(" ", relativeX) + clip(title, panelLayout.Title.Width)
		}
		for i, line := range body {
			if y := panelLayout.Body.Y - layout.Main.Y + i; y >= 0 && y < len(result) {
				result[y] = strings.Repeat(" ", relativeX) + clip(line, panelLayout.Body.Width)
			}
		}
		if !panelLayout.Actions.Empty() {
			y := panelLayout.Actions.Y - layout.Main.Y
			if y >= 0 && y < len(result) {
				result[y] = strings.Repeat(" ", relativeX) + panelLayout.ActionBar.Text
			}
		}
		return result
	}
	if m.ActiveView == ViewOverview && m.Mode == ModeTable {
		return m.renderOverview(layout.Main.Width, layout.Main.Height)
	}
	rows := m.rows()
	if layout.Detail.Empty() {
		if m.Detail {
			lines := []string{activeStyle.Render(m.detailTitle())}
			limit := max(0, layout.List.Height-2)
			detail := m.detailLinesForArea(layout.List.Width, limit)
			start := min(max(0, m.DetailScroll), max(0, len(detail)-limit))
			end := min(len(detail), start+limit)
			lines = append(lines, detail[start:end]...)
			for len(lines) < max(1, layout.List.Height-1) {
				lines = append(lines, "")
			}
			if len(m.primaryActions()) > 0 && layout.List.Height > 1 {
				lines = append(lines, m.renderActionBar(layout.List.Width))
			}
			return lines
		}
		geometry := m.visibleRowsLayout(layout)
		lines := []string{mutedStyle.Render(rangeTitle(m.collectionTitle(), geometry.Start, geometry.End, len(rows)))}
		if m.hasPinnedDetail() {
			limit := max(0, layout.List.Height-2)
			detail := m.detailLinesForArea(layout.List.Width, limit)
			if len(detail) > limit {
				detail = detail[:limit]
			}
			lines = append(lines, detail...)
			for len(lines) < max(1, layout.List.Height-1) {
				lines = append(lines, "")
			}
			if len(m.primaryActions()) > 0 && layout.List.Height > 1 {
				lines = append(lines, m.renderActionBar(layout.List.Width))
			}
			return lines
		}
		for i := geometry.Start; i < geometry.End; i++ {
			lines = append(lines, m.renderRowLines(rows[i], i == m.Cursor && m.Focus == FocusList, layout.List.Width)...)
		}
		if len(rows) == 0 && len(lines) < layout.List.Height-1 {
			lines = append(lines, mutedStyle.Render(m.emptyState()))
		}
		for len(lines) < max(1, layout.List.Height-1) {
			lines = append(lines, "")
		}
		if len(m.primaryActions()) > 0 && layout.List.Height > 1 {
			lines = append(lines, m.renderActionBar(layout.List.Width))
		}
		return lines
	}
	capacity := max(0, layout.List.Height-1)
	start, end := VisibleRange(len(rows), m.Cursor, m.Scroll, capacity)
	listWidth, detailWidth := layout.List.Width, layout.Detail.Width
	lines := []string{joinColumns(mutedStyle.Render(rangeTitle(m.collectionTitle(), start, end, len(rows))), activeStyle.Render(m.detailTitle()), listWidth, detailWidth)}
	detail := m.detailLinesForArea(detailWidth, max(0, capacity-1))
	for i := 0; i < capacity; i++ {
		left := ""
		if start+i < end {
			current := rows[start+i]
			if m.ActiveView == ViewMigration && current.Origin != "" {
				current.Name = current.Origin + " · " + current.Name
			}
			left = m.renderRow(current, start+i == m.Cursor && m.Focus == FocusList, listWidth)
		} else if i == 0 && len(rows) == 0 {
			left = mutedStyle.Render(m.emptyState())
		}
		right := ""
		if i == capacity-1 && len(m.primaryActions()) > 0 {
			right = m.renderActionBar(detailWidth)
		} else if detailIndex := m.DetailScroll + i; detailIndex < len(detail) {
			right = detail[detailIndex]
		}
		lines = append(lines, joinColumns(left, right, listWidth, detailWidth))
	}
	return lines
}

func (m Model) scrollableOverlayBody() bool {
	return m.Help || m.Mode == ModeConfirm || m.Mode == ModeErrorDetail
}

func (m Model) hasPinnedDetail() bool {
	return (m.Mode == ModeUpdates && (len(m.UpdateWarnings) > 0 || len(m.UpdateFailures) > 0)) ||
		(m.ActiveView == ViewLibrary && (len(m.BatchResult.Items) > 0 || len(m.BatchResult.Issues) > 0)) ||
		(m.ActiveView == ViewWorkspaces && projectResultVisible(m.ProjectResult)) ||
		len(m.RecoveryResult.Completed) > 0 || len(m.RecoveryResult.Issues) > 0
}

func (m Model) renderOverview(width, height int) []string {
	metrics := m.overviewMetricLines(width)
	rows := m.rows()
	if height <= 3 {
		lines := []string{clipPlain(fmt.Sprintf("Skills %d · Issues %d", len(m.Snapshot.Config.Library.Skills), m.attentionCounts().status), width)}
		if len(rows) == 0 {
			return append(lines, "Attention · [OK] All clear")
		}
		lines = append(lines, "Attention", m.renderRow(rows[min(m.Cursor, len(rows)-1)], m.Focus == FocusList, width))
		return lines[:min(len(lines), height)]
	}
	tiny := height <= 5
	lines := make([]string, 0, len(metrics)+4)
	if !tiny {
		lines = append(lines, uiTheme.panelTitle.Render("Overview"))
	}
	lines = append(lines, metrics...)
	if !tiny && height >= len(metrics)+4 {
		lines = append(lines, "")
	}
	heading := "Needs attention"
	if tiny {
		heading = "Attention"
	}
	if len(rows) == 0 {
		return append(lines, uiTheme.panelTitle.Render(m.filteredTitle(heading)), uiTheme.success.Render("[OK] All clear · No action needed"))
	}
	layout := Layout{Main: Rect{Width: width, Height: height}}
	geometry := m.visibleRowsLayout(layout)
	lines = append(lines, uiTheme.panelTitle.Render(rangeTitle(m.filteredTitle(heading), geometry.Start, geometry.End, len(rows))))
	for i := geometry.Start; i < geometry.End; i++ {
		rowLines := m.renderRowLines(rows[i], i == m.Cursor && m.Focus == FocusList, width)
		lines = append(lines, rowLines[0])
		if !tiny {
			lines = append(lines, rowLines[1])
		}
	}
	return lines
}

func (m Model) overviewMetricLines(width int) []string {
	summary := m.attentionCounts()
	updates := "Not checked"
	if m.Snapshot.Updates.Results != nil || m.Snapshot.Updates.Warnings != nil {
		updates = fmt.Sprintf("%d", summary.updates)
	} else if width < 30 {
		updates = "—"
	}
	unmanaged := 0
	for _, item := range m.Inventory.Items {
		state := item.State
		if state == "" {
			state = item.ManagementState
		}
		if state == app.ScanStateUnmanaged {
			unmanaged++
		}
	}
	return packLineSegments(width, []string{
		fmt.Sprintf("Skills  %d", len(m.Snapshot.Config.Library.Skills)),
		"Updates  " + updates,
		fmt.Sprintf("Unmanaged  %d", unmanaged),
		fmt.Sprintf("Issues  %d", summary.status),
	})
}

func (m Model) renderRow(current row, active bool, width int) string {
	return m.renderRowLines(current, active, width)[0]
}

func (m Model) renderRowLines(current row, active bool, width int) []string {
	mark := "  "
	if m.selectionRendered() {
		mark = "  [ ] "
		if m.rowSelected(current) {
			mark = "  [x] "
		}
	}
	if active {
		if m.selectionRendered() {
			mark = ">" + mark[1:]
		} else {
			mark = "> "
		}
	}
	stateWidth := min(24, max(12, width*2/5))
	if m.Mode == ModeScopePicker || m.Mode == ModePresetPicker {
		stateWidth = min(8, max(1, width/5))
	}
	nameWidth := max(1, width-stateWidth-len([]rune(mark))-1)
	name := clipPlain(current.Name, nameWidth)
	namePadding := max(0, nameWidth-lipgloss.Width(name))
	line := mark + name + strings.Repeat(" ", namePadding) + " " + renderStateBadge(current.State, current.Severity, stateWidth)
	if active {
		line = selectStyle.Render(clip(line, width))
	} else {
		line = clip(line, width)
	}
	context := m.rowContext(current)
	context = strings.Join(strings.Fields(context), " ")
	indent := "    "
	if m.selectionRendered() {
		indent = "      "
	}
	return []string{line, clip(mutedStyle.Render(indent+context), width)}
}

func renderStateBadge(state string, severity rowSeverity, width int) string {
	if severity == rowSeverityInfo {
		return mutedStyle.Render(clipPlain(state, width))
	}
	marker := "INFO"
	style := uiTheme.badge
	switch severity {
	case rowSeveritySuccess:
		marker, style = "OK", uiTheme.success
	case rowSeverityWarning, rowSeverityConflict:
		marker, style = "WARN", uiTheme.warning
	case rowSeverityError, rowSeverityRecovery:
		marker, style = "ERR", uiTheme.error
	}
	plain := clipPlain("["+marker+"] "+state, width)
	return style.Render(plain)
}

func (m Model) rowContext(current row) string {
	switch m.ActiveView {
	case ViewLibrary:
		return firstNonEmpty(current.Source, current.Detail, "Library skill")
	case ViewWorkspaces:
		return firstNonEmpty(current.Source, current.Detail, current.State)
	case ViewPresets:
		return firstNonEmpty(current.Detail, current.State)
	case ViewMigration:
		parts := make([]string, 0, 4)
		for _, value := range []string{current.Origin, current.Detail, current.Action, current.Target} {
			if value != "" && !slices.Contains(parts, value) {
				parts = append(parts, value)
			}
		}
		return firstNonEmpty(strings.Join(parts, " · "), "Review in Migration")
	case ViewStatus:
		return firstNonEmpty(current.Source, current.Detail, "Review status")
	default:
		return firstNonEmpty(current.Detail, current.Source, current.State)
	}
}

func (m Model) detailLines() []string {
	if m.ActiveView == ViewOverview {
		lines := []string{fmt.Sprintf("Workspace needs attention: %d", len(m.attentionRows())), "Startup scans local paths only."}
		for _, issue := range m.Inventory.Issues {
			lines = append(lines, "Issue: "+issue.Message)
			if len(lines) >= 4 {
				break
			}
		}
		return lines
	}
	if m.Mode == ModeUpdates && (len(m.UpdateWarnings) > 0 || len(m.UpdateFailures) > 0) {
		lines := []string{"Update check problems"}
		for _, warning := range m.UpdateWarnings {
			lines = append(lines, "Warning: "+warning)
		}
		for _, failed := range m.UpdateFailures {
			lines = append(lines, "Failed: "+failed.SkillID, "Error: "+firstNonEmpty(failed.Error, "unknown error"))
		}
		return lines
	}
	if m.ActiveView == ViewLibrary && (len(m.BatchResult.Items) > 0 || len(m.BatchResult.Issues) > 0) {
		lines := []string{"Last library batch"}
		for _, item := range m.BatchResult.Items {
			state := "Attempted"
			if item.Changed && m.BatchResult.Changed {
				state = "Changed"
				if m.Err != "" {
					state = "Changed before failure"
				}
			}
			if item.Issue != nil {
				state = "Issue: " + item.Issue.Message
				if item.Issue.Err != nil && item.Issue.Err.Error() != item.Issue.Message {
					state += " · " + item.Issue.Err.Error()
				}
			}
			lines = append(lines, item.Item+": "+state)
		}
		for _, issue := range m.BatchResult.Issues {
			line := firstNonEmpty(issue.Item, issue.Operation) + " · " + firstNonEmpty(issue.Path, "no path") + " · " + issue.Message
			if issue.Err != nil && issue.Err.Error() != issue.Message {
				lines = append(lines, line, "Error: "+issue.Err.Error())
				continue
			}
			lines = append(lines, line)
		}
		return lines
	}
	if m.ActiveView == ViewWorkspaces && projectResultVisible(m.ProjectResult) {
		lines := []string{"Last project operation"}
		for _, warning := range m.ProjectResult.Warnings {
			lines = append(lines, "Warning: "+warning)
		}
		for _, action := range m.ProjectResult.Plan.Actions {
			lines = append(lines, "Planned: "+action.Path)
		}
		for _, issue := range append(append([]link.Issue(nil), m.ProjectResult.Plan.Issues...), m.ProjectResult.Plan.Warnings...) {
			lines = append(lines, firstNonEmpty(issue.Message, string(issue.Kind)), "Path: "+firstNonEmpty(issue.Path, "no path"))
			if issue.Err != nil && issue.Err.Error() != issue.Message {
				lines = append(lines, "Error: "+issue.Err.Error())
			}
		}
		for _, action := range m.ProjectResult.Link.Actions {
			lines = append(lines, "Result: "+action.Path)
		}
		for _, issue := range append(append(append([]link.Issue(nil), m.ProjectResult.Link.Failures...), m.ProjectResult.Link.Issues...), m.ProjectResult.Link.Warnings...) {
			line := firstNonEmpty(issue.Message, string(issue.Kind))
			if issue.Err != nil && issue.Err.Error() != issue.Message {
				lines = append(lines, line, "Path: "+firstNonEmpty(issue.Path, "no path"), "Error: "+issue.Err.Error())
				continue
			}
			lines = append(lines, line, "Path: "+firstNonEmpty(issue.Path, "no path"))
		}
		return lines
	}
	if len(m.RecoveryResult.Completed) > 0 || len(m.RecoveryResult.Issues) > 0 {
		lines := []string{"Last recovery result"}
		for _, id := range m.RecoveryResult.Completed {
			lines = append(lines, "Completed: "+id)
		}
		for _, issue := range m.RecoveryResult.Issues {
			lines = append(lines, "Issue: "+issue.Message+" · "+firstNonEmpty(issue.Path, "no path"))
			if issue.Err != nil && issue.Err.Error() != issue.Message {
				lines = append(lines, "Error: "+issue.Err.Error())
			}
		}
		return lines
	}
	if m.ActiveView == ViewMigration && m.Detail && (m.Compare.Left.Skill.ID != "" || m.Compare.Right.Skill.ID != "") {
		lines := []string{"Compare: " + m.Compare.Left.Skill.ID + " ↔ " + m.Compare.Right.Skill.ID}
		if m.Compare.Equal {
			lines = append(lines, "Content is equal")
		}
		for _, diff := range m.Compare.Metadata {
			lines = append(lines, diff.Field+": "+diff.Left+" → "+diff.Right)
		}
		for _, diff := range m.Compare.Files {
			lines = append(lines, string(diff.Kind)+": "+diff.Path)
		}
		return lines
	}
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		if m.Inventory.Loading && m.ActiveView == ViewMigration {
			return []string{"Scanning registered roots..."}
		}
		return []string{"Select an item to see details."}
	}
	current := rows[m.Cursor]
	if m.ActiveView == ViewLibrary && m.Detail && m.SkillDetail.Skill.ID == current.ID {
		lines := []string{activeStyle.Render("Summary"), m.SkillDetail.Skill.Name, "Source: " + firstNonEmpty(m.SkillDetail.Skill.Source, "local")}
		if m.SkillDetail.Skill.Ref != nil {
			lines = append(lines, "Ref: "+m.SkillDetail.Skill.Ref.Kind+":"+m.SkillDetail.Skill.Ref.Value)
		}
		if len(m.SkillDetail.EnabledLocations) > 0 {
			lines = append(lines, activeStyle.Render("Usage"))
		}
		for _, scope := range m.SkillDetail.EnabledLocations {
			location := "Global / " + firstNonEmpty(scope.Agent, "common")
			if scope.Project != "" {
				location = "Project / " + scope.Project + " / " + firstNonEmpty(scope.Agent, "common")
			}
			lines = append(lines, "Enabled: "+location)
		}
		if len(m.SkillDetail.Files) > 0 || m.SkillDetail.SkillMD != "" || m.SkillDetail.SkillMDTruncated {
			lines = append(lines, activeStyle.Render("Files"))
		}
		for _, file := range m.SkillDetail.Files {
			lines = append(lines, "File: "+file.Path)
		}
		if m.SkillDetail.SkillMD != "" {
			lines = append(lines, strings.Split(m.SkillDetail.SkillMD, "\n")...)
		}
		if m.SkillDetail.SkillMDTruncated {
			lines = append(lines, "(SKILL.md preview truncated)")
		}
		if m.Err != "" {
			lines = append(lines, activeStyle.Render("Diagnostics"), m.Err)
		}
		return lines
	}
	lines := []string{activeStyle.Render("Summary"), current.Name, "State: " + current.State}
	if current.Action != "" {
		lines = append(lines, "Action: "+current.Action)
	}
	if current.Source != "" {
		lines = append(lines, activeStyle.Render("Usage"), current.Source)
	}
	if current.Detail != "" || m.Err != "" {
		lines = append(lines, activeStyle.Render("Diagnostics"))
		if current.Detail != "" {
			lines = append(lines, current.Detail)
		}
		if m.Err != "" {
			lines = append(lines, m.Err)
		}
	}
	return lines
}

func (m Model) detailLinesForWidth(width int) []string {
	width = max(1, width)
	lines := m.detailLines()
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.ContainsAny(line, "\r\n") && lipgloss.Width(line) <= width {
			wrapped = append(wrapped, line)
			continue
		}
		wrapped = append(wrapped, wrapText(stripANSI(line), width)...)
	}
	return wrapped
}

func (m Model) detailLinesForArea(width, budget int) []string {
	rows := m.rows()
	if m.ActiveView == ViewLibrary && m.Detail && m.Cursor >= 0 && m.Cursor < len(rows) && m.SkillDetail.Skill.ID == rows[m.Cursor].ID {
		contentBudget := budget
		if m.Err != "" && budget >= 2 {
			contentBudget -= 2
		}
		lines := compactSkillDetailLines(m.SkillDetail, width, contentBudget)
		if m.Err != "" && budget >= 2 {
			lines = append(lines, clip(activeStyle.Render("Diagnostics"), width), clip(m.Err, width))
		}
		return lines
	}
	return m.detailLinesForWidth(width)
}

func compactSkillDetailLines(detail app.SkillDetail, width, budget int) []string {
	if budget <= 0 {
		return nil
	}
	width = max(1, width)
	line := func(value string) string { return clip(value, width) }
	summaryLines := []string{
		line(activeStyle.Render("Summary")),
		line(detail.Skill.Name),
		line("Source: " + firstNonEmpty(detail.Skill.Source, "local")),
	}
	if detail.Skill.Ref != nil {
		summaryLines = append(summaryLines, line("Ref: "+detail.Skill.Ref.Kind+":"+detail.Skill.Ref.Value))
	}
	lines := append([]string(nil), summaryLines...)
	if len(detail.EnabledLocations) > 0 {
		lines = append(lines, line(activeStyle.Render("Usage")))
		location := "Global / " + firstNonEmpty(detail.EnabledLocations[0].Agent, "common")
		if detail.EnabledLocations[0].Project != "" {
			location = "Project / " + detail.EnabledLocations[0].Project + " / " + firstNonEmpty(detail.EnabledLocations[0].Agent, "common")
		}
		lines = append(lines, line("Enabled: "+location))
		if omitted := len(detail.EnabledLocations) - 1; omitted > 0 {
			lines = append(lines, line(fmt.Sprintf("… +%d locations", omitted)))
		}
	}
	if len(detail.Files) > 0 {
		lines = append(lines, line(activeStyle.Render("Files")), line("File: "+detail.Files[0].Path))
		if omitted := len(detail.Files) - 1; omitted > 0 {
			lines = append(lines, line(fmt.Sprintf("… +%d files", omitted)))
		}
	}
	sourceLines := strings.Split(detail.SkillMD, "\n")
	if detail.SkillMD != "" || detail.SkillMDTruncated {
		lines = append(lines, line(activeStyle.Render("Content preview")))
		previewLimit := min(4, len(sourceLines))
		for index := 0; index < previewLimit; index++ {
			lines = append(lines, line(sourceLines[index]))
		}
		omitted := max(0, len(sourceLines)-previewLimit)
		marker := ""
		if omitted > 0 {
			marker = fmt.Sprintf("… +%d lines", omitted)
		}
		if detail.SkillMDTruncated {
			if marker != "" {
				marker += " · "
			}
			marker += "64K cap"
		}
		if marker != "" {
			lines = append(lines, line(marker))
		}
	}
	if len(lines) > budget {
		tail := compactSkillOmissionLines(detail, width)
		maximumTail := budget
		if budget > 3 {
			maximumTail = budget - 1
		}
		if len(tail) > maximumTail {
			tail = tail[len(tail)-maximumTail:]
		}
		prefixBudget := budget - len(tail)
		lines = append(append([]string(nil), summaryLines[:min(prefixBudget, len(summaryLines))]...), tail...)
	}
	return lines
}

func compactSkillOmissionLines(detail app.SkillDetail, width int) []string {
	line := func(value string) string { return clip(value, max(1, width)) }
	markers := make([]string, 0, 4)
	locations, files := len(detail.EnabledLocations), len(detail.Files)
	if locations > 0 && files > 0 {
		marker := fmt.Sprintf("… %d locations · %d files", locations, files)
		if lipgloss.Width(marker) > width {
			marker = fmt.Sprintf("… %d locations %d files", locations, files)
		}
		markers = append(markers, line(marker))
	} else if locations > 0 {
		markers = append(markers, line(fmt.Sprintf("… %d locations", locations)))
	} else if files > 0 {
		markers = append(markers, line(fmt.Sprintf("… %d files", files)))
	}
	if detail.SkillMD != "" || detail.SkillMDTruncated {
		count := 0
		if detail.SkillMD != "" {
			count = len(strings.Split(detail.SkillMD, "\n"))
		}
		marker := fmt.Sprintf("… %d lines", count)
		if detail.SkillMDTruncated {
			marker += " · 64K cap"
		}
		markers = append(markers, line(marker))
	}
	return append(markers, line("… View SKILL.md"))
}

func projectResultVisible(result app.Result) bool {
	return result.Changed || len(result.Warnings) > 0 || len(result.Plan.Actions) > 0 || len(result.Plan.Issues) > 0 || len(result.Plan.Warnings) > 0 || len(result.Link.Actions) > 0 || len(result.Link.Failures) > 0 || len(result.Link.Issues) > 0 || len(result.Link.Warnings) > 0
}

func (m Model) renderActionBar(width int) string {
	return layoutActionBar(m.primaryActions(), m.Focus == FocusActions, m.ActionIndex, width).Text
}

func (m Model) detailTitle() string {
	if m.ActiveView == ViewOverview {
		return "Attention"
	}
	return "Details"
}

func (m Model) collectionTitle() string {
	label := "Items"
	switch m.ActiveView {
	case ViewOverview:
		label = "Workspace summary"
	case ViewLibrary:
		label = "Library skills"
	case ViewWorkspaces:
		label = "Workspace scopes"
	case ViewPresets:
		label = "Presets"
	case ViewMigration:
		label = "Discovered skills"
	case ViewStatus:
		label = "Issues"
	}
	return m.filteredTitle(label)
}

func (m Model) filteredTitle(label string) string {
	query := strings.TrimSpace(m.Filter)
	if query == "" {
		return label
	}
	count := len(m.rows())
	noun := "results"
	if count == 1 {
		noun = "result"
	}
	return fmt.Sprintf("%s · Filter: %s · %d %s", label, query, count, noun)
}

func rangeTitle(label string, start, end, total int) string {
	if total <= 0 || end <= start || (start == 0 && end == total) {
		return label
	}
	if end-start == 1 {
		return fmt.Sprintf("%s · %d/%d", label, start+1, total)
	}
	return fmt.Sprintf("%s · %d-%d/%d", label, start+1, end, total)
}

func packLineSegments(width int, segments []string) []string {
	width = max(1, width)
	lines := make([]string, 0, len(segments))
	current := ""
	for _, segment := range segments {
		candidate := segment
		if current != "" {
			candidate = current + "    " + segment
		}
		if current != "" && lipgloss.Width(candidate) > width {
			lines = append(lines, current)
			current = segment
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func (m Model) emptyState() string {
	if strings.TrimSpace(m.Filter) != "" {
		return "No matches · Press / to edit or clear the filter"
	}
	if m.Mode == ModeUpdates {
		if len(m.UpdateWarnings) > 0 || len(m.UpdateFailures) > 0 {
			return "No selectable updates · Review problems on the right"
		}
		return "No updates available · Press Esc to return"
	}
	switch m.ActiveView {
	case ViewLibrary:
		return "No library skills · Choose Add source to get started"
	case ViewMigration:
		return "No migration items · Press r to rescan local skills"
	case ViewStatus:
		return "No issues · Everything is in sync"
	case ViewPresets:
		return "No presets · Choose Create to add one"
	case ViewWorkspaces:
		return "No workspace items in this scope · Press Esc to return"
	default:
		return "No items"
	}
}

func (m Model) primaryActions() []string {
	if m.Mode == ModeWorkspaceSkills {
		return []string{"Enable selected", "Disable selected", "Cancel"}
	}
	if m.Mode == ModeScopePicker || m.Mode == ModePresetPicker {
		return []string{"Apply", "Cancel"}
	}
	if m.Mode == ModeProjectAgents {
		return []string{"Save agents", "Cancel"}
	}
	if m.Mode == ModeErrorDetail {
		return []string{"Close"}
	}
	if m.Mode == ModeConfiguration {
		return []string{"Validate", "Reload", "Show paths", "Close"}
	}
	if m.Mode == ModeConfirm {
		return []string{"Confirm", "Cancel"}
	}
	if m.Mode == ModeMore {
		switch m.ActiveView {
		case ViewLibrary:
			actions := []string{"State filter", "Source filter", "Check updates"}
			selected := len(m.selectedLibraryIDs())
			if selected > 0 {
				actions = append(actions, "Enable selected", "Disable selected")
				if _, err := m.libraryBatchRequest(app.BatchUpdate); err == nil {
					actions = append(actions, "Update selected")
				}
				actions = append(actions, "Remove selected")
			}
			if selected == 1 {
				actions = append(actions, "Change ref")
			}
			if selected > 0 {
				actions = append(actions, "Clear selection")
			}
			return append(actions, "Close")
		case ViewWorkspaces:
			if m.Scope.Level == "workspace-projects" || m.Scope.Level == "project-skills" {
				return []string{"Create project", m.workspacePresetAction(), "Rename project", "Manage agents", "Change project directory", "Remove project", "Close"}
			}
			if m.Scope.Level == "project-targets" {
				return []string{m.workspacePresetAction(), "Rename project", "Manage agents", "Change project directory", "Remove project", "Close"}
			}
			if m.Scope.Level == "workspace-global" {
				actions := []string{m.workspacePresetAction()}
				if len(m.selectedWorkspaceSkillIDs()) > 0 {
					actions = append(actions, "Enable selected", "Disable selected", "Clear selection")
				}
				return append(actions, "Sync preview", "Close")
			}
			if m.Scope.Level == "agent-skills" {
				return []string{m.workspacePresetAction(), "Sync preview", "Close"}
			}
			return []string{"Sync preview", "Close"}
		case ViewPresets:
			if m.Scope.Level == "preset-skills" {
				return []string{"Close"}
			}
			return []string{"Create preset", "Duplicate", "Rename", "Apply", "Delete", "Close"}
		case ViewStatus:
			actions := []string{"Refresh"}
			if m.selectedStatusCanSync() {
				actions = append(actions, "Retry")
			}
			return append(actions, "Error details", "Close")
		}
	}
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		if m.ActiveView == ViewPresets && m.Scope.Level == "" {
			return []string{"Create preset"}
		}
		if m.ActiveView == ViewLibrary {
			return []string{"Add source", "More"}
		}
		if m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-projects" {
			return []string{"Create project"}
		}
		if m.ActiveView == ViewMigration {
			return []string{"Refresh"}
		}
		return nil
	}
	current := rows[m.Cursor]
	switch m.ActiveView {
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
			return []string{"View SKILL.md", "Add source", "More"}
		}
		return []string{"Open", "Add source", "More"}
	case ViewWorkspaces:
		if m.Scope.Level == "workspace-projects" {
			return []string{"Add skill", m.workspacePresetAction(), "More"}
		}
		if m.Scope.Level == "workspace-agents" {
			return []string{"Open", m.workspacePresetAction(), "More"}
		}
		if m.Scope.Level == "agent-skills" || m.Scope.Level == "project-skills" {
			if current.Direct {
				return []string{"Disable", "Select skills", "More"}
			}
			if current.Enabled {
				return []string{"Manage source", "Select skills", "More"}
			}
			return []string{"Enable", "Select skills", "More"}
		}
		if m.Scope.Level == "project-targets" {
			if project, ok := findProject(m.Snapshot.Config.Projects, m.Scope.Project); ok && len(project.Agents) > 0 {
				return []string{"Open", m.workspacePresetAction(), "More"}
			}
			return []string{"Open", "Manage agents", "More"}
		}
		if m.Scope.Level == "workspace-global" {
			return []string{"Open", m.workspacePresetAction(), "More"}
		}
		return []string{"Open", "More"}
	case ViewPresets:
		if m.Scope.Level == "preset-skills" {
			return []string{"Open", "Save", "More"}
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
	case ViewOverview:
		return []string{"Open"}
	default:
		return nil
	}
}

func (m Model) workspacePresetAction() string {
	if len(m.Snapshot.Config.Presets) == 0 {
		return "Create preset"
	}
	return "Apply preset"
}

func (m Model) renderStatus(width int) string {
	if m.Err != "" {
		return clip(errorStyle.Render("Error: "+m.Err), width)
	}
	if len(m.Inventory.Issues) > 0 {
		return clip(errorStyle.Render("Issue: "+m.Inventory.Issues[0].Message), width)
	}
	if m.Inventory.Loading {
		if m.Inventory.Total == 0 {
			return clip(mutedStyle.Render("Scanning local inventory..."), width)
		}
		return clip(mutedStyle.Render(fmt.Sprintf("Scanning local inventory %d/%d", m.Inventory.Completed, m.Inventory.Total)), width)
	}
	return clip(mutedStyle.Render(m.Status), width)
}

func (m Model) footer() string {
	if m.Mode == ModeConfirm {
		if m.Width < 30 {
			return "Enter OK · Esc Cancel"
		}
		return "Enter Confirm   Esc Cancel"
	}
	if m.Mode == ModeFilter || m.Mode == ModeInput {
		if m.Width < 30 {
			return "Enter Apply · Esc Cancel"
		}
		return "Enter Apply   Esc Cancel"
	}
	if m.Help {
		return "↑/↓ Scroll   Esc Close"
	}
	if m.Mode == ModeConfiguration || m.Mode == ModeMore || m.Mode == ModeErrorDetail {
		return "Esc Close"
	}
	if m.Detail && m.Width < 60 {
		return "Esc Back"
	}
	if m.Focus == FocusActions {
		actions := m.currentActions()
		if len(actions) > 0 {
			index := min(max(0, m.ActionIndex), len(actions)-1)
			if m.Width < 30 {
				return "Enter " + actions[index]
			}
			return "Enter " + actions[index] + "   ←/→ Actions   Esc List"
		}
	}
	if m.ActiveView == ViewMigration {
		if m.Width < 30 {
			return "Space · Enter · ? · Ctrl+Q"
		}
		if m.Width < 60 {
			return "Space · Enter · ? Help · Ctrl+Q Quit"
		}
		return "Space Select   Enter Review   r Refresh   ? Help   Ctrl+Q Quit"
	}
	if m.Width < 30 {
		return "Enter · / · ? · Ctrl+Q"
	}
	if m.Width < 60 {
		return "Enter · / Find · ? Help · Ctrl+Q Quit"
	}
	return "Enter Open   / Search   r Refresh   ? Help   Ctrl+Q Quit"
}

func viewLabel(view View) string {
	switch view {
	case ViewOverview:
		return "Overview"
	case ViewLibrary:
		return "Library"
	case ViewWorkspaces:
		return "Workspaces"
	case ViewPresets:
		return "Presets"
	case ViewMigration:
		return "Migration"
	case ViewStatus:
		return "Status"
	default:
		return title(string(view))
	}
}

func (m Model) scopeBreadcrumb() string {
	if m.Scope.Project != "" {
		return " / " + m.Scope.Project
	}
	if m.Scope.Agent != "" {
		return " / " + m.Scope.Agent
	}
	if m.Scope.Preset != "" {
		return " / " + m.Scope.Preset
	}
	return ""
}

func (m Model) breadcrumb() string {
	value := "aikit / " + viewLabel(m.ActiveView) + m.scopeBreadcrumb()
	if !m.Detail {
		return value
	}
	rows := m.rows()
	if m.Cursor >= 0 && m.Cursor < len(rows) {
		value += " / " + rows[m.Cursor].Name
	}
	return value
}

func (m Model) selectionRendered() bool {
	return m.ActiveView == ViewLibrary || m.ActiveView == ViewMigration || (m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-global") || (m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills") || m.Mode == ModeScan || m.Mode == ModeUpdates || m.Mode == ModeAddSelect || m.Mode == ModeProjectAgents || m.Mode == ModeWorkspaceSkills || (m.Mode == ModeFilter && (m.filterParent == ModeScan || m.filterParent == ModeUpdates || m.filterParent == ModeAddSelect || m.filterParent == ModeProjectAgents || m.filterParent == ModeWorkspaceSkills))
}

func (m Model) rowSelected(current row) bool {
	if m.ActiveView == ViewPresets && m.Scope.Level == "preset-skills" {
		return m.Selected[current.ID]
	}
	return m.Selected[current.selectionKey()]
}

func joinColumns(left, right string, leftWidth, rightWidth int) string {
	left = clip(left, leftWidth)
	padding := max(0, leftWidth-lipgloss.Width(left))
	return left + strings.Repeat(" ", padding) + "│" + clip(right, rightWidth)
}

func joinRail(left, right string, leftWidth, rightWidth int) string {
	left = clip(left, leftWidth)
	padding := max(0, leftWidth-lipgloss.Width(left))
	return left + strings.Repeat(" ", padding) + " " + clip(right, rightWidth)
}
