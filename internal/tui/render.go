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
	lines := make([]string, layout.Height)
	if !layout.Header.Empty() {
		lines[layout.Header.Y] = clip(m.renderAppBar(layout.Header.Width), layout.Header.Width)
	}
	main := m.renderMain(layout)
	if layout.Narrow {
		if !layout.Breadcrumb.Empty() {
			lines[layout.Breadcrumb.Y] = clip("‹ "+m.breadcrumb(), layout.Breadcrumb.Width)
		}
		for i, line := range main {
			if y := layout.Main.Y + i; y < layout.Main.Bottom() && y < len(lines) {
				lines[y] = clip(line, layout.Main.Width)
			}
		}
	} else {
		navigation := make(map[int]string)
		for _, item := range layoutNavigation(layout, topViews, m.ActiveView) {
			item.Label = m.navigationLabel(item)
			label := "  " + item.Label
			if item.Active {
				label = uiTheme.focused(item.Label)
			} else {
				label = uiTheme.navigation.Render(label)
			}
			navigation[item.Rect.Y] = label
		}
		for y := layout.Navigation.Y; y < layout.Navigation.Bottom(); y++ {
			right := ""
			if i := y - layout.Main.Y; i >= 0 && i < len(main) {
				right = main[i]
			}
			lines[y] = joinRail(navigation[y], right, layout.Navigation.Width, layout.Main.Width)
		}
	}
	if !layout.Status.Empty() {
		lines[layout.Status.Y] = m.renderStatus(layout.Status.Width)
	}
	if !layout.Footer.Empty() {
		lines[layout.Footer.Y] = clip(m.footer(), layout.Footer.Width)
	}
	for i := range lines {
		lines[i] = clip(lines[i], layout.Width)
	}
	return strings.Join(lines, "\n")
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
	bar := uiTheme.appTitle.Render("aikit") + uiTheme.muted.Render("  ·  "+viewLabel(m.ActiveView))
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
			detail := m.detailLines()
			limit := max(0, layout.List.Height-2)
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
			detail := m.detailLines()
			limit := max(0, layout.List.Height-2)
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
	detail := m.detailLines()
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
		} else if i < len(detail) {
			right = detail[i]
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
	tiny := height <= 5
	lines := make([]string, 0, len(metrics)+4)
	if !tiny {
		lines = append(lines, uiTheme.panelTitle.Render("Overview"))
	}
	lines = append(lines, metrics...)
	if !tiny && height >= len(metrics)+4 {
		lines = append(lines, "")
	}
	rows := m.rows()
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
		if m.Selected[current.selectionKey()] {
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
		summary := m.attentionCounts()
		lines := []string{fmt.Sprintf("Workspace needs attention: %d", summary.migration+summary.status+summary.updates), "Startup scans local paths only."}
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
		return []string{"Select", "Cancel"}
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
			if m.Scope.Level == "workspace-projects" || m.Scope.Level == "project-targets" || m.Scope.Level == "project-skills" {
				return []string{"Apply preset", "Rename project", "Manage agents", "Change project directory", "Remove project", "Close"}
			}
			if m.Scope.Level == "workspace-global" {
				actions := []string{"Apply preset"}
				if len(m.selectedWorkspaceSkillIDs()) > 0 {
					actions = append(actions, "Enable selected", "Disable selected", "Clear selection")
				}
				return append(actions, "Sync preview", "Close")
			}
			if m.Scope.Level == "agent-skills" {
				return []string{"Apply preset", "Sync preview", "Close"}
			}
			return []string{"Sync preview", "Close"}
		case ViewPresets:
			if m.Scope.Level == "preset-skills" {
				return []string{"Close"}
			}
			return []string{"Duplicate", "Rename", "Apply", "Delete", "Close"}
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
			return []string{"Create"}
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
		return []string{"Open", "Add source", "More"}
	case ViewWorkspaces:
		if m.Scope.Level == "workspace-projects" {
			return []string{"Open", "Create project", "More"}
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
			return []string{"Open", "Manage agents", "More"}
		}
		if m.Scope.Level == "workspace-global" {
			return []string{"Open", "Apply preset", "More"}
		}
		return []string{"Open", "More"}
	case ViewPresets:
		if m.Scope.Level == "preset-skills" {
			return []string{"Open", "Save", "More"}
		}
		return []string{"Open", "Create", "More"}
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

func (m Model) renderStatus(width int) string {
	if m.Err != "" {
		return clip(errorStyle.Render("Error: "+m.Err), width)
	}
	if len(m.Inventory.Issues) > 0 {
		return clip(errorStyle.Render("Issue: "+m.Inventory.Issues[0].Message), width)
	}
	if m.Inventory.Loading {
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
	if m.ActiveView == ViewMigration {
		if m.Width < 30 {
			return "Space · Enter · ? · q"
		}
		if m.Width < 60 {
			return "Space Select · Enter · ? Help · q Quit"
		}
		return "Space Select   Enter Review   r Refresh   ? Help   q Quit"
	}
	if m.Width < 30 {
		return "Enter · / · ? · q"
	}
	if m.Width < 60 {
		return "Enter Open · / Find · ? Help · q Quit"
	}
	return "Enter Open   / Search   r Refresh   ? Help   q Quit"
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
	return m.ActiveView == ViewLibrary || m.ActiveView == ViewMigration || (m.ActiveView == ViewWorkspaces && m.Scope.Level == "workspace-global") || m.Mode == ModeScan || m.Mode == ModeUpdates || m.Mode == ModeAddSelect || m.Mode == ModeProjectAgents || m.Mode == ModeWorkspaceSkills || (m.Mode == ModeFilter && (m.filterParent == ModeScan || m.filterParent == ModeUpdates || m.filterParent == ModeAddSelect || m.filterParent == ModeProjectAgents || m.filterParent == ModeWorkspaceSkills))
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
