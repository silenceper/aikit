package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type navigationKind int

const (
	navigationView navigationKind = iota
	navigationConfiguration
	navigationAction
)

type navigationEntry struct {
	Key     string
	Label   string
	Section string
	Kind    navigationKind
	View    View
	Action  uiAction
}

func navigationEntries(m Model) []navigationEntry {
	entries := []navigationEntry{
		{Key: "overview", Label: "Overview", Section: "Main", Kind: navigationView, View: ViewOverview},
		{Key: "library", Label: "Library", Section: "Main", Kind: navigationView, View: ViewLibrary},
		{Key: "workspaces", Label: "Workspaces", Section: "Main", Kind: navigationView, View: ViewWorkspaces},
		{Key: "presets", Label: "Presets", Section: "Main", Kind: navigationView, View: ViewPresets},
		{Key: "status", Label: "Status", Section: "Main", Kind: navigationView, View: ViewStatus},
		{Key: "migration", Label: "Migration", Section: "Tools", Kind: navigationView, View: ViewMigration},
		{Key: "configuration", Label: "Configuration", Section: "Tools", Kind: navigationConfiguration},
		{Key: "add-source", Label: "Add source", Section: "Actions", Kind: navigationAction, Action: uiAddSource},
		{Key: "create-project", Label: "Create project", Section: "Actions", Kind: navigationAction, Action: uiCreateProject},
		{Key: "create-preset", Label: "Create preset", Section: "Actions", Kind: navigationAction, Action: uiCreatePreset},
	}
	if len(m.Snapshot.Config.PendingOperations) > 0 {
		entries = append(entries, navigationEntry{Key: "review-recovery", Label: "Review recovery", Section: "Actions", Kind: navigationAction, Action: uiReviewRecovery})
	}
	return entries
}

func (m Model) commandEntries() []navigationEntry {
	query := strings.ToLower(strings.TrimSpace(m.CommandDraft))
	entries := navigationEntries(m)
	result := make([]navigationEntry, 0, len(entries))
	for _, entry := range entries {
		if query == "" || strings.Contains(strings.ToLower(entry.Label), query) || strings.Contains(strings.ToLower(entry.Section), query) {
			result = append(result, entry)
		}
	}
	return result
}

func (m *Model) enterCommandPalette() {
	m.Mode = ModeCommand
	m.CommandDraft = ""
	m.CommandIndex = 0
	m.OverlayScroll = 0
}

func (m Model) activateCommandEntry(entry navigationEntry) (tea.Model, tea.Cmd) {
	switch entry.Kind {
	case navigationView:
		m.switchView(entry.View)
		return m, nil
	case navigationConfiguration:
		m.enterConfiguration()
		return m, configurationCmd(m.ctx, m.service)
	case navigationAction:
		m.Mode = ModeTable
		return m.perform(entry.Action)
	default:
		return m, nil
	}
}
