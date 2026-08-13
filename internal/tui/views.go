package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/silenceper/aikit/internal/updatecheck"
)

var (
	activeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	selectStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

func (m Model) ViewString() string { return m.render() }
func (m Model) View() string       { return m.render() }

func (m Model) render() string {
	width := m.Width
	if width < 20 {
		width = 20
	}
	var lines []string
	lines = append(lines, m.header(width), strings.Repeat("─", width))
	if m.Help {
		lines = append(lines,
			activeStyle.Render("Keyboard help"),
			"1-5 views   j/k or ↑/↓ move   enter details",
			"space toggle/select   / filter   u updates",
			"S scan/adopt   s sync   esc back   q quit",
		)
	} else {
		lines = append(lines, m.table(width)...)
		if m.Mode == ModeFilter {
			lines = append(lines, activeStyle.Render("filter: ")+m.Filter+"█")
		}
		if m.Mode == ModeConfirm {
			lines = append(lines, selectStyle.Render(fmt.Sprintf("Confirm %s for %d selected item(s)? enter=yes esc=cancel", m.confirm, len(m.selectedIDs()))))
		}
		if m.Detail && m.Height >= 16 {
			lines = append(lines, strings.Repeat("─", width), m.detailLine())
		}
	}
	lines = append(lines, strings.Repeat("─", width))
	footer := m.footer()
	if m.Err != "" {
		lines = append(lines, errorStyle.Render("error: "+m.Err))
	} else if m.Status != "" {
		lines = append(lines, mutedStyle.Render(m.Status))
	}
	lines = append(lines, footer)
	for i := range lines {
		lines[i] = clip(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) header(width int) string {
	updates := 0
	for _, item := range m.Snapshot.Updates.Results {
		if item.State == updatecheck.StateUpdateAvailable {
			updates++
		}
	}
	if width < 55 {
		parts := []string{"aikit", tab(m.ActiveView == ViewLibrary, "1 lib"), tab(m.ActiveView == ViewAgents, "2 agt"), tab(m.ActiveView == ViewProjects, "3 prj"), tab(m.ActiveView == ViewPresets, "4 pre"), tab(m.ActiveView == ViewStatus, "5 sts")}
		if updates > 0 {
			parts = append(parts, fmt.Sprintf("↑%d", updates))
		}
		return strings.Join(parts, " ")
	}
	parts := []string{"aikit", tab(m.ActiveView == ViewLibrary, "1 library"), tab(m.ActiveView == ViewAgents, "2 agents"), tab(m.ActiveView == ViewProjects, "3 projects"), tab(m.ActiveView == ViewPresets, "4 presets"), tab(m.ActiveView == ViewStatus, "5 status")}
	if updates > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d updates", updates))
	}
	return strings.Join(parts, " │ ")
}

func tab(active bool, label string) string {
	if active {
		return activeStyle.Render(label)
	}
	return label
}

func (m Model) table(width int) []string {
	rows := m.rows()
	nameWidth := 20
	if width < 60 {
		nameWidth = 12
	}
	idWidth := width - nameWidth - 18
	if idWidth < 8 {
		idWidth = 8
	}
	lines := []string{mutedStyle.Render(fmt.Sprintf("  %-*s %-*s %s", nameWidth, "NAME", idWidth, "ID", "STATE"))}
	if len(rows) == 0 {
		return append(lines, mutedStyle.Render("  no items"))
	}
	maxRows := m.Height - 7
	if maxRows < 2 {
		maxRows = 2
	}
	start := 0
	if m.Cursor >= maxRows {
		start = m.Cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
	}
	for i := start; i < end; i++ {
		current := rows[i]
		cursor := " "
		if i == m.Cursor {
			cursor = "▸"
		}
		selected := " "
		if m.Selected[current.selectionKey()] {
			selected = "✓"
		}
		line := fmt.Sprintf("%s%s %-*s %-*s %s", cursor, selected, nameWidth, clipPlain(current.Name, nameWidth), idWidth, clipPlain(current.ID, idWidth), current.State)
		if i == m.Cursor {
			line = selectStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) detailLine() string {
	rows := m.rows()
	if m.Cursor >= len(rows) {
		return ""
	}
	row := rows[m.Cursor]
	return fmt.Sprintf("%s · %s · %s · %s", row.Name, row.ID, row.Source, row.Detail)
}

func (m Model) footer() string {
	if m.Mode == ModeConfirm {
		return "enter confirm   esc cancel"
	}
	if m.Mode == ModeUpdates || m.Mode == ModeScan {
		return "space select   enter confirm   esc cancel   ? help"
	}
	if m.Width < 55 {
		return "/ filter  space  u  S  ?  q"
	}
	return "/ filter   space toggle   enter detail   u updates   S scan/adopt   ? help   q quit"
}

func clip(value string, width int) string {
	// Lipgloss width accounts for ANSI escapes used by the styles above.
	if lipgloss.Width(value) <= width {
		return value
	}
	plain := stripANSI(value)
	return clipPlain(plain, width)
}

func clipPlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func stripANSI(value string) string {
	var b strings.Builder
	escape := false
	for _, r := range value {
		if r == '\x1b' {
			escape = true
			continue
		}
		if escape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				escape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
