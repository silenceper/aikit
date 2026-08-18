package tui

import (
	"reflect"
	"strings"
	"testing"

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
