package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

func (m Model) overlayLines() []string {
	switch {
	case m.Help:
		return []string{
			activeStyle.Render("Keyboard help"),
			"1-6 pages · ↑/↓ or j/k move · Enter open",
			"Tab/Shift+Tab focus · ←/→ actions · space select",
			"/ search · Enter apply · Esc cancel · Backspace to clear draft",
			"PgUp/PgDn scroll details and long previews",
			"Mouse: click rows/actions · wheel scrolls the focused area",
			"r refresh · Ctrl+K configuration · ? help",
			"Esc back/cancel · q quit from a page · Ctrl+C force quit",
		}
	case m.Mode == ModeConfiguration:
		lines := []string{
			activeStyle.Render("Configuration"),
			"Config:  " + firstNonEmpty(m.Config.Config, "Loading..."),
			"Library: " + firstNonEmpty(m.Config.Library, "Loading..."),
			"Cache:   " + firstNonEmpty(m.Config.Cache, "Loading..."),
		}
		if m.ConfigValidation.Valid {
			lines = append(lines, "Configuration valid: "+m.ConfigValidation.Path)
		}
		return lines
	case m.Mode == ModeErrorDetail:
		lines := []string{activeStyle.Render("Error details")}
		return append(lines, wrapText(m.FullError, max(1, overlayActionBarWidth(m.Width, m.Height)))...)
	case m.Mode == ModeConfirm:
		lines := []string{
			activeStyle.Render("Confirm " + string(m.confirm)),
			firstNonEmpty(m.Preview.Summary, fmt.Sprintf("%d selected item(s). This action may change local files.", len(m.selectedIDs()))),
		}
		if m.confirm == ActionRecovery {
			lines[0] = activeStyle.Render("Review recovery")
			lines[1] = fmt.Sprintf("Resume %d exact pending operation(s).", len(m.RecoveryPreview.Operations))
			for _, item := range m.RecoveryPreview.Operations {
				group := firstNonEmpty(item.Operation.TransactionID, "ungrouped")
				canResume := "no"
				if item.CanResume {
					canResume = "yes"
				}
				lines = append(lines, "Group: "+group+" · ID: "+item.Operation.ID, "Path: "+item.Operation.Target, "Can resume: "+canResume)
			}
			for _, issue := range m.RecoveryPreview.Issues {
				lines = append(lines, "Issue: "+issue.Message+" · "+firstNonEmpty(issue.Path, "no path"))
				if issue.Err != nil && issue.Err.Error() != issue.Message {
					lines = append(lines, "Error: "+issue.Err.Error())
				}
			}
		}
		for _, reference := range m.Preview.References {
			lines = append(lines, "Reference: "+reference)
		}
		for _, scope := range m.Preview.AffectedScopes {
			label := "Global / " + firstNonEmpty(scope.Agent, "common")
			if scope.Project != "" {
				label = "Project / " + scope.Project + " / " + firstNonEmpty(scope.Agent, "common")
			}
			lines = append(lines, "Scope: "+label)
		}
		for _, warning := range m.Preview.Warnings {
			lines = append(lines, "Warning: "+warning)
		}
		for _, conflict := range m.Preview.Conflicts {
			lines = append(lines, "Conflict: "+conflict)
		}
		for _, action := range m.Preview.Plan.Actions {
			lines = append(lines, "Path: "+action.Path)
		}
		for _, warning := range m.Preview.Plan.Warnings {
			lines = append(lines, "Plan warning: "+warning.Message+" · "+firstNonEmpty(warning.Path, "no path"))
			if warning.Err != nil && warning.Err.Error() != warning.Message {
				lines = append(lines, "Error: "+warning.Err.Error())
			}
		}
		for _, issue := range m.Preview.Plan.Issues {
			lines = append(lines, "Plan issue: "+issue.Message+" · "+firstNonEmpty(issue.Path, "no path"))
			if issue.Err != nil && issue.Err.Error() != issue.Message {
				lines = append(lines, "Error: "+issue.Err.Error())
			}
		}
		for _, action := range m.PlanPreview.Plan.Actions {
			lines = append(lines, "Path: "+action.Path)
		}
		if m.confirm == ActionProjectEdit {
			for _, action := range m.ProjectPreview.Cleanup.Actions {
				lines = append(lines, "Cleanup: "+action.Path)
			}
			for _, issue := range m.ProjectPreview.Cleanup.Issues {
				lines = append(lines, "Cleanup issue: "+issue.Message+" · "+issue.Path)
			}
			for _, warning := range m.ProjectPreview.Cleanup.Warnings {
				lines = append(lines, "Cleanup warning: "+warning.Message+" · "+warning.Path)
			}
			for _, action := range m.ProjectPreview.Next.Actions {
				lines = append(lines, "Next: "+action.Path)
			}
			for _, issue := range m.ProjectPreview.Next.Issues {
				lines = append(lines, "Next issue: "+issue.Message+" · "+issue.Path)
			}
			for _, warning := range m.ProjectPreview.Next.Warnings {
				lines = append(lines, "Next warning: "+warning.Message+" · "+warning.Path)
			}
		}
		if m.confirm == ActionScan {
			for _, item := range m.Scan.Items {
				if !m.selectorPending(item.Key) {
					continue
				}
				libraryID := firstNonEmpty(item.MatchedLibraryID, item.Skill.ID)
				lines = append(lines, "Library: "+libraryID)
				if m.pendingScan.Adopt {
					binding := firstNonEmpty(item.Project, item.Scope.Project, "Global") + " / " + firstNonEmpty(item.Agent, item.Scope.Agent, "common")
					lines = append(lines, "Binding: "+binding, "Replace: "+item.Target)
				} else {
					lines = append(lines, "Source unchanged: "+item.Target)
				}
			}
		}
		if m.confirm == ActionForceRemove || m.confirm == ActionForceBatchRemove || m.confirm == ActionForcePresetDelete {
			lines = append(lines, "Force removal detaches every listed reference.")
		}
		return lines
	case m.Mode == ModeFilter:
		return []string{activeStyle.Render("Search: ") + m.FilterDraft + "█"}
	case m.Mode == ModeInput:
		return []string{activeStyle.Render(m.Input.Prompt), m.Input.Value + "█"}
	case m.Mode == ModeMore:
		lines := []string{activeStyle.Render("More actions")}
		actions := m.primaryActions()
		visible := layoutMoreActions(ComputeLayout(m.Width, m.Height), actions, m.ActionIndex, m.OverlayScroll)
		for i := visible.Start; i < visible.End; i++ {
			marker := "  "
			if i == m.ActionIndex {
				marker = "> "
			}
			lines = append(lines, fmt.Sprintf("%s[%d] %s", marker, i+1, actions[i]))
		}
		return lines
	default:
		return nil
	}
}

func overlayActionBarWidth(width, height int) int {
	return ComputeLayout(max(20, width), max(1, height)).Overlay.Width
}

func wrapText(value string, width int) []string {
	if value == "" {
		return []string{"No error is selected."}
	}
	width = max(1, width)
	var lines []string
	for paragraphIndex, paragraph := range strings.Split(value, "\n") {
		if paragraphIndex > 0 && paragraph == "" {
			lines = append(lines, "")
			continue
		}
		var line strings.Builder
		cells := 0
		graphemes := uniseg.NewGraphemes(paragraph)
		for graphemes.Next() {
			cluster := graphemes.Str()
			clusterWidth := lipgloss.Width(cluster)
			if cells > 0 && cells+clusterWidth > width {
				lines = append(lines, line.String())
				line.Reset()
				cells = 0
			}
			line.WriteString(cluster)
			cells += clusterWidth
			if cells >= width {
				lines = append(lines, line.String())
				line.Reset()
				cells = 0
			}
		}
		if line.Len() > 0 || paragraph == "" {
			lines = append(lines, line.String())
		}
	}
	return lines
}

func wrapOverlayBody(body []string, width int) []string {
	wrapped := make([]string, 0, len(body))
	for _, line := range body {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapText(line, width)...)
	}
	return wrapped
}

func (m Model) selectorPending(key string) bool {
	for _, selector := range m.pendingScan.Selectors {
		if selector.Key == key {
			return true
		}
	}
	return false
}
