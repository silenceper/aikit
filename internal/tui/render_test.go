package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/status"
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
						landmark = "Attention"
					}
					if state.name == "input" && width < 60 {
						landmark = "[Apply]"
					}
					if state.name == "partial" && width == 24 {
						landmark = "partial warni"
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

func TestOverviewDeduplicatesSnapshotAndInventoryForTheSameTarget(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Updates = updatecheck.CheckReport{}
	target := "/work/.cursor/skills/planning"
	m.Snapshot.Status.Items = []status.Item{{Kind: status.Unmanaged, Name: "planning", Path: target, Scope: config.Scope{Agent: "cursor"}}}
	m.Inventory = InventoryState{Complete: true, Items: []app.ScanItem{{
		Key: "g/cursor\x00" + target, Origin: "g/cursor", Target: target,
		State: app.ScanStateBrokenLink, Action: app.ScanActionAdopt,
		Skill: config.Skill{Name: "planning"},
	}}}

	rows := m.attentionRows()
	if len(rows) != 1 || rows[0].DestinationView != ViewMigration || rows[0].Name != "planning" {
		t.Fatalf("same target rendered as duplicate attention rows: %+v", rows)
	}
	counts := m.attentionCounts()
	if counts.migration != 1 || counts.status != 1 {
		t.Fatalf("deduplicated attention counts = %+v", counts)
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

func TestOneLineCollectionsKeepContextInLiveDetail(t *testing.T) {
	tests := []struct {
		name      string
		view      View
		configure func(*Model)
		primary   string
		secondary string
	}{
		{"library", ViewLibrary, nil, "alpha", "example.test"},
		{"workspaces", ViewWorkspaces, nil, "aikit", "/work/aikit"},
		{"presets", ViewPresets, nil, "review", "1 skill"},
		{"migration", ViewMigration, func(m *Model) { m.Inventory.Items = testInventoryItems() }, "one", "Adopt"},
		{"status", ViewStatus, nil, "loose", "/work/.codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, tt.view, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
			if tt.configure != nil {
				tt.configure(&m)
			}
			lines := strings.Split(stripANSI(m.ViewString()), "\n")
			regions := m.hitRegions()
			if len(regions.Rows) == 0 || regions.Rows[0].Height != 1 || !strings.Contains(lines[regions.Rows[0].Y], tt.primary) || !strings.Contains(stripANSI(m.ViewString()), tt.secondary) {
				t.Fatalf("%s did not render one-line collection plus live context:\n%s", tt.name, m.ViewString())
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

func TestCompactSkillDetailHasHardWrappedDisplayBudgets(t *testing.T) {
	detail := largeSkillDetail()
	for _, width := range []int{24, 38, 59, 80, 120} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			lines := compactSkillDetailLines(detail, width, 18)
			if len(lines) > 18 {
				t.Fatalf("detail lines=%d, budget=18: %q", len(lines), lines)
			}
			joined := strings.Join(lines, "\n")
			for _, marker := range []string{"locations", "files", "lines", "64K"} {
				if !strings.Contains(joined, marker) {
					t.Fatalf("width=%d missing explicit %q marker:\n%s", width, marker, joined)
				}
			}
			if strings.Contains(joined, "source-line-219") {
				t.Fatalf("main detail leaked final source line at width=%d:\n%s", width, joined)
			}
			for index, line := range lines {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("line %d width=%d > %d: %q", index, got, width, line)
				}
			}
		})
	}
	t.Run("four-line short layout keeps summary first and all omissions explicit", func(t *testing.T) {
		lines := compactSkillDetailLines(detail, 24, 4)
		if len(lines) != 4 || stripANSI(lines[0]) != "Summary" {
			t.Fatalf("four-line detail=%q, want Summary first", lines)
		}
		joined := strings.Join(lines, "\n")
		for _, marker := range []string{"locations", "files", "lines", "64K", "View SKILL.md"} {
			if !strings.Contains(joined, marker) {
				t.Fatalf("four-line detail missing %q: %q", marker, lines)
			}
		}
	})
}

func TestCompactSkillDetailScreenStaysBoundedAtRealTerminalSizes(t *testing.T) {
	for _, width := range []int{24, 38, 59, 80, 120} {
		for _, height := range []int{12, 24} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
				m.Snapshot, m.Width, m.Height = testSnapshot(), width, height
				m.Detail, m.Focus, m.SkillDetail = true, FocusDetail, largeSkillDetail()
				got := m.ViewString()
				plain := stripANSI(got)
				if strings.Contains(stripANSI(got), "source-line-219") {
					t.Fatalf("main detail exposed unbounded content:\n%s", got)
				}
				if width == 24 && height == 12 {
					for _, marker := range []string{"locations", "files", "lines", "64K"} {
						if !strings.Contains(plain, marker) {
							t.Fatalf("small terminal silently dropped %q omission marker:\n%s", marker, got)
						}
					}
				}
				lines := strings.Split(got, "\n")
				if len(lines) > height {
					t.Fatalf("screen lines=%d > height=%d", len(lines), height)
				}
				for index, line := range lines {
					if cells := lipgloss.Width(line); cells > width {
						t.Fatalf("screen line %d width=%d > %d: %q", index, cells, width, line)
					}
				}
			})
		}
	}
}

func TestWideLibrarySelectionSplitsEmbeddedDescriptionLinesBeforeJoiningColumns(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Width, m.Height = 180, 30
	m.Snapshot.Config.Library.Skills[0].Description = strings.Join([]string{
		"first description paragraph",
		"second description paragraph",
		"third description paragraph",
		"fourth description paragraph",
		"fifth description paragraph",
	}, "\n")

	layout := ComputeLayout(m.Width, m.Height)
	for index, line := range m.detailLinesForWidth(layout.Detail.Width) {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("detail line %d still contains a physical newline: %q", index, line)
		}
	}
	assertScreenBounds(t, m.ViewString(), m.Width, m.Height)
}

func TestFramedShellUsesSharedThreePaneGeometry(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m.Width, m.Height = 120, 30
	m.switchView(ViewLibrary)

	view := stripANSI(m.ViewString())
	corner := m.frameGlyphs().TopLeft
	for _, want := range []string{corner, "Navigation", "Library skills", "Details", "Shortcuts", "> Library skills"} {
		if !strings.Contains(view, want) {
			t.Fatalf("framed shell missing %q:\n%s", want, view)
		}
	}
	if strings.Count(view, corner) < 5 {
		t.Fatalf("expected app/navigation/collection/detail/footer frames:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.Width {
			t.Fatalf("line width=%d exceeds %d: %q", lipgloss.Width(line), m.Width, line)
		}
	}

	regions := m.hitRegions()
	if len(regions.Rows) == 0 || !regions.Layout.CollectionPanel.Body.Contains(regions.Rows[0].X, regions.Rows[0].Y) {
		t.Fatalf("row hit is not inside collection body: panel=%+v rows=%+v", regions.Layout.CollectionPanel.Body, regions.Rows)
	}
	if regions.ActionBar.Empty() || !regions.Layout.DetailPanel.Actions.Contains(regions.ActionBar.X, regions.ActionBar.Y) {
		t.Fatalf("action bar is not inside detail actions: panel=%+v bar=%+v", regions.Layout.DetailPanel.Actions, regions.ActionBar)
	}
}

func TestFramedShellLowHeightKeepsOneActivePanel(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 8
	view := stripANSI(m.ViewString())
	glyphs, frameLines := m.frameGlyphs(), 0
	for _, line := range strings.Split(view, "\n") {
		if (strings.HasPrefix(line, glyphs.TopLeft) && strings.HasSuffix(line, glyphs.TopRight)) || (strings.HasPrefix(line, glyphs.BottomLeft) && strings.HasSuffix(line, glyphs.BottomRight)) {
			frameLines++
		}
	}
	if frameLines != 2 || !strings.Contains(view, "> Library skills") {
		t.Fatalf("low-height frame is not one active collection panel: frame lines=%d:\n%s", frameLines, view)
	}
	if strings.Contains(view, "Navigation") || strings.Contains(view, "Details") {
		t.Fatalf("low-height view rendered hidden panes:\n%s", view)
	}
	if got := len(strings.Split(view, "\n")); got != 8 {
		t.Fatalf("height=%d want=8:\n%s", got, view)
	}
}

func TestCompactLibraryRowCollapsesEmbeddedContextToOnePhysicalLine(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Width, m.Height = 80, 12
	m.Snapshot.Config.Library.Skills[0].Source = ""
	m.Snapshot.Config.Library.Skills[0].Description = "first context line\nsecond context line\nthird context line"

	rows := m.rows()
	rendered := m.renderRowLines(rows[0], true, ComputeLayout(m.Width, m.Height).List.Width)
	if len(rendered) != 2 || strings.ContainsAny(rendered[1], "\r\n") {
		t.Fatalf("compact row escaped its two-line geometry: %q", rendered)
	}
	assertScreenBounds(t, m.ViewString(), m.Width, m.Height)
}

func assertScreenBounds(t *testing.T, rendered string, width, height int) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	if len(lines) > height {
		t.Fatalf("rendered %d physical lines into height %d:\n%s", len(lines), height, rendered)
	}
	for index, line := range lines {
		if cells := lipgloss.Width(line); cells > width {
			t.Fatalf("line %d width=%d > %d: %q", index, cells, width, line)
		}
	}
}

func largeSkillDetail() app.SkillDetail {
	lines := make([]string, 220)
	for index := range lines {
		lines[index] = fmt.Sprintf("source-line-%03d %s", index, strings.Repeat("content ", 8))
	}
	locations := make([]config.Scope, 7)
	for index := range locations {
		locations[index] = config.Scope{Project: fmt.Sprintf("project-%d", index), Agent: "codex"}
	}
	files := make([]app.SkillFile, 12)
	for index := range files {
		files[index] = app.SkillFile{Path: fmt.Sprintf("references/deep/file-%02d.md", index), Kind: app.SkillFileRegular}
	}
	return app.SkillDetail{
		Skill:            testSnapshot().Config.Library.Skills[0],
		EnabledLocations: locations,
		Files:            files,
		SkillMD:          strings.Join(lines, "\n"),
		SkillMDTruncated: true,
	}
}

func TestInventoryLoadingBeforeFirstRootDoesNotShowZeroOverZero(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Width, m.Height = 80, 24
	m.Inventory.Loading = true
	got := stripANSI(m.ViewString())
	if strings.Contains(got, "0/0") || !strings.Contains(got, "Scanning local inventory") {
		t.Fatalf("initial inventory status = %q", got)
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
	if !strings.Contains(got, "Migration") || strings.Contains(got, "/private/full/path") {
		t.Fatalf("narrow render is not a breadcrumb single pane:\n%s", got)
	}
}
