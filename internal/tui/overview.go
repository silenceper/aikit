package tui

import (
	"strings"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type overviewSectionID string

const (
	overviewQuick   overviewSectionID = "quick"
	overviewUpdates overviewSectionID = "updates"
	overviewLocal   overviewSectionID = "local"
	overviewHealth  overviewSectionID = "health"
)

type overviewTask struct {
	Key               string
	SelectionKey      string
	Name              string
	State             string
	Detail            string
	SkillID           string
	Current           string
	Remote            string
	Origin            string
	Target            string
	Action            app.ScanAction
	Selector          app.ScanSelector
	Selectable        bool
	Severity          rowSeverity
	DestinationView   View
	DestinationMode   Mode
	DestinationKey    string
	DestinationAction Action
}

type overviewDashboard struct {
	QuickActions []uiAction
	Updates      []overviewTask
	Local        []overviewTask
	Health       []overviewTask
}

func (overviewDashboard) sectionIDs() []overviewSectionID {
	return []overviewSectionID{overviewQuick, overviewUpdates, overviewLocal, overviewHealth}
}

func (dashboard overviewDashboard) tasks(section overviewSectionID) []overviewTask {
	switch section {
	case overviewUpdates:
		return dashboard.Updates
	case overviewLocal:
		return dashboard.Local
	case overviewHealth:
		return dashboard.Health
	default:
		return nil
	}
}

func (m Model) overviewDashboard() overviewDashboard {
	dashboard := overviewDashboard{QuickActions: []uiAction{uiAddSource, uiCreateProject, uiCreatePreset}}
	dashboard.Updates = m.overviewUpdateTasks()
	dashboard.Local = m.overviewLocalTasks()
	dashboard.Health = m.overviewHealthTasks()
	return dashboard
}

func (m Model) overviewUpdateTasks() []overviewTask {
	tasks := make([]overviewTask, 0, len(m.Snapshot.Updates.Results)+len(m.Snapshot.Updates.Warnings))
	for _, result := range m.Snapshot.Updates.Results {
		if result.State != updatecheck.StateUpdateAvailable && result.State != updatecheck.StateCheckFailed {
			continue
		}
		skill, found := overviewSkill(m.Snapshot.Config, result.SkillID)
		selectable := result.State == updatecheck.StateUpdateAvailable && found && skill.Source != "" && skill.Ref != nil && skill.Ref.Kind == "branch" && result.Current != "" && result.Remote != "" && result.Current == skill.Resolved
		state := "Update available"
		detail := shortOID(result.Current) + " -> " + shortOID(result.Remote)
		severity := rowSeverityWarning
		if result.State == updatecheck.StateCheckFailed {
			state, detail, severity = "Update check failed", firstNonEmpty(result.Error, "Update check failed"), rowSeverityError
		} else if !selectable {
			detail = firstNonEmpty(detail, "Update token incomplete")
		}
		tasks = append(tasks, overviewTask{
			Key: "overview:update:" + result.SkillID, SelectionKey: "overview:update:" + result.SkillID,
			Name: skillName(m.Snapshot.Config.Library.Skills, result.SkillID), State: state, Detail: detail,
			SkillID: result.SkillID, Current: result.Current, Remote: result.Remote, Selectable: selectable, Severity: severity,
			DestinationMode: ModeUpdates, DestinationKey: result.SkillID,
		})
	}
	for _, warning := range uniqueStrings(m.Snapshot.Updates.Warnings) {
		tasks = append(tasks, overviewTask{Key: "overview:update-warning:" + warning, Name: "Update check warning", State: "Update check failed", Detail: warning, Severity: rowSeverityError, DestinationView: ViewStatus})
	}
	return tasks
}

func (m Model) overviewLocalTasks() []overviewTask {
	tasks := make([]overviewTask, 0, len(m.Inventory.Items))
	for _, item := range m.Inventory.Items {
		if item.State == app.ScanStateManaged && item.Action == app.ScanActionNone {
			continue
		}
		state := item.State
		if state == "" {
			state = item.ManagementState
		}
		selectable := overviewLocalExecutable(item, state)
		name := firstNonEmpty(item.Skill.Name, item.Discovered.Name, item.MatchedLibraryID, item.Key)
		tasks = append(tasks, overviewTask{
			Key: "overview:local:" + item.Key, SelectionKey: "overview:local:" + item.Key,
			Name: name, State: humanState(string(state)), Detail: firstNonEmpty(humanAction(item.Action), item.Error),
			SkillID: item.Skill.ID, Origin: item.Origin, Target: item.Target, Action: item.Action,
			Selector: app.ScanSelector{
				Key: item.Key, Origin: item.Origin, Target: item.Target,
				ExpectedHash: item.ContentHash, ExpectedObjectID: item.ObjectID, ExpectedRootID: item.RootObjectID,
				ExpectedState: item.State, ExpectedSkillID: item.Skill.ID, ExpectedLibraryHash: item.MatchedLibraryHash,
			},
			Selectable: selectable, Severity: scanStateSeverity(state), DestinationView: ViewMigration, DestinationKey: item.Key,
		})
	}
	return tasks
}

func (m Model) overviewHealthTasks() []overviewTask {
	attention := m.attentionRows()
	tasks := make([]overviewTask, 0, len(attention))
	for _, current := range attention {
		if current.DestinationMode == ModeUpdates || strings.HasPrefix(current.Key, "attention:inventory:") && current.Severity == rowSeverityWarning {
			continue
		}
		tasks = append(tasks, overviewTask{
			Key: current.Key, Name: current.Name, State: current.State, Detail: current.Detail, SkillID: current.ID,
			Severity: current.Severity, DestinationView: current.DestinationView, DestinationMode: current.DestinationMode,
			DestinationKey: current.DestinationKey, DestinationAction: current.DestinationAction,
		})
	}
	return tasks
}

func overviewLocalExecutable(item app.ScanItem, state app.ScanState) bool {
	switch state {
	case app.ScanStateNameConflict, app.ScanStateDrifted, app.ScanStateError, app.ScanStatePendingRecovery, app.ScanStateBrokenLink:
		return false
	}
	switch item.Action {
	case app.ScanActionImport, app.ScanActionAdopt, app.ScanActionLinkExisting:
		return item.Key != "" && item.Origin != "" && item.Target != ""
	default:
		return false
	}
}

func overviewSkill(cfg config.Config, id string) (config.Skill, bool) {
	for _, skill := range cfg.Library.Skills {
		if skill.ID == id {
			return skill, true
		}
	}
	return config.Skill{}, false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (m *Model) switchOverviewSection(section overviewSectionID) {
	if section != overviewQuick && section != overviewUpdates && section != overviewLocal && section != overviewHealth {
		return
	}
	if m.OverviewSection == section {
		return
	}
	m.OverviewSection, m.Cursor, m.Scroll, m.ActionIndex = section, 0, 0, 0
	if section == overviewQuick {
		m.Focus = FocusCollectionActions
	} else {
		m.Focus = FocusList
	}
}

func (m *Model) ensureOverviewVisible() {
	dashboard := m.overviewDashboard()
	tasks := dashboard.tasks(m.OverviewSection)
	if len(tasks) == 0 {
		m.Cursor, m.Scroll = 0, 0
		return
	}
	m.Cursor = min(max(0, m.Cursor), len(tasks)-1)
	geometry := m.overviewLayout(ComputeLayout(m.Width, m.Height))
	m.Scroll = geometry.Rows[m.OverviewSection].Start
}
