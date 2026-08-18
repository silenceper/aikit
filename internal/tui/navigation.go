package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type navigationKind int

const (
	navigationView navigationKind = iota
	navigationAction
)

type navigationEntry struct {
	Key     string
	Label   string
	Section string
	Kind    navigationKind
	View    View
	Scope   Scope
	Action  uiAction
}

func navigationEntries(m Model) []navigationEntry {
	entries := []navigationEntry{
		{Key: "overview", Label: "Overview", Section: "Main", Kind: navigationView, View: ViewOverview},
		{Key: "library", Label: "Library", Section: "Main", Kind: navigationView, View: ViewLibrary},
		{Key: "presets", Label: "Presets", Section: "Main", Kind: navigationView, View: ViewPresets},
		{Key: "global", Label: "Global", Section: "Workspaces", Kind: navigationView, View: ViewWorkspaces, Scope: Scope{Level: "workspace-global"}},
		{Key: "agents", Label: "Agents", Section: "Workspaces", Kind: navigationView, View: ViewWorkspaces, Scope: Scope{Level: "workspace-agents"}},
		{Key: "projects", Label: "Projects", Section: "Workspaces", Kind: navigationView, View: ViewWorkspaces, Scope: Scope{Level: "workspace-projects"}},
		configurationNavigationEntry(),
		{Key: "add-source", Label: "Add source", Section: "Actions", Kind: navigationAction, Action: uiAddSource},
		{Key: "create-project", Label: "Create project", Section: "Actions", Kind: navigationAction, Action: uiCreateProject},
		{Key: "create-preset", Label: "Create preset", Section: "Actions", Kind: navigationAction, Action: uiCreatePreset},
	}
	if len(m.Snapshot.Config.PendingOperations) > 0 {
		entries = append(entries, navigationEntry{Key: "review-recovery", Label: "Review recovery", Section: "Actions", Kind: navigationAction, Action: uiReviewRecovery})
	}
	return entries
}

func commandNavigationEntries(m Model) []navigationEntry {
	entries := navigationEntries(m)
	advanced := []navigationEntry{
		{Key: "status", Label: "Review health details", Section: "Tools", Kind: navigationView, View: ViewStatus},
		{Key: "migration", Label: "Review local skill imports", Section: "Tools", Kind: navigationView, View: ViewMigration},
	}
	insertAt := len(entries)
	for i, entry := range entries {
		if entry.Section == "Actions" {
			insertAt = i
			break
		}
	}
	result := make([]navigationEntry, 0, len(entries)+len(advanced))
	result = append(result, entries[:insertAt]...)
	result = append(result, advanced...)
	result = append(result, entries[insertAt:]...)
	return result
}

func (m Model) commandEntries() []navigationEntry {
	query := strings.ToLower(strings.TrimSpace(m.CommandDraft))
	entries := commandNavigationEntries(m)
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
		if entry.View == ViewConfiguration {
			if m.readingActivityActive() {
				m.switchDestination(entry)
				return m, nil
			}
			m.switchDestination(entry)
			m.Busy, m.Status = true, "Loading configuration paths..."
			return m, configurationCmd(m.ctx, m.service)
		}
		m.switchDestination(entry)
		return m, nil
	case navigationAction:
		m.Mode = ModeTable
		return m.perform(entry.Action)
	default:
		return m, nil
	}
}

func configurationNavigationEntry() navigationEntry {
	return navigationEntry{Key: "configuration", Label: "Configuration", Section: "Tools", Kind: navigationView, View: ViewConfiguration}
}

func (m *Model) switchDestination(entry navigationEntry) {
	m.saveRoutePosition()
	m.switchView(entry.View)
	if entry.Scope.Level != "" {
		m.Scope = entry.Scope
	}
	m.restoreRoutePosition()
	m.Focus, m.ActionPane, m.ActionIndex = FocusList, actionPaneNone, 0
	m.syncNavigationIndex()
}

func (m *Model) saveRoutePosition() {
	if m.routePositions == nil {
		m.routePositions = make(map[string]routePosition)
	}
	key := m.routeKey()
	if key == "" {
		return
	}
	m.routePositions[key] = routePosition{Cursor: m.Cursor, Scroll: m.Scroll, ActiveKey: m.activeKey()}
}

func (m *Model) restoreRoutePosition() {
	position, ok := m.routePositions[m.routeKey()]
	if !ok {
		m.Cursor, m.Scroll = 0, 0
		return
	}
	m.Cursor, m.Scroll = position.Cursor, position.Scroll
	if position.ActiveKey != "" {
		m.restoreActiveKey(position.ActiveKey)
	}
	m.clampCursor()
}

func (m Model) routeKey() string {
	if m.ActiveView == ViewWorkspaces {
		return string(m.ActiveView) + ":" + workspaceRouteLevel(m.Scope.Level)
	}
	if m.ActiveView == "" {
		return ""
	}
	return string(m.ActiveView)
}

func (m *Model) syncNavigationIndex() {
	entries := layoutNavigationEntries(ComputeLayout(m.Width, m.Height), *m)
	for index, item := range entries {
		if navigationEntryActive(*m, item.Entry) {
			m.NavigationIndex = index
			return
		}
	}
	m.NavigationIndex = 0
}

func navigationEntryActive(m Model, entry navigationEntry) bool {
	if entry.Kind != navigationView || entry.View != m.ActiveView {
		return false
	}
	if entry.View != ViewWorkspaces {
		return true
	}
	return entry.Scope.Level == workspaceRouteLevel(m.Scope.Level)
}

func workspaceRouteLevel(level string) string {
	switch level {
	case "workspace-global":
		return "workspace-global"
	case "workspace-agents", "agent-skills":
		return "workspace-agents"
	case "workspace-projects", "project-targets", "project-skills":
		return "workspace-projects"
	default:
		return "workspace-projects"
	}
}
