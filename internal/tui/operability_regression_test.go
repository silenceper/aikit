package tui

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func TestOverviewAttentionOpenRoutesToExactDestination(t *testing.T) {
	t.Run("status item", func(t *testing.T) {
		m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
		m.switchView(ViewOverview)
		moveCursorToPrefix(t, &m, "attention:status:item:")
		wanted := strings.TrimPrefix(m.rows()[m.Cursor].Key, "attention:")

		m, _ = apply(m, "enter")

		if m.ActiveView != ViewStatus || m.Mode != ModeTable {
			t.Fatalf("open status attention = view %s mode %s", m.ActiveView, m.Mode)
		}
		if got := m.rows()[m.Cursor].selectionKey(); got != wanted {
			t.Fatalf("focused status row = %q, want %q", got, wanted)
		}
	})

	t.Run("migration item", func(t *testing.T) {
		m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
		m.Inventory.Items = []app.ScanItem{{
			Key: "inventory-one", Origin: "g/codex", Target: "/work/.codex/skills/one",
			Skill: config.Skill{ID: "local/one", Name: "one"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt,
		}}
		m.switchView(ViewOverview)
		moveCursorToPrefix(t, &m, "attention:inventory:")

		m, _ = apply(m, "enter")

		if m.ActiveView != ViewMigration || m.Mode != ModeTable {
			t.Fatalf("open migration attention = view %s mode %s", m.ActiveView, m.Mode)
		}
		if got := m.rows()[m.Cursor].selectionKey(); got != "inventory-one" {
			t.Fatalf("focused migration row = %q, want inventory-one", got)
		}
	})

	t.Run("available update", func(t *testing.T) {
		m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
		m.switchView(ViewOverview)
		moveCursorToPrefix(t, &m, "attention:update:")

		m, _ = apply(m, "enter")

		if m.Mode != ModeUpdates {
			t.Fatalf("open update attention = mode %s, want %s", m.Mode, ModeUpdates)
		}
		if got := m.rows()[m.Cursor].selectionKey(); got != "acme/alpha" {
			t.Fatalf("focused update row = %q, want acme/alpha", got)
		}
	})

	for _, tt := range []struct {
		name      string
		prefix    string
		wantedKey string
		configure func(*Model)
	}{
		{
			name: "update failure", prefix: "attention:update-failure:", wantedKey: "status:update-failure:acme/alpha",
			configure: func(m *Model) {
				m.Snapshot.Updates.Results = []updatecheck.Result{{SkillID: "acme/alpha", State: updatecheck.StateCheckFailed, Error: "offline"}}
			},
		},
		{
			name: "update warning", prefix: "attention:update-warning:",
			configure: func(m *Model) {
				m.Snapshot.Updates.Results = nil
				m.Snapshot.Updates.Warnings = []string{"cache unreadable"}
				m.Snapshot.Status.Items = nil
			},
		},
		{
			name: "inventory issue", prefix: "attention:issue:", wantedKey: "inventory-issue:0:g/codex",
			configure: func(m *Model) {
				m.Snapshot.Status.Items, m.Snapshot.Updates = nil, updatecheck.CheckReport{}
				m.Inventory.Issues = []app.ScanIssue{{Origin: "g/codex", Path: "/work/.codex/skills", State: app.ScanStateError, Message: "permission denied"}}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
			tt.configure(&m)
			m.switchView(ViewOverview)
			moveCursorToPrefix(t, &m, tt.prefix)
			destination := m.rows()[m.Cursor].DestinationKey
			m, _ = apply(m, "enter")
			if m.ActiveView != ViewStatus || m.Mode != ModeTable {
				t.Fatalf("route = view %s mode %s", m.ActiveView, m.Mode)
			}
			wanted := tt.wantedKey
			if wanted == "" {
				wanted = destination
			}
			if got := m.rows()[m.Cursor].selectionKey(); got != wanted {
				t.Fatalf("focused row=%q, want %q", got, wanted)
			}
		})
	}

	t.Run("recovery operation", func(t *testing.T) {
		operation := config.PendingOperation{ID: "op-exact", Target: "/work/skill"}
		service := &fakeService{snapshot: testSnapshot(), recoveryPreview: app.RecoveryPreview{Operations: []app.RecoveryOperation{{Operation: operation, CanResume: true}}}}
		service.snapshot.Config.PendingOperations = []config.PendingOperation{operation}
		m := NewModel(nil, service, &fakeMigration{}, ViewOverview, ActionNone)
		m.Snapshot = service.snapshot
		moveCursorToPrefix(t, &m, "attention:recovery:")
		next, preview := apply(m, "enter")
		m = next
		if preview == nil || !m.Busy {
			t.Fatalf("recovery preview cmd=%v busy=%v", preview != nil, m.Busy)
		}
		nextModel, _ := m.Update(preview())
		m = nextModel.(Model)
		if m.Mode != ModeConfirm || !reflect.DeepEqual(service.lastRecoveryPreview.OperationIDs, []string{"op-exact"}) {
			t.Fatalf("recovery mode=%s request=%v", m.Mode, service.lastRecoveryPreview.OperationIDs)
		}
	})
}

func TestMigrationPreviewCancelRestoresInventoryContext(t *testing.T) {
	migration := &fakeMigration{result: app.ScanResult{Items: []app.ScanItem{{
		Key: "preview-two", Origin: "g/codex", Target: "/work/.codex/skills/two",
		Skill: config.Skill{ID: "local/two", Name: "two"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt,
	}}}}
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, migration)
	m.switchView(ViewMigration)
	m.Inventory.Items = []app.ScanItem{
		{Key: "inventory-one", Origin: "g/codex", Target: "/work/.codex/skills/one", Skill: config.Skill{ID: "local/one", Name: "one"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt},
		{Key: "inventory-two", Origin: "g/codex", Target: "/work/.codex/skills/two", Skill: config.Skill{ID: "local/two", Name: "two"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt},
	}
	m.Cursor, m.Scroll, m.Focus = 1, 1, FocusList
	m.Selected["inventory-one"] = true
	wantSelected := map[string]bool{"inventory-one": true}

	next, preview := apply(m, "enter")
	m = next
	if preview == nil {
		t.Fatal("migration preview command is nil")
	}
	nextModel, _ := m.Update(preview())
	m = nextModel.(Model)
	if m.Mode != ModeConfirm {
		t.Fatalf("preview mode = %s, want confirm", m.Mode)
	}

	m, _ = apply(m, "esc")

	if m.ActiveView != ViewMigration || m.Mode != ModeTable || m.Focus != FocusList {
		t.Fatalf("cancel restored view=%s mode=%s focus=%s", m.ActiveView, m.Mode, m.Focus)
	}
	if m.Cursor != 1 || m.Scroll != 1 || len(m.rows()) != 2 {
		t.Fatalf("cancel restored cursor=%d scroll=%d rows=%d", m.Cursor, m.Scroll, len(m.rows()))
	}
	if !reflect.DeepEqual(m.Selected, wantSelected) {
		t.Fatalf("cancel restored selection=%v, want %v", m.Selected, wantSelected)
	}
}

func TestConfirmationCancelRestoresParentActionViewport(t *testing.T) {
	service := &fakeService{removePreview: app.MutationPreview{Summary: "Remove selected skills", RequiresConfirmation: true}}
	m := selectedLibraryModel(service)
	m.Mode, m.Focus = ModeMore, FocusActions
	m.ActionIndex = actionIndex(t, m, "Remove selected")
	m.OverlayScroll = 3
	wantAction, wantSelection := m.ActionIndex, mapsClone(m.Selected)

	next, preview := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if preview == nil {
		t.Fatal("remove preview command is nil")
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm {
		t.Fatalf("preview mode=%s, want confirm", m.Mode)
	}
	m, _ = apply(m, "esc")
	if m.Mode != ModeMore || m.Focus != FocusActions || m.ActionIndex != wantAction || m.OverlayScroll != 3 || !reflect.DeepEqual(m.Selected, wantSelection) {
		t.Fatalf("restored mode=%s focus=%s action=%d scroll=%d selected=%v", m.Mode, m.Focus, m.ActionIndex, m.OverlayScroll, m.Selected)
	}
}

func TestFilterDraftApplyAndCancelAreReversible(t *testing.T) {
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, &fakeMigration{})
	m.Filter, m.Cursor, m.Scroll = "a", 1, 1

	m, _ = apply(m, "/")
	if m.Mode != ModeFilter || m.FilterDraft != "a" {
		t.Fatalf("filter opened with mode=%s draft=%q", m.Mode, m.FilterDraft)
	}
	m, _ = apply(m, "backspace")
	m, _ = apply(m, "z")
	if m.Filter != "a" || len(m.rows()) != 0 {
		t.Fatalf("draft mutated applied filter=%q rows=%d", m.Filter, len(m.rows()))
	}
	m, _ = apply(m, "esc")
	if m.Mode != ModeTable || m.Filter != "a" || m.Cursor != 1 || m.Scroll != 1 || len(m.rows()) != 2 {
		t.Fatalf("cancel restored mode=%s filter=%q cursor=%d scroll=%d rows=%d", m.Mode, m.Filter, m.Cursor, m.Scroll, len(m.rows()))
	}

	m, _ = apply(m, "/")
	m, _ = apply(m, "backspace")
	m, _ = apply(m, "b")
	m, _ = apply(m, "e")
	m, _ = apply(m, "t")
	m, _ = apply(m, "a")
	if got := stripANSI(m.ViewString()); !strings.Contains(got, "Search: beta") {
		t.Fatalf("existing filter draft is not visible:\n%s", got)
	}
	m, _ = apply(m, "enter")
	if m.Filter != "beta" || len(m.rows()) != 1 || m.rows()[0].ID != "acme/beta" {
		t.Fatalf("apply filter=%q rows=%#v", m.Filter, m.rows())
	}
	if got := stripANSI(m.ViewString()); !strings.Contains(got, "Filter: beta") || !strings.Contains(got, "1 result") {
		t.Fatalf("applied filter state is not visible:\n%s", got)
	}
}

func TestActionsOnlyExposeValidOperations(t *testing.T) {
	t.Run("migration comparison requires a match", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewMigration, ActionNone)
		m.Inventory.Items = []app.ScanItem{{Key: "loose", Skill: config.Skill{ID: "local/loose", Name: "loose"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt}}
		if actions := m.primaryActions(); slices.Contains(actions, "Compare") {
			t.Fatalf("unmatched migration actions=%v", actions)
		}
		m.Inventory.Items[0].MatchedLibraryID = "acme/alpha"
		if actions := m.primaryActions(); !slices.Contains(actions, "Compare") {
			t.Fatalf("matched migration actions=%v", actions)
		}
	})

	t.Run("library batch requires selection", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
		m.Snapshot = testSnapshot()
		m.Mode = ModeMore
		for _, invalid := range []string{"Enable selected", "Disable selected", "Update selected", "Remove selected", "Change ref", "Clear selection"} {
			if actions := m.primaryActions(); slices.Contains(actions, invalid) {
				t.Fatalf("unselected library exposes %q in %v", invalid, actions)
			}
		}
		m.Selected["library:acme/alpha"] = true
		for _, valid := range []string{"Remove selected", "Change ref", "Clear selection"} {
			if actions := m.primaryActions(); !slices.Contains(actions, valid) {
				t.Fatalf("selected library hides %q in %v", valid, actions)
			}
		}
		if actions := m.primaryActions(); slices.Contains(actions, "Update selected") {
			t.Fatalf("library exposes update without complete update evidence: %v", actions)
		}
	})

	t.Run("preset member view hides preset-level actions", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewPresets, ActionNone)
		m.Snapshot = testSnapshot()
		m.Scope, m.Mode = Scope{Preset: "review", Level: "preset-skills"}, ModeMore
		if actions := m.primaryActions(); !reflect.DeepEqual(actions, []string{"Close"}) {
			t.Fatalf("preset member More actions=%v", actions)
		}
	})

	t.Run("status repair matches selected issue", func(t *testing.T) {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewStatus, ActionNone)
		m.Snapshot = testSnapshot()
		if actions := m.primaryActions(); slices.Contains(actions, "Sync preview") || !slices.Contains(actions, "Adopt") {
			t.Fatalf("unmanaged status actions=%v", actions)
		}
		m.Snapshot.Status.Items[0].Kind = status.Missing
		if actions := m.primaryActions(); !slices.Contains(actions, "Sync preview") || slices.Contains(actions, "Adopt") {
			t.Fatalf("missing status actions=%v", actions)
		}
	})
}

func TestEscapeOnlyNavigatesAndPlainQNeverQuits(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Detail, m.Focus = true, FocusDetail

	qModel, quit := apply(m, "q")
	if quit != nil || !qModel.Detail || qModel.Focus != FocusDetail || !strings.Contains(qModel.Status, "Ctrl+Q") {
		t.Fatalf("q changed page: cmd=%v detail=%v focus=%s status=%q", quit != nil, qModel.Detail, qModel.Focus, qModel.Status)
	}
	escaped, escCmd := apply(m, "esc")
	if escCmd != nil || escaped.Focus != FocusList {
		t.Fatalf("esc did not return focus: cmd=%v focus=%s", escCmd != nil, escaped.Focus)
	}
	escaped, escCmd = apply(escaped, "esc")
	if escCmd != nil || escaped.Detail {
		t.Fatalf("second esc did not close detail: cmd=%v detail=%v", escCmd != nil, escaped.Detail)
	}

	m.enterConfirm(ActionUpdate)
	confirmed, modalQuit := apply(m, "q")
	if modalQuit != nil || confirmed.Mode != ModeConfirm || !strings.Contains(confirmed.Status, "Ctrl+Q") {
		t.Fatalf("q escaped confirmation: cmd=%v mode=%s status=%q", modalQuit != nil, confirmed.Mode, confirmed.Status)
	}
}

func TestExitKeyMatrix(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot = testSnapshot()
	base.Selected["acme/alpha"] = true

	t.Run("root escape is a strict no-op", func(t *testing.T) {
		next, cmd := apply(base, "esc")
		if cmd != nil || next.ActiveView != base.ActiveView || next.Mode != base.Mode || next.Focus != base.Focus || !next.Selected["acme/alpha"] {
			t.Fatalf("root esc changed state: %+v cmd=%v", next, cmd != nil)
		}
	})
	t.Run("escape from actions and detail returns directly to collection", func(t *testing.T) {
		for _, focus := range []Focus{FocusActions, FocusDetail} {
			current := base
			current.Detail, current.Focus, current.ActionIndex = true, focus, 1
			next, cmd := apply(current, "esc")
			if cmd != nil || next.Focus != FocusList {
				t.Fatalf("esc from %s returned focus=%s cmd=%v", focus, next.Focus, cmd != nil)
			}
		}
	})
	t.Run("filter and input keep q as text", func(t *testing.T) {
		filter := base
		filter.beginFilter()
		filter, cmd := apply(filter, "q")
		if cmd != nil || filter.FilterDraft != "q" || filter.Mode != ModeFilter {
			t.Fatalf("filter q=%q mode=%s cmd=%v", filter.FilterDraft, filter.Mode, cmd != nil)
		}
		input := base
		input.enterInput(inputState{Prompt: "Value"})
		input, cmd = apply(input, "q")
		if cmd != nil || input.Input.Value != "q" || input.Mode != ModeInput {
			t.Fatalf("input q=%q mode=%s cmd=%v", input.Input.Value, input.Mode, cmd != nil)
		}
	})
	t.Run("ctrl q exits ordinary modal and busy read", func(t *testing.T) {
		for _, setup := range []func(*Model){
			func(m *Model) {},
			func(m *Model) { m.enterMore() },
			func(m *Model) { m.enterErrorDetail("details") },
			func(m *Model) { m.Busy = true },
		} {
			current := base
			setup(&current)
			_, cmd := apply(current, "ctrl+q")
			if !isQuit(cmd) {
				t.Fatalf("ctrl+q did not quit mode=%s busy=%v", current.Mode, current.Busy)
			}
		}
	})
	t.Run("mutation refuses normal quit but ctrl c is emergency", func(t *testing.T) {
		current := base
		current.Busy, current.MutationBusy = true, true
		next, cmd := apply(current, "ctrl+q")
		if cmd != nil || !next.Busy || !next.MutationBusy || !strings.Contains(strings.ToLower(next.Status), "operation in progress") {
			t.Fatalf("mutation accepted ctrl+q: busy=%v mutation=%v status=%q cmd=%v", next.Busy, next.MutationBusy, next.Status, cmd != nil)
		}
		_, cmd = apply(current, "ctrl+c")
		if !isQuit(cmd) {
			t.Fatal("mutation blocked emergency ctrl+c")
		}
	})
}

func TestExitKeysAcrossInteractiveStatesUseRealBubbleTeaKeys(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot = testSnapshot()
	states := []struct {
		name  string
		setup func(*Model)
	}{
		{"list", func(m *Model) { m.Focus = FocusList }},
		{"actions", func(m *Model) { m.Detail, m.Focus = true, FocusActions }},
		{"detail", func(m *Model) { m.Detail, m.Focus = true, FocusDetail }},
		{"filter", func(m *Model) { m.beginFilter() }},
		{"input", func(m *Model) { m.enterInput(inputState{Prompt: "Value"}) }},
		{"confirm", func(m *Model) { m.enterConfirm(ActionUpdate) }},
		{"more", func(m *Model) { m.enterMore() }},
		{"error-detail", func(m *Model) { m.enterErrorDetail("failure") }},
		{"busy", func(m *Model) { m.Busy = true }},
	}
	for _, state := range states {
		t.Run(state.name+"/ctrl-q", func(t *testing.T) {
			current := base
			state.setup(&current)
			_, cmd := current.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
			if !isQuit(cmd) {
				t.Fatalf("real ctrl+q did not quit state=%s", state.name)
			}
		})
	}

	mutation := base
	mutation.Busy, mutation.MutationBusy = true, true
	if _, cmd := mutation.Update(tea.KeyMsg{Type: tea.KeyCtrlQ}); isQuit(cmd) {
		t.Fatal("real ctrl+q quit during mutation")
	}
	if _, cmd := mutation.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Fatal("real ctrl+c did not perform emergency quit")
	}
}

func TestMigrationRowsExposeHumanIdentityBeforeAction(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewMigration, ActionNone)
	m.Width, m.Height = 100, 20
	m.Inventory.Items = []app.ScanItem{
		{Key: "opaque-111111111111111111111111111111111", Origin: "g/codex", Target: "/work/.codex/skills/duplicate", Skill: config.Skill{ID: "local/one", Name: "duplicate"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt},
		{Key: "opaque-222222222222222222222222222222222", Origin: "g/claude", Target: "/work/.claude/skills/duplicate", Skill: config.Skill{ID: "local/two", Name: "duplicate"}, State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt},
		{Key: "f9f92ac1897a40d7bc45d707dc7c1cd1", Origin: "g/codex", Target: "/work/.codex/skills/broken-skill", State: app.ScanStateBrokenLink},
	}

	rows := m.rows()
	if rows[2].Name != "broken-skill" {
		t.Fatalf("broken link name=%q, want target basename", rows[2].Name)
	}
	got := stripANSI(m.ViewString())
	for _, want := range []string{"g/codex", "g/claude", "Adopt"} {
		if !strings.Contains(got, want) {
			t.Fatalf("migration identity missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "f9f92ac1897a40d7bc45d707dc7c1cd1") {
		t.Fatalf("migration exposed opaque key:\n%s", got)
	}
}

func TestResponsiveMetricsRangeAndNavigationStayLegible(t *testing.T) {
	overview := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	overview.Snapshot, overview.Width, overview.Height = testSnapshot(), 38, 16
	plain := stripANSI(overview.ViewString())
	for _, metric := range []string{"Skills", "Updates", "Unmanaged", "Issues"} {
		if !strings.Contains(plain, metric) {
			t.Fatalf("38-column overview lost %q:\n%s", metric, plain)
		}
	}

	library := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	library.Width, library.Height = 38, 12
	for i := 0; i < 12; i++ {
		library.Snapshot.Config.Library.Skills = append(library.Snapshot.Config.Library.Skills, config.Skill{ID: "skill/" + string(rune('a'+i)), Name: "skill"})
	}
	if got := stripANSI(library.ViewString()); !strings.Contains(got, "/12") {
		t.Fatalf("scrollable list has no visible range:\n%s", got)
	}

	labels := layoutNavigation(ComputeLayout(80, 20), topViews, ViewLibrary)
	if len(labels) != 3 || labels[2].Label != "Presets" {
		t.Fatalf("compact navigation labels=%+v", labels)
	}

	overview.Width = 38
	if footer := overview.footer(); !strings.Contains(footer, "? Help") || !strings.Contains(footer, "Ctrl+Q Quit") || len([]rune(footer)) > overview.Width {
		t.Fatalf("38-column footer=%q", footer)
	}
	overview.Width = 24
	if footer := overview.footer(); !strings.Contains(footer, "?") || !strings.Contains(footer, "Ctrl+Q") || len([]rune(footer)) > overview.Width {
		t.Fatalf("24-column footer=%q", footer)
	}
	overview.Height = 8
	if got := stripANSI(overview.ViewString()); !strings.Contains(got, "Attention") || !strings.Contains(got, "loose") {
		t.Fatalf("24x8 overview lost the actionable row:\n%s", got)
	}
}

func TestConfirmationBodyWrapPreservesExactLongPath(t *testing.T) {
	path := "/Users/silenceper/workspace/aikit/.codex/skills/a-very-long-skill-name/SKILL.md"
	lines := wrapOverlayBody([]string{"Path: " + path}, 20)
	if got := strings.Join(lines, ""); got != "Path: "+path {
		t.Fatalf("wrapped path=%q, want exact %q", got, "Path: "+path)
	}

	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Width, m.Height, m.Mode, m.confirm = 38, 16, ModeConfirm, ActionRemoveSkill
	m.Preview = app.MutationPreview{Summary: "Review exact removal", Plan: link.Plan{Actions: []link.Action{{Path: path}}}}
	if got := stripANSI(m.ViewString()); strings.Contains(got, "Path: /Users/silenceper…") {
		t.Fatalf("confirmation still clips the path:\n%s", got)
	}
}

func TestInformationalRowsAndEmptyStatesReduceNoise(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	if got := stripANSI(m.renderRowLines(row{Name: "preset", State: "2 skills", Severity: rowSeverityInfo}, true, 40)[0]); strings.Contains(got, "[INFO]") {
		t.Fatalf("informational metadata rendered as a badge: %q", got)
	}
	m.Width, m.Height = 80, 16
	if got := stripANSI(m.ViewString()); !strings.Contains(got, "Add source") {
		t.Fatalf("empty library lacks next step:\n%s", got)
	}
}

func TestWideLayoutShowsContextualDetailAndKeepsActionsReachable(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	layout := ComputeLayout(m.Width, m.Height)
	if layout.Detail.Empty() || layout.List.Overlaps(layout.Detail) {
		t.Fatalf("wide layout has no usable detail pane: %+v", layout)
	}
	if got := stripANSI(m.ViewString()); !strings.Contains(got, "Summary") || !strings.Contains(got, "alpha") {
		t.Fatalf("wide contextual detail missing:\n%s", got)
	}
	m, _ = apply(m, "tab")
	if m.Focus != FocusCollectionActions {
		t.Fatalf("first Tab focus=%s, want collection actions", m.Focus)
	}
	m, _ = apply(m, "tab")
	if m.Focus != FocusDetail {
		t.Fatalf("second Tab focus=%s, want detail", m.Focus)
	}
	m, _ = apply(m, "tab")
	if m.Focus != FocusDetailActions {
		t.Fatalf("third Tab focus=%s, want detail actions", m.Focus)
	}
}

func TestHelpDocumentsAndScrollsEveryInputPath(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Width, m.Height, m.Help = 38, 12, true
	all := strings.Join(m.overlayLines(), "\n")
	for _, want := range []string{"clear", "PgUp/PgDn", "Mouse", "Esc", "Ctrl+Q quit"} {
		if !strings.Contains(all, want) {
			t.Fatalf("help copy missing %q:\n%s", want, all)
		}
	}
	before := m.OverlayScroll
	m, _ = apply(m, "down")
	if m.OverlayScroll <= before {
		t.Fatalf("help did not scroll: before=%d after=%d\n%s", before, m.OverlayScroll, m.ViewString())
	}
}

func moveCursorToPrefix(t *testing.T, m *Model, prefix string) {
	t.Helper()
	for index, current := range m.rows() {
		if strings.HasPrefix(current.Key, prefix) {
			m.Cursor = index
			return
		}
	}
	t.Fatalf("row with prefix %q not found in %#v", prefix, m.rows())
}

func mapsClone(source map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
