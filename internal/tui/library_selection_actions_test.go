package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func TestLibrarySelectionActions(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	if actions := m.librarySelectionActions(); len(actions) != 0 {
		t.Fatalf("zero selection actions=%+v", actions)
	}
	m.Selected["library:acme/alpha"] = true
	got := m.librarySelectionActions()
	want := []librarySelectionActionID{selectionEnable, selectionDisable, selectionUpdate, selectionRemove, selectionClear}
	if len(got) != len(want) {
		t.Fatalf("actions=%+v", got)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("action[%d]=%q want %q", i, got[i].ID, want[i])
		}
	}
	if got[0].Mnemonic != 'e' || got[1].Mnemonic != 'd' || got[2].Mnemonic != 'u' || got[3].Mnemonic != 'r' || got[4].Mnemonic != 'c' {
		t.Fatalf("mnemonics=%+v", got)
	}
}

func TestLibrarySelectionBarResponsiveGeometry(t *testing.T) {
	for _, width := range []int{24, 38, 59, 80, 120} {
		t.Run(string(rune(width)), func(t *testing.T) {
			m := selectedLibraryModel(&fakeService{})
			m.Width, m.Height = width, 20
			before := ComputeLayout(m.Width, m.Height)
			plain := stripANSI(m.ViewString())
			if !strings.Contains(plain, "2 selected") {
				t.Fatalf("width=%d missing contextual bar:\n%s", width, plain)
			}
			if got := m.collectionActions(); slices.Contains(got, "Add source") || slices.Contains(got, "More") {
				t.Fatalf("width=%d legacy collection actions remain: %v", width, got)
			}
			if after := ComputeLayout(m.Width, m.Height); !reflect.DeepEqual(before.CollectionPanel, after.CollectionPanel) || !reflect.DeepEqual(before.DetailPanel, after.DetailPanel) {
				t.Fatalf("width=%d selection changed pane geometry", width)
			}
			lines := strings.Split(plain, "\n")
			regions := m.hitRegions().CollectionActions
			if regions.Bar.Empty() || regions.Bar.Y >= len(lines) {
				t.Fatalf("width=%d missing action geometry: %+v", width, regions)
			}
			if lipgloss.Width(lines[regions.Bar.Y]) > width {
				t.Fatalf("width=%d overflow=%d row=%q", width, lipgloss.Width(lines[regions.Bar.Y]), lines[regions.Bar.Y])
			}
			for visible, rect := range regions.Buttons {
				if visible >= len(regions.Indexes) {
					t.Fatal("button/index mismatch")
				}
				for _, x := range []int{rect.X, rect.Right() - 1} {
					if !rect.Contains(x, rect.Y) {
						t.Fatalf("width=%d action=%d boundary cell=%d outside %+v", width, regions.Indexes[visible], x, rect)
					}
				}
			}
		})
	}
}

func TestLibrarySelectionBarEveryActionIsMouseReachableAt24Columns(t *testing.T) {
	m := selectedLibraryModel(&fakeService{})
	m.Width, m.Height = 24, 20
	want := len(m.librarySelectionActions())
	seen := make(map[int]bool)
	for step := 0; step < want*2; step++ {
		regions := m.hitRegions().CollectionActions
		for _, index := range regions.Indexes {
			seen[index] = true
		}
		if len(seen) == want {
			return
		}
		if regions.Next.Empty() {
			t.Fatalf("only reached %v without next control:\n%s", seen, m.ViewString())
		}
		next, cmd := m.Update(tea.MouseMsg{X: regions.Next.X, Y: regions.Next.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if cmd != nil {
			t.Fatal("page control returned command")
		}
		m = next.(Model)
	}
	t.Fatalf("only reached actions %v", seen)
}

func TestLibrarySelectionBarSurvivesPinnedBatchDetail(t *testing.T) {
	for _, width := range []int{24, 38, 59, 80} {
		m := selectedLibraryModel(&fakeService{})
		m.Width, m.Height = width, 20
		m.BatchResult = app.BatchResult{Issues: []app.OperationIssue{{Item: "acme/alpha", Message: "blocked"}}}
		regions := m.hitRegions()
		if regions.CollectionActions.Bar.Empty() || len(regions.CollectionActions.Indexes) == 0 {
			t.Fatalf("width=%d selection bar hidden by result detail: %+v\n%s", width, regions.CollectionActions, m.ViewString())
		}
		if !strings.Contains(stripANSI(m.ViewString()), "selected") {
			t.Fatalf("width=%d selection count hidden:\n%s", width, m.ViewString())
		}
	}
}

func TestLibrarySelectionMnemonicParity(t *testing.T) {
	keyboardService := &fakeService{}
	keyboard := selectedLibraryModel(keyboardService)
	keyboard.Activity, keyboard.Busy = Activity{}, false
	next, keyboardCmd := keyboard.Update(key("u"))
	keyboard = next.(Model)
	if keyboardCmd == nil || keyboardService.previewBatchCalls != 0 || keyboard.pendingBatch.Operation != app.BatchUpdate {
		t.Fatalf("keyboard pending=%+v calls=%d cmd=%v", keyboard.pendingBatch, keyboardService.previewBatchCalls, keyboardCmd != nil)
	}

	mouseService := &fakeService{}
	mouse := selectedLibraryModel(mouseService)
	mouse.Activity, mouse.Busy = Activity{}, false
	mouse.ActionIndex = 2
	regions := mouse.hitRegions().CollectionActions
	update := -1
	for visible, index := range regions.Indexes {
		if mouse.librarySelectionActions()[index].ID == selectionUpdate {
			update = visible
			break
		}
	}
	if update < 0 {
		t.Fatalf("Update is not visible: %+v", regions)
	}
	next, mouseCmd := mouse.Update(click(regions.Buttons[update].X, regions.Buttons[update].Y))
	mouse = next.(Model)
	if mouseCmd == nil || mouseService.previewBatchCalls != 0 || !reflect.DeepEqual(mouse.pendingBatch, keyboard.pendingBatch) {
		t.Fatalf("mouse pending=%+v keyboard=%+v calls=%d cmd=%v", mouse.pendingBatch, keyboard.pendingBatch, mouseService.previewBatchCalls, mouseCmd != nil)
	}
}

func TestDisabledLibrarySelectionActionReportsReasonWithoutBackendCall(t *testing.T) {
	for _, input := range []string{"keyboard", "mouse"} {
		t.Run(input, func(t *testing.T) {
			service := &fakeService{}
			m := selectedLibraryModel(service)
			m.Activity, m.Busy = Activity{}, false
			m.Snapshot.Config.Library.Skills[0].Source = ""
			var cmd tea.Cmd
			if input == "keyboard" {
				m, cmd = apply(m, "u")
			} else {
				m.ActionIndex = 2
				regions := m.hitRegions().CollectionActions
				visible := slices.Index(regions.Indexes, 2)
				if visible < 0 {
					t.Fatalf("disabled Update missing from hit regions: %+v", regions)
				}
				next, nextCmd := m.Update(click(regions.Buttons[visible].X, regions.Buttons[visible].Y))
				m, cmd = next.(Model), nextCmd
			}
			if cmd != nil || service.previewBatchCalls != 0 || service.batchCalls != 0 {
				t.Fatalf("disabled action called backend: preview=%d batch=%d cmd=%v", service.previewBatchCalls, service.batchCalls, cmd != nil)
			}
			if !strings.Contains(m.Status, "acme/alpha") || !strings.Contains(m.Status, "Git source") || m.Activity.Kind == ActivityWarning {
				t.Fatalf("disabled feedback status=%q activity=%+v", m.Status, m.Activity)
			}
		})
	}
}

func TestLibrarySelectionEscapeClosesDetailThenClearsSelection(t *testing.T) {
	m := selectedLibraryModel(&fakeService{})
	m.Activity, m.Busy = Activity{}, false
	m.Width = 59
	m.Detail, m.Focus = true, FocusDetail
	m, _ = apply(m, "esc")
	if m.Detail || m.librarySelectionCount() != 2 {
		t.Fatalf("first Esc detail=%v selected=%d", m.Detail, m.librarySelectionCount())
	}
	m, _ = apply(m, "esc")
	if m.librarySelectionCount() != 0 || m.ActiveView != ViewLibrary || m.Mode != ModeTable {
		t.Fatalf("second Esc selected=%d view=%s mode=%s", m.librarySelectionCount(), m.ActiveView, m.Mode)
	}
}

func TestLibraryBatchScopePickerOffersOnlySingleExactScopes(t *testing.T) {
	m := selectedLibraryModel(&fakeService{})
	m.Activity, m.Busy = Activity{}, false
	m, cmd := apply(m, "e")
	if cmd != nil || m.Mode != ModeScopePicker || m.Picker.Purpose != pickerLibraryBatchScope {
		t.Fatalf("mode=%s purpose=%s cmd=%v", m.Mode, m.Picker.Purpose, cmd != nil)
	}
	for _, choice := range m.Picker.Choices {
		if choice.Label == "All agents" || len(choice.Bindings) != 1 {
			t.Fatalf("non-exact choice=%+v", choice)
		}
	}
	index := -1
	for i, choice := range m.Picker.Choices {
		if choice.Label == "Global / codex" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("Global / codex missing: %+v", m.Picker.Choices)
	}
	m.Cursor = index
	m = m.chooseHighlightedPicker()
	next, previewCmd := m.applyPicker()
	m = next.(Model)
	if previewCmd == nil || len(m.pendingBatch.Bindings) != m.librarySelectionCount() {
		t.Fatalf("bindings=%+v cmd=%v", m.pendingBatch.Bindings, previewCmd != nil)
	}
	for _, binding := range m.pendingBatch.Bindings {
		if binding.Agent != "codex" || binding.Project != "" {
			t.Fatalf("binding is not one exact scope: %+v", binding)
		}
	}
}

func TestLibrarySelectionUpdateEligibility(t *testing.T) {
	base := func() Model {
		m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
		m.Snapshot = testSnapshot()
		m.Snapshot.Config.Library.Skills[0].Source = "https://example.test/acme/repo.git"
		m.Snapshot.Config.Library.Skills[0].Ref = &config.Ref{Kind: "branch", Value: "main"}
		m.Selected["library:acme/alpha"] = true
		return m
	}
	findUpdate := func(t *testing.T, m Model) librarySelectionAction {
		t.Helper()
		for _, action := range m.librarySelectionActions() {
			if action.ID == selectionUpdate {
				return action
			}
		}
		t.Fatal("missing update action")
		return librarySelectionAction{}
	}

	tests := []struct {
		name string
		edit func(*Model)
		want string
	}{
		{"local source", func(m *Model) { m.Snapshot.Config.Library.Skills[0].Source = "/tmp/local" }, "Git source"},
		{"missing ref", func(m *Model) { m.Snapshot.Config.Library.Skills[0].Ref = nil }, "branch ref"},
		{"tag ref", func(m *Model) { m.Snapshot.Config.Library.Skills[0].Ref = &config.Ref{Kind: "tag", Value: "v1"} }, "branch ref"},
		{"missing result", func(m *Model) { m.Snapshot.Updates.Results = nil }, "update-available"},
		{"check failed", func(m *Model) { m.Snapshot.Updates.Results[0].State = updatecheck.StateCheckFailed }, "update-available"},
		{"empty remote", func(m *Model) { m.Snapshot.Updates.Results[0].Remote = "" }, "remote"},
		{"stale current", func(m *Model) { m.Snapshot.Updates.Results[0].Current = "stale" }, "changed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base()
			tt.edit(&m)
			action := findUpdate(t, m)
			if action.Enabled || !strings.Contains(action.Reason, "acme/alpha") || !strings.Contains(action.Reason, tt.want) {
				t.Fatalf("action=%+v", action)
			}
		})
	}

	m := base()
	action := findUpdate(t, m)
	if !action.Enabled || action.Reason != "" {
		t.Fatalf("enabled action=%+v", action)
	}
	request, err := m.libraryBatchRequest(app.BatchUpdate)
	if err != nil {
		t.Fatal(err)
	}
	checked := m.Snapshot.Updates.Results[0]
	want := app.ExpectedUpdate{
		Ref:      &config.Ref{Kind: m.Snapshot.Config.Library.Skills[0].Ref.Kind, Value: m.Snapshot.Config.Library.Skills[0].Ref.Value},
		Resolved: checked.Current,
		Remote:   checked.Remote,
	}
	if !reflect.DeepEqual(request.Expected["acme/alpha"], want) {
		t.Fatalf("expected=%+v want %+v", request.Expected["acme/alpha"], want)
	}
}
