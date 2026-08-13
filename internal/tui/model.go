package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type View string

const (
	ViewLibrary  View = "library"
	ViewAgents   View = "agents"
	ViewProjects View = "projects"
	ViewPresets  View = "presets"
	ViewStatus   View = "status"
)

type Action string

const (
	ActionNone          Action = ""
	ActionEnable        Action = "enable"
	ActionUpdate        Action = "update"
	ActionScan          Action = "scan"
	ActionRemoveSkill   Action = "remove skill"
	ActionRemoveProject Action = "remove project"
)

type Mode string

const (
	ModeTable   Mode = "table"
	ModeFilter  Mode = "filter"
	ModeUpdates Mode = "updates"
	ModeScan    Mode = "scan"
	ModeConfirm Mode = "confirm"
)

type Scope struct {
	Agent   string
	Project string
	Preset  string
	Level   string
}

type row struct {
	ID      string
	Name    string
	Origin  string
	Target  string
	Source  string
	State   string
	Detail  string
	Enabled bool
}

func (r row) selectionKey() string {
	if r.Target != "" {
		return r.Origin + "\x00" + r.Target
	}
	return r.ID
}

type Model struct {
	ctx       context.Context
	service   app.Service
	migration app.MigrationService
	action    Action

	ActiveView   View
	Mode         Mode
	Scope        Scope
	Cursor       int
	Filter       string
	Help         bool
	Detail       bool
	Width        int
	Height       int
	Status       string
	Err          string
	Busy         bool
	Snapshot     app.Snapshot
	Scan         app.ScanResult
	Selected     map[string]bool
	confirm      Action
	pendingID    string
	filterParent Mode
}

func NewModel(ctx context.Context, service app.Service, migration app.MigrationService, initialView View, initialAction Action) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	if !validView(initialView) {
		initialView = ViewLibrary
	}
	return Model{
		ctx: ctx, service: service, migration: migration, action: initialAction,
		ActiveView: initialView, Mode: ModeTable, Width: 80, Height: 24,
		Status: "loading snapshot…", Selected: make(map[string]bool),
	}
}

func validView(view View) bool {
	switch view {
	case ViewLibrary, ViewAgents, ViewProjects, ViewPresets, ViewStatus:
		return true
	default:
		return false
	}
}

func (m Model) Init() tea.Cmd { return snapshotCmd(m.ctx, m.service) }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		return m, nil
	case snapshotMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err = msg.err.Error()
			m.Status = "snapshot failed"
			return m, nil
		}
		m.Snapshot = msg.snapshot
		m.Err = strings.Join(msg.snapshot.Updates.Warnings, "; ")
		m.Status = "ready"
		action := m.action
		m.action = ActionNone
		switch action {
		case ActionUpdate:
			m.openUpdates()
		case ActionScan:
			m.Busy = true
			m.Status = "scanning…"
			return m, scanCmd(m.ctx, m.migration, app.ScanRequest{All: true, DryRun: true})
		case ActionEnable:
			m.ActiveView = ViewAgents
		}
		m.clampCursor()
		return m, nil
	case scanMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err = msg.err.Error()
			m.Status = "scan failed"
			return m, nil
		}
		m.Scan = msg.result
		m.Mode = ModeScan
		m.Cursor = 0
		m.Selected = make(map[string]bool)
		m.Status = fmt.Sprintf("found %d candidates", len(msg.result.Items))
		m.Err = strings.Join(msg.result.Warnings, "; ")
		return m, nil
	case operationMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err = msg.err.Error()
			m.Status = msg.name + " failed"
			return m, nil
		}
		m.Err = ""
		m.Status = msg.name + " completed"
		if msg.name == "update" {
			m.Status = "updated selected skills"
		}
		m.Mode = ModeTable
		m.confirm = ActionNone
		m.Selected = make(map[string]bool)
		return m, snapshotCmd(m.ctx, m.service)
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m *Model) openUpdates() {
	m.Mode = ModeUpdates
	m.Scope = Scope{}
	m.Cursor = 0
	m.Detail = false
	m.Help = false
	m.Selected = make(map[string]bool)
	m.Status = "select updates with space"
}

func (m *Model) switchView(view View) {
	m.ActiveView = view
	m.Mode = ModeTable
	m.Scope = Scope{}
	m.Cursor = 0
	m.Filter = ""
	m.Detail = false
	m.Help = false
	m.Selected = make(map[string]bool)
	m.filterParent = ModeTable
}

func (m *Model) clampCursor() {
	n := len(m.rows())
	if n == 0 {
		m.Cursor = 0
	} else if m.Cursor >= n {
		m.Cursor = n - 1
	} else if m.Cursor < 0 {
		m.Cursor = 0
	}
}

func (m Model) rows() []row {
	var rows []row
	mode := m.Mode
	if mode == ModeFilter {
		mode = m.filterParent
	}
	switch mode {
	case ModeUpdates:
		for _, item := range m.Snapshot.Updates.Results {
			if item.State == updatecheck.StateUpdateAvailable {
				rows = append(rows, row{ID: item.SkillID, Name: skillName(m.Snapshot.Config.Library.Skills, item.SkillID), State: string(item.State), Detail: shortOID(item.Current) + " → " + shortOID(item.Remote)})
			}
		}
		return m.filtered(rows)
	case ModeScan:
		for _, item := range m.Scan.Items {
			state := "candidate"
			if item.Adopted {
				state = "adopted"
			} else if item.Error != "" {
				state = "error"
			}
			rows = append(rows, row{ID: item.Skill.ID, Name: item.Skill.Name, Origin: item.Origin, Target: item.Target, Source: item.Target, State: state, Detail: item.Origin + " · " + item.Error})
		}
		return m.filtered(rows)
	}

	switch m.ActiveView {
	case ViewLibrary:
		for _, skill := range m.Snapshot.Config.Library.Skills {
			rows = append(rows, row{ID: skill.ID, Name: skill.Name, Source: skill.Source, State: updateState(m.Snapshot.Updates, skill.ID), Detail: skill.Description + " · resolved " + shortOID(skill.Resolved)})
		}
	case ViewAgents:
		if m.Scope.Level == "agent-skills" {
			binding := m.Snapshot.Config.Agents[m.Scope.Agent]
			for _, skill := range m.Snapshot.Config.Library.Skills {
				rows = append(rows, row{ID: skill.ID, Name: skill.Name, State: toggleState(contains(binding.Skills, skill.ID)), Enabled: contains(binding.Skills, skill.ID), Detail: skill.Description})
			}
		} else {
			for _, name := range agent.Names() {
				binding := m.Snapshot.Config.Agents[name]
				rows = append(rows, row{ID: name, Name: name, State: fmt.Sprintf("%d skills", len(binding.Skills))})
			}
		}
	case ViewProjects:
		rows = m.projectRows()
	case ViewPresets:
		if m.Scope.Level == "preset-skills" {
			if preset, ok := findPreset(m.Snapshot.Config.Presets, m.Scope.Preset); ok {
				for _, skill := range m.Snapshot.Config.Library.Skills {
					enabled := contains(preset.Skills, skill.ID)
					rows = append(rows, row{ID: skill.ID, Name: skill.Name, State: toggleState(enabled), Enabled: enabled})
				}
			}
		} else {
			for _, preset := range m.Snapshot.Config.Presets {
				rows = append(rows, row{ID: preset.Name, Name: preset.Name, State: fmt.Sprintf("%d skills", len(preset.Skills))})
			}
		}
	case ViewStatus:
		for i, item := range m.Snapshot.Status.Items {
			id := item.SkillID
			if id == "" {
				id = fmt.Sprintf("%s:%d", item.Kind, i)
			}
			rows = append(rows, row{ID: id, Name: firstNonEmpty(item.Name, item.SkillID, item.Path), Source: item.Path, State: string(item.Kind), Detail: item.Message})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if m.ActiveView == ViewProjects && m.Scope.Level == "project-targets" {
			if rows[i].ID == "common" {
				return true
			}
			if rows[j].ID == "common" {
				return false
			}
		}
		return rows[i].ID < rows[j].ID
	})
	return m.filtered(rows)
}

func (m Model) projectRows() []row {
	project, ok := findProject(m.Snapshot.Config.Projects, m.Scope.Project)
	switch m.Scope.Level {
	case "project-targets":
		if !ok {
			return nil
		}
		rows := []row{{ID: "common", Name: "common", State: fmt.Sprintf("%d skills", len(project.Skills))}}
		for _, name := range project.Agents {
			rows = append(rows, row{ID: name, Name: name, State: fmt.Sprintf("%d skills", len(project.AgentBindings[name].Skills))})
		}
		return m.filtered(rows)
	case "project-skills":
		if !ok {
			return nil
		}
		binding := project.Binding
		if m.Scope.Agent != "" {
			binding = project.AgentBindings[m.Scope.Agent]
		}
		var rows []row
		for _, skill := range m.Snapshot.Config.Library.Skills {
			enabled := contains(binding.Skills, skill.ID)
			rows = append(rows, row{ID: skill.ID, Name: skill.Name, State: toggleState(enabled), Enabled: enabled})
		}
		return m.filtered(rows)
	default:
		var rows []row
		for _, current := range m.Snapshot.Config.Projects {
			rows = append(rows, row{ID: current.Name, Name: current.Name, Source: current.Path, State: strings.Join(current.Agents, ",")})
		}
		return m.filtered(rows)
	}
}

func (m Model) filtered(rows []row) []row {
	query := strings.ToLower(strings.TrimSpace(m.Filter))
	if query == "" {
		return rows
	}
	filtered := make([]row, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.ID+" "+row.Name+" "+row.Source+" "+row.State), query) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func updateState(report updatecheck.CheckReport, id string) string {
	for _, item := range report.Results {
		if item.SkillID == id {
			if item.State == updatecheck.StateUpdateAvailable {
				return "↑ update"
			}
			return string(item.State)
		}
	}
	return ""
}

func skillName(skills []config.Skill, id string) string {
	for _, skill := range skills {
		if skill.ID == id {
			return skill.Name
		}
	}
	return id
}

func findProject(projects []config.Project, name string) (config.Project, bool) {
	for _, project := range projects {
		if project.Name == name {
			return project, true
		}
	}
	return config.Project{}, false
}

func findPreset(presets []config.Preset, name string) (config.Preset, bool) {
	for _, preset := range presets {
		if preset.Name == name {
			return preset, true
		}
	}
	return config.Preset{}, false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "-"
}

func toggleState(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func shortOID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func isUnmanaged(item status.Item) bool { return item.Kind == status.Unmanaged }
