package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

func readingActivityModel() Model {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Width, m.Height = 100, 28
	m.Busy = true
	m.Activity = Activity{Kind: ActivityReading, Label: "Scanning local inventory"}
	return m
}

func TestReadingActivityAllowsKeyboardBrowsingButRejectsSubmissions(t *testing.T) {
	m := readingActivityModel()

	next, cmd := apply(m, "down")
	if cmd != nil || next.Cursor != 1 {
		t.Fatalf("reading activity blocked list browsing: cursor=%d cmd=%v", next.Cursor, cmd != nil)
	}
	next, cmd = apply(next, "1")
	if cmd != nil || next.ActiveView != ViewOverview {
		t.Fatalf("reading activity blocked navigation: view=%s cmd=%v", next.ActiveView, cmd != nil)
	}
	next, _ = apply(next, "2")
	next, _ = apply(next, "/")
	next, _ = apply(next, "a")
	if next.Mode != ModeFilter || next.FilterDraft != "a" {
		t.Fatalf("reading activity blocked local filter: mode=%s draft=%q", next.Mode, next.FilterDraft)
	}
	next, _ = apply(next, "enter")
	if next.Mode != ModeTable || next.Filter != "a" {
		t.Fatalf("reading activity did not apply local filter: mode=%s filter=%q", next.Mode, next.Filter)
	}

	before := next
	for _, key := range []string{" ", "r", "u", "a", "S"} {
		got, rejected := apply(before, key)
		if rejected != nil || len(got.Selected) != len(before.Selected) || got.Mode != before.Mode || got.ActiveView != before.ActiveView {
			t.Fatalf("reading activity accepted submission %q: mode=%s view=%s selected=%v cmd=%v", key, got.Mode, got.ActiveView, got.Selected, rejected != nil)
		}
	}
}

func TestReadingActivityMouseBrowsingRejectsSelectionAndActions(t *testing.T) {
	m := readingActivityModel()
	regions := m.hitRegions()
	if len(regions.Rows) < 2 {
		t.Fatalf("need two visible rows, got %d", len(regions.Rows))
	}
	second := regions.Rows[1]
	nextModel, cmd := m.updateMouse(tea.MouseMsg{X: second.X + second.Width - 1, Y: second.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	next := nextModel.(Model)
	if cmd != nil || next.Cursor != 1 || len(next.Selected) != 0 {
		t.Fatalf("row browse mismatch: cursor=%d selected=%v cmd=%v", next.Cursor, next.Selected, cmd != nil)
	}

	regions = next.hitRegions()
	if len(regions.Checkboxes) == 0 {
		t.Fatal("expected checkbox hit region")
	}
	box := regions.Checkboxes[0]
	blockedModel, blockedCmd := next.updateMouse(tea.MouseMsg{X: box.X, Y: box.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	blocked := blockedModel.(Model)
	if blockedCmd != nil || len(blocked.Selected) != 0 {
		t.Fatalf("reading activity accepted mouse selection: selected=%v cmd=%v", blocked.Selected, blockedCmd != nil)
	}

	for _, item := range regions.Navigation {
		if item.Entry.Kind == navigationView && item.Entry.View == ViewOverview {
			navModel, navCmd := blocked.updateMouse(tea.MouseMsg{X: item.Rect.X, Y: item.Rect.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			nav := navModel.(Model)
			if navCmd != nil || nav.ActiveView != ViewOverview {
				t.Fatalf("reading activity blocked mouse navigation: view=%s cmd=%v", nav.ActiveView, navCmd != nil)
			}
			return
		}
	}
	t.Fatal("overview navigation region not found")
}

func TestMutatingActivityStillStrictlyLocksBrowsing(t *testing.T) {
	m := readingActivityModel()
	m.MutationBusy = true
	m.Activity = Activity{Kind: ActivityMutating, Label: "Applying changes"}
	next, cmd := apply(m, "down")
	if cmd != nil || next.Cursor != m.Cursor {
		t.Fatalf("mutation gate allowed keyboard browsing: cursor=%d cmd=%v", next.Cursor, cmd != nil)
	}
	regions := m.hitRegions()
	row := regions.Rows[1]
	nextModel, mouseCmd := m.updateMouse(tea.MouseMsg{X: row.X, Y: row.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	mouse := nextModel.(Model)
	if mouseCmd != nil || mouse.Cursor != m.Cursor {
		t.Fatalf("mutation gate allowed mouse browsing: cursor=%d cmd=%v", mouse.Cursor, mouseCmd != nil)
	}
}

func TestReviewableActivityIsAConditionalKeyboardFocusTarget(t *testing.T) {
	m := readingActivityModel()
	m.Busy = false
	m.Activity = Activity{Kind: ActivityWarning, Label: "Batch completed with issues", Review: ReviewTarget{Kind: ReviewBatchResult, Key: "batch:9"}}
	m.BatchResult = app.BatchResult{Issues: []app.OperationIssue{{Item: "acme/alpha", Path: "/tmp/alpha", Message: "permission denied"}}}

	found := false
	for range 10 {
		next, _ := apply(m, "tab")
		m = next
		if m.Focus == FocusStatus {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("reviewable activity was not reachable with Tab")
	}
	next, cmd := apply(m, "enter")
	if cmd != nil || next.Mode != ModeErrorDetail || !strings.Contains(next.FullError, "acme/alpha") || !strings.Contains(next.FullError, "permission denied") {
		t.Fatalf("status review did not open exact typed batch result: mode=%s detail=%q cmd=%v", next.Mode, next.FullError, cmd != nil)
	}

	m.Activity.Review = ReviewTarget{}
	for _, focus := range m.visibleFocusOrder() {
		if focus == FocusStatus {
			t.Fatal("non-reviewable activity exposed status focus")
		}
	}
}

func TestReviewableActivityMouseFocusThenEnterMatchesKeyboard(t *testing.T) {
	m := readingActivityModel()
	m.Busy = false
	m.Err = "remote update check failed: timeout"
	m.Activity = Activity{Kind: ActivityError, Label: "Update check failed", Review: ReviewTarget{Kind: ReviewFullError, Key: "update:4"}}
	regions := m.hitRegions()
	if regions.ActivityStatus.Empty() {
		t.Fatal("missing activity status hit target")
	}
	clickedModel, cmd := m.updateMouse(tea.MouseMsg{X: regions.ActivityStatus.X, Y: regions.ActivityStatus.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	clicked := clickedModel.(Model)
	if cmd != nil || clicked.Focus != FocusStatus || clicked.Mode != ModeTable {
		t.Fatalf("activity click should focus without opening: focus=%s mode=%s cmd=%v", clicked.Focus, clicked.Mode, cmd != nil)
	}
	opened, _ := apply(clicked, "enter")
	if opened.Mode != ModeErrorDetail || opened.FullError != m.Err {
		t.Fatalf("mouse-focused review mismatch: mode=%s detail=%q", opened.Mode, opened.FullError)
	}
}
