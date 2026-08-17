package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func installRenderTheme(t *testing.T, theme semanticTheme) {
	t.Helper()
	oldTheme, oldActive, oldMuted, oldError, oldSelect := uiTheme, activeStyle, mutedStyle, errorStyle, selectStyle
	oldProfile, oldDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	switch theme.mode {
	case themeDark:
		lipgloss.SetColorProfile(termenv.TrueColor)
		lipgloss.SetHasDarkBackground(true)
	case themeLight:
		lipgloss.SetColorProfile(termenv.TrueColor)
		lipgloss.SetHasDarkBackground(false)
	case themeReduced:
		lipgloss.SetColorProfile(termenv.ANSI)
	case themeNoColor:
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	uiTheme, activeStyle, mutedStyle, errorStyle, selectStyle = theme, theme.panelTitle, theme.muted, theme.error, theme.selected
	t.Cleanup(func() {
		uiTheme, activeStyle, mutedStyle, errorStyle, selectStyle = oldTheme, oldActive, oldMuted, oldError, oldSelect
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldDark)
	})
}

func TestVisualInspectionFixtures(t *testing.T) {
	seen := make(map[themeMode]bool)
	for _, mode := range []themeMode{themeDark, themeLight, themeReduced, themeNoColor} {
		t.Run(mode.String(), func(t *testing.T) {
			installRenderTheme(t, newSemanticTheme(mode))
			seen[mode] = true
			for _, width := range []int{120, 80, 59, 38} {
				m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
				m.Snapshot, m.Width, m.Height = testSnapshot(), width, 20
				m.Inventory.Items = []app.ScanItem{{Key: "error", State: app.ScanStateError, Skill: config.Skill{Name: "Broken skill"}}}
				ansi, plain := m.ViewString(), stripANSI(m.ViewString())
				t.Logf("theme=%s width=%d ANSI:\n%s\nSTRIPPED:\n%s", mode, width, ansi, plain)
				for _, landmark := range []string{"aikit", "Overview", "Needs attention", "Broken skill", "Error", ">"} {
					if !strings.Contains(plain, landmark) {
						t.Fatalf("theme=%s width=%d missing %q:\n%s", mode, width, landmark, plain)
					}
				}
				if mode == themeNoColor && strings.Contains(ansi, "\x1b[") {
					t.Fatalf("NO_COLOR fixture emitted ANSI: %q", ansi)
				}
			}
		})
	}
	if len(seen) != 4 {
		t.Fatalf("visual fixture modes=%v", seen)
	}
}

func TestVisualStateMatrixStaysBoundedAndKeepsLandmarks(t *testing.T) {
	states := []struct {
		name     string
		landmark string
		setup    func(*Model)
	}{
		{"healthy", "Overview", func(m *Model) { m.Snapshot.Status.Items = nil }},
		{"empty", "Add source", func(m *Model) { m.ActiveView = ViewLibrary; m.Snapshot.Config.Library.Skills = nil }},
		{"loading", "scanning", func(m *Model) { m.Inventory.Loading = true }},
		{"partial", "partial warning", func(m *Model) {
			m.ActiveView = ViewWorkspaces
			m.ProjectResult = app.Result{Exit: app.ExitPartial, Warnings: []string{"partial warning"}}
		}},
		{"warning", "Issue:", func(m *Model) { m.Inventory.Issues = []app.ScanIssue{{Message: "warning state"}} }},
		{"error", "Error:", func(m *Model) { m.Err = "error state" }},
		{"confirm", "[Confirm]", func(m *Model) { m.Mode, m.Preview.Summary = ModeConfirm, "Review exact plan" }},
		{"input", "Enter Apply", func(m *Model) { m.Mode, m.Input = ModeInput, inputState{Prompt: "Source"} }},
		{"more", "More actions", func(m *Model) { m.ActiveView, m.Mode = ViewStatus, ModeMore }},
		{"configuration", "[Validate]", func(m *Model) { m.Mode = ModeConfiguration }},
	}
	for _, width := range []int{120, 96, 95, 80, 60, 59, 38, 24} {
		for _, height := range []int{8, 12, 16, 30} {
			for _, state := range states {
				t.Run(fmt.Sprintf("%s-%dx%d", state.name, width, height), func(t *testing.T) {
					m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
					m.Snapshot, m.Width, m.Height = testSnapshot(), width, height
					state.setup(&m)
					got := m.ViewString()
					landmark := state.landmark
					if height == 8 && (state.name == "warning" || state.name == "error") {
						if width < 60 {
							landmark = "Attention"
						} else {
							landmark = "Needs attention"
						}
					}
					if state.name == "loading" && (height == 8 || width == 24) {
						landmark = "aikit"
					}
					if !strings.Contains(stripANSI(got), landmark) {
						t.Fatalf("missing %q:\n%s", landmark, got)
					}
					lines := strings.Split(got, "\n")
					if len(lines) > height {
						t.Fatalf("rendered %d lines into height %d", len(lines), height)
					}
					for i, line := range lines {
						if lipgloss.Width(line) > width {
							t.Fatalf("line %d width=%d > %d: %q", i, lipgloss.Width(line), width, line)
						}
					}
				})
			}
		}
	}
}

func TestModernOverviewMetricsAndAttentionOrdering(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Updates = updatecheck.CheckReport{}
	m.Inventory = InventoryState{Complete: true, Items: []app.ScanItem{
		{Key: "z-unmanaged", State: app.ScanStateUnmanaged, Skill: config.Skill{Name: "Unmanaged item"}},
		{Key: "z-error", State: app.ScanStateError, Skill: config.Skill{Name: "Error item"}},
		{Key: "b-drift", State: app.ScanStateDrifted, Skill: config.Skill{Name: "Drift item"}},
		{Key: "a-conflict", State: app.ScanStateNameConflict, Skill: config.Skill{Name: "Conflict item"}},
		{Key: "a-update", State: app.ScanStateUpdateAvailable, Skill: config.Skill{Name: "Update item"}},
	}}
	m.Width, m.Height = 120, 30

	got := stripANSI(m.ViewString())
	for _, want := range []string{"Skills", "Updates", "Not checked", "Unmanaged", "Issues", "Needs attention"} {
		if !strings.Contains(got, want) {
			t.Fatalf("overview missing %q:\n%s", want, got)
		}
	}
	ordered := []string{"Error item", "Conflict item", "Drift item", "Update item", "Unmanaged item"}
	previous := -1
	for _, want := range ordered {
		index := strings.Index(got, want)
		if index < 0 || index <= previous {
			t.Fatalf("attention order missing/out of order %q:\n%s", want, got)
		}
		previous = index
	}
	if strings.Contains(got, "SKILL.md") || strings.Contains(got, "/private/") || strings.Contains(got, "hash") {
		t.Fatalf("overview leaked heavy detail:\n%s", got)
	}
}

func TestModernOverviewUsesOnlySnapshotUpdateData(t *testing.T) {
	service := &fakeService{}
	m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Updates.Results = []updatecheck.Result{{SkillID: "acme/alpha", State: updatecheck.StateUpdateAvailable}}
	m.Width, m.Height = 96, 16
	got := stripANSI(m.ViewString())
	if !strings.Contains(got, "Updates  1") || service.snapshotCalls != 0 || service.updateCalls != 0 {
		t.Fatalf("overview update metric/calls invalid: snapshot=%d update=%d\n%s", service.snapshotCalls, service.updateCalls, got)
	}
}

func TestOverviewIncludesTypedUpdateFailuresAndWarningsAsErrors(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Status.Items = nil
	m.Snapshot.Config.PendingOperations = nil
	m.Snapshot.Updates = updatecheck.CheckReport{
		Results: []updatecheck.Result{
			{SkillID: "acme/alpha", State: updatecheck.StateCheckFailed, Error: "remote authentication failed"},
			{SkillID: "acme/beta", State: updatecheck.StateUpdateAvailable},
		},
		Warnings: []string{"update cache is unreadable"},
	}
	m.Inventory = InventoryState{Complete: true}
	m.Width, m.Height = 120, 24

	rows := m.attentionRows()
	if len(rows) != 3 || rows[0].Severity != rowSeverityError || rows[1].Severity != rowSeverityError || rows[2].Severity != rowSeverityWarning {
		t.Fatalf("update attention rows=%+v", rows)
	}
	keys := []string{rows[0].selectionKey(), rows[1].selectionKey(), rows[2].selectionKey()}
	again := m.attentionRows()
	if keys[0] != again[0].selectionKey() || keys[1] != again[1].selectionKey() || keys[2] != again[2].selectionKey() {
		t.Fatalf("update attention keys unstable: %v then %q/%q/%q", keys, again[0].selectionKey(), again[1].selectionKey(), again[2].selectionKey())
	}
	got := stripANSI(m.ViewString())
	for _, want := range []string{"Updates  1", "Issues  2", "remote authentication failed", "update cache is unreadable"} {
		if !strings.Contains(got, want) {
			t.Fatalf("overview missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "All clear") || strings.Contains(got, "Updates  Not checked") {
		t.Fatalf("overview misreported checked errors:\n%s", got)
	}
}

func TestTwoLineCollectionsHavePrimaryAndContextLines(t *testing.T) {
	tests := []struct {
		name      string
		view      View
		configure func(*Model)
		primary   string
		secondary string
	}{
		{"library", ViewLibrary, nil, "alpha", "example.test"},
		{"workspaces", ViewWorkspaces, nil, "Global", "All agents"},
		{"presets", ViewPresets, nil, "review", "1 skill"},
		{"migration", ViewMigration, func(m *Model) { m.Inventory.Items = testInventoryItems() }, "one", "Adopt"},
		{"status", ViewStatus, nil, "loose", "/work/.codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, tt.view, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 24
			if tt.configure != nil {
				tt.configure(&m)
			}
			lines := strings.Split(stripANSI(m.ViewString()), "\n")
			found := false
			for i := 0; i+1 < len(lines); i++ {
				if strings.Contains(lines[i], tt.primary) && strings.Contains(lines[i+1], tt.secondary) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s did not render adjacent primary/context lines:\n%s", tt.name, m.ViewString())
			}
		})
	}
}

func TestProgressiveDetailsUseSemanticGroups(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 24
	m.Detail = true
	m.SkillDetail = app.SkillDetail{
		Skill:            m.Snapshot.Config.Library.Skills[0],
		EnabledLocations: []config.Scope{{Agent: "codex"}},
		Files:            []app.SkillFile{{Path: "SKILL.md", Kind: app.SkillFileRegular}},
	}
	got := stripANSI(m.ViewString())
	for _, heading := range []string{"Summary", "Usage", "Files"} {
		if !strings.Contains(got, heading) {
			t.Fatalf("detail missing %q:\n%s", heading, got)
		}
	}
	if strings.Contains(got, "Diagnostics") {
		t.Fatalf("irrelevant Diagnostics group rendered:\n%s", got)
	}
	m.Err = "broken detail"
	if got = stripANSI(m.ViewString()); !strings.Contains(got, "Diagnostics") || !strings.Contains(got, "broken detail") {
		t.Fatalf("diagnostics not revealed when relevant:\n%s", got)
	}
}

func TestPrimaryActionsStayAtThreeOutsideMore(t *testing.T) {
	for _, view := range topViews {
		m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, view, ActionNone)
		m.Snapshot = testSnapshot()
		m.Inventory.Items = testInventoryItems()
		if actions := m.primaryActions(); len(actions) > 3 {
			t.Fatalf("%s actions=%v", view, actions)
		}
	}
}

func TestModernNavigationShowsCompactCountsAndAttention(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Inventory.Items = testInventoryItems()
	for _, tt := range []struct {
		width int
		want  []string
	}{
		{120, []string{"Library  2", "Migration  !", "Status  1"}},
		{80, []string{"Library  2", "Migration  !", "Status  1"}},
	} {
		m.Width, m.Height = tt.width, 20
		got := stripANSI(m.ViewString())
		for _, want := range tt.want {
			if !strings.Contains(got, want) {
				t.Fatalf("width %d missing navigation indicator %q:\n%s", tt.width, want, got)
			}
		}
	}
}

func TestSemanticBadgesOnCollectionRowsKeepNoColorMarkers(t *testing.T) {
	installRenderTheme(t, newSemanticTheme(themeNoColor))
	tests := []struct {
		name      string
		view      View
		configure func(*Model)
		want      string
	}{
		{"library-warning", ViewLibrary, nil, "[WARN]"},
		{"migration-conflict", ViewMigration, func(m *Model) {
			m.Inventory.Items = []app.ScanItem{{Key: "conflict", State: app.ScanStateNameConflict, Skill: config.Skill{Name: "conflict"}}}
		}, "[WARN]"},
		{"workspace-success", ViewWorkspaces, func(m *Model) { m.Scope = Scope{Agent: "codex", Level: "agent-skills"} }, "[OK] Direct"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, tt.view, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 96, 20
			if tt.configure != nil {
				tt.configure(&m)
			}
			got := m.ViewString()
			if strings.Contains(got, "\x1b[") || !strings.Contains(stripANSI(got), tt.want) || !strings.Contains(stripANSI(got), ">") {
				t.Fatalf("semantic row missing marker %q:\n%s", tt.want, got)
			}
		})
	}
}

func TestBadgeRenderingUsesTypedRowSeverityOnly(t *testing.T) {
	installRenderTheme(t, newSemanticTheme(themeNoColor))
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	tests := []struct {
		state    string
		severity rowSeverity
		marker   string
	}{
		{"healthy words", rowSeverityError, "[ERR]"},
		{"error conflict unmanaged update", rowSeverityInfo, "error conflict"},
		{"anything", rowSeverityConflict, "[WARN]"},
		{"anything", rowSeveritySuccess, "[OK]"},
	}
	for _, tt := range tests {
		got := stripANSI(m.renderRowLines(row{Name: "row", State: tt.state, Severity: tt.severity}, true, 60)[0])
		if !strings.Contains(got, tt.marker) {
			t.Fatalf("state=%q severity=%v rendered %q, want %s", tt.state, tt.severity, got, tt.marker)
		}
	}
}

func TestRenderNewInformationArchitectureAndDensity(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Inventory = InventoryState{Complete: true, Issues: []app.ScanIssue{{Message: "one root failed"}}, Items: []app.ScanItem{{
		Key: "candidate", Origin: "g/codex", Target: "/private/full/path/that/must/not/be/shown", Skill: config.Skill{Name: "loose"},
		State: app.ScanStateNameConflict, Action: app.ScanActionConflict,
	}}}
	m.Width, m.Height = 100, 24

	got := m.ViewString()
	for _, want := range []string{"Overview", "Library", "Workspaces", "Presets", "Migration", "Status", "Needs attention", "one root failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("overview missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/private/full/path") || strings.Contains(got, "SKILL.md") {
		t.Fatalf("overview leaked heavy detail:\n%s", got)
	}

	m.switchView(ViewMigration)
	got = m.ViewString()
	for _, want := range []string{"loose", "Name conflict", "Conflict"} {
		if !strings.Contains(got, want) {
			t.Fatalf("migration missing %q:\n%s", want, got)
		}
	}
	if len(m.primaryActions()) > 3 {
		t.Fatalf("detail exposed more than three actions:\n%s", got)
	}

	m.Width, m.Height = 38, 12
	got = m.ViewString()
	if !strings.Contains(got, "aikit / Migration") || strings.Contains(got, "/private/full/path") {
		t.Fatalf("narrow render is not a breadcrumb single pane:\n%s", got)
	}
}
