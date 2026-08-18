package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

func selectedLibraryModel(service *fakeService) Model {
	m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 22
	m.Selected["library:acme/alpha"] = true
	m.Selected["library:acme/beta"] = true
	m.Snapshot.Config.Library.Skills[0].Ref = &config.Ref{Kind: "branch", Value: "main"}
	m.Snapshot.Config.Library.Skills[1].Ref = &config.Ref{Kind: "branch", Value: "main"}
	m.Snapshot.Updates.Results = append(m.Snapshot.Updates.Results, updatecheck.Result{
		SkillID: "acme/beta", Current: strings.Repeat("b", 40), Remote: strings.Repeat("d", 40), State: updatecheck.StateUpdateAvailable,
	})
	return m
}

func typeRunes(m Model, value string) Model {
	for _, char := range value {
		m, _ = apply(m, string(char))
	}
	return m
}

func TestExplicitRefChangeIsConfirmedRollbackSafeAndKeyboardMouseEquivalent(t *testing.T) {
	invoke := func(t *testing.T, mouse bool) (Model, *fakeService) {
		t.Helper()
		service := &fakeService{}
		m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 22
		m.Selected["library:acme/alpha"] = true
		m.ActionPane = actionPaneDetail
		if mouse {
			m, _ = mouseAction(t, m, actionIndex(t, m, "More"))
			index := actionIndex(t, m, "Change ref")
			regions := m.hitRegions()
			next, _ := m.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
			return next.(Model), service
		}
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
		index := actionIndex(t, m, "Change ref")
		for i := 0; i < index; i++ {
			next, _ := m.Update(actionKey(tea.KeyRight))
			m = next.(Model)
		}
		next, _ := m.Update(actionKey(tea.KeyEnter))
		return next.(Model), service
	}

	for _, mouse := range []bool{false, true} {
		name := "keyboard"
		if mouse {
			name = "mouse"
		}
		t.Run(name, func(t *testing.T) {
			m, service := invoke(t, mouse)
			if m.Mode != ModeInput || m.Input.Kind != inputRefChange || service.updateCalls != 0 {
				t.Fatalf("ref input mode=%s kind=%s calls=%d", m.Mode, m.Input.Kind, service.updateCalls)
			}
			for _, char := range "tag:v2" {
				m, _ = apply(m, string(char))
			}
			m, cmd := apply(m, "enter")
			if cmd != nil || m.Mode != ModeConfirm || service.updateCalls != 0 {
				t.Fatalf("ref preview mode=%s cmd=%v calls=%d", m.Mode, cmd != nil, service.updateCalls)
			}
			wantRef := &config.Ref{Kind: "tag", Value: "v2"}
			if len(m.pendingUpdate.SkillIDs) != 1 || m.pendingUpdate.SkillIDs[0] != "acme/alpha" || !reflect.DeepEqual(m.pendingUpdate.Ref, wantRef) || m.pendingUpdate.Confirmed {
				t.Fatalf("pending ref update=%+v", m.pendingUpdate)
			}
			if !strings.Contains(strings.ToLower(m.ViewString()), "roll back") {
				t.Fatalf("ref confirmation did not disclose rollback:\n%s", m.ViewString())
			}
			cancelled, cancelCmd := apply(m, "esc")
			if cancelCmd != nil || service.updateCalls != 0 || cancelled.Mode == ModeConfirm {
				t.Fatalf("ref cancel mutated: cmd=%v calls=%d mode=%s", cancelCmd != nil, service.updateCalls, cancelled.Mode)
			}
			m, execute := apply(m, "enter")
			if execute == nil || service.updateCalls != 0 || !m.MutationBusy {
				t.Fatalf("ref confirm not deferred: cmd=%v calls=%d busy=%v", execute != nil, service.updateCalls, m.MutationBusy)
			}
			_ = execute()
			if service.updateCalls != 1 || !service.lastUpdate.Confirmed || !reflect.DeepEqual(service.lastUpdate.Ref, wantRef) {
				t.Fatalf("ref update calls=%d request=%+v", service.updateCalls, service.lastUpdate)
			}
		})
	}
}

func TestManualUpdateCheckSelectionConfirmCancelAndParity(t *testing.T) {
	report := updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "acme/alpha", Current: "old", Remote: "new", State: updatecheck.StateUpdateAvailable}}}
	invoke := func(t *testing.T, mouse bool) (Model, tea.Cmd, *fakeService) {
		t.Helper()
		service := &fakeService{updateResult: app.Result{Updates: report}}
		m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
		m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 22
		if mouse {
			m, _ = mouseAction(t, m, actionIndex(t, m, "More"))
			index := actionIndex(t, m, "Check updates")
			regions := m.hitRegions()
			next, cmd := m.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
			return next.(Model), cmd, service
		}
		m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
		index := actionIndex(t, m, "Check updates")
		for i := 0; i < index; i++ {
			next, _ := m.Update(actionKey(tea.KeyRight))
			m = next.(Model)
		}
		next, cmd := m.Update(actionKey(tea.KeyEnter))
		return next.(Model), cmd, service
	}

	keyboard, keyboardCmd, keyboardService := invoke(t, false)
	mouse, mouseCmd, mouseService := invoke(t, true)
	for name, result := range map[string]struct {
		m       Model
		cmd     tea.Cmd
		service *fakeService
	}{"keyboard": {keyboard, keyboardCmd, keyboardService}, "mouse": {mouse, mouseCmd, mouseService}} {
		t.Run(name, func(t *testing.T) {
			if result.cmd == nil || result.service.updateCalls != 0 {
				t.Fatalf("check not deferred: cmd=%v calls=%d", result.cmd != nil, result.service.updateCalls)
			}
			msg := result.cmd()
			if result.service.updateCalls != 1 || !result.service.lastUpdate.CheckOnly || !result.service.lastUpdate.Refresh || result.service.lastUpdate.Confirmed {
				t.Fatalf("check request=%+v calls=%d", result.service.lastUpdate, result.service.updateCalls)
			}
			next, _ := result.m.Update(msg)
			checked := next.(Model)
			if checked.Mode != ModeUpdates || len(checked.rows()) != 1 || checked.rows()[0].ID != "acme/alpha" {
				t.Fatalf("checked mode=%s rows=%+v", checked.Mode, checked.rows())
			}
			checked, _ = apply(checked, "space")
			checked, _ = apply(checked, "enter")
			if checked.Mode != ModeConfirm || result.service.updateCalls != 1 {
				t.Fatalf("update selection skipped confirmation: mode=%s calls=%d", checked.Mode, result.service.updateCalls)
			}
			cancelled, cancelCmd := apply(checked, "esc")
			if cancelCmd != nil || result.service.updateCalls != 1 || cancelled.Mode != ModeUpdates {
				t.Fatalf("cancel mutated: mode=%s calls=%d cmd=%v", cancelled.Mode, result.service.updateCalls, cancelCmd != nil)
			}
		})
	}
}

func TestManualUpdateCheckPersistsWarningsAndFailuresWithoutFalseSuccess(t *testing.T) {
	report := updatecheck.CheckReport{
		Warnings: []string{"cache write failed"},
		Results:  []updatecheck.Result{{SkillID: "acme/alpha", Current: "old", State: updatecheck.StateCheckFailed, Error: "remote unavailable"}},
	}
	service := &fakeService{updateResult: app.Result{Updates: report}}
	m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 22
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	index := actionIndex(t, m, "Check updates")
	for i := 0; i < index; i++ {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	got := m.ViewString()
	if m.Mode != ModeUpdates || !strings.Contains(got, "cache write failed") || !strings.Contains(got, "remote unavailable") || strings.Contains(m.Status, "0 update(s) available") {
		t.Fatalf("diagnostics lost or false success: mode=%s status=%q\n%s", m.Mode, m.Status, got)
	}
}

func TestLibraryStateAndSourceFiltersPreserveExactSelection(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Config.Library.Skills[1].Source = "/tmp/local-beta"
	m.Width, m.Height = 100, 20

	// Library rows are directly multi-selectable.
	next, cmd := m.Update(actionKey(tea.KeySpace))
	m = next.(Model)
	if cmd != nil || !m.Selected["library:acme/alpha"] {
		t.Fatalf("library selection cmd=%v selected=%v", cmd != nil, m.Selected)
	}

	m.LibraryStateFilter = LibraryStateUpdateAvailable
	rows := m.rows()
	if len(rows) != 1 || rows[0].ID != "acme/alpha" {
		t.Fatalf("update state filter rows=%+v", rows)
	}
	m.LibraryStateFilter = LibraryStateAll
	m.LibrarySourceFilter = LibrarySourceLocal
	rows = m.rows()
	if len(rows) != 1 || rows[0].ID != "acme/beta" {
		t.Fatalf("local source filter rows=%+v", rows)
	}
	if !m.Selected["library:acme/alpha"] {
		t.Fatalf("filter discarded hidden exact selection: %v", m.Selected)
	}
	m.LibrarySourceFilter = LibrarySourceGit
	rows = m.rows()
	if len(rows) != 1 || rows[0].ID != "acme/alpha" {
		t.Fatalf("git source filter rows=%+v", rows)
	}
}

func TestLibraryBatchActionsUseOneConfirmedAppBatchCallWithParity(t *testing.T) {
	tests := []struct {
		label string
		op    app.BatchOperation
	}{
		{"Update selected", app.BatchUpdate},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			keyboardService := &fakeService{}
			keyboard := selectedLibraryModel(keyboardService)
			keyboard, _ = keyboardAction(t, keyboard, actionIndex(t, keyboard, "More"))
			index := actionIndex(t, keyboard, tt.label)
			for i := 0; i < index; i++ {
				next, _ := keyboard.Update(actionKey(tea.KeyRight))
				keyboard = next.(Model)
			}
			next, cmd := keyboard.Update(actionKey(tea.KeyEnter))
			keyboard = next.(Model)
			if cmd == nil || keyboard.Mode != ModeMore || keyboardService.batchCalls != 0 || keyboard.pendingBatch.Operation != tt.op {
				t.Fatalf("keyboard preview mode=%s op=%s calls=%d cmd=%v", keyboard.Mode, keyboard.pendingBatch.Operation, keyboardService.batchCalls, cmd != nil)
			}
			next, _ = keyboard.Update(cmd())
			keyboard = next.(Model)
			if keyboard.Mode != ModeConfirm || keyboardService.previewBatchCalls != 1 {
				t.Fatalf("keyboard preview mode=%s calls=%d", keyboard.Mode, keyboardService.previewBatchCalls)
			}

			mouseService := &fakeService{}
			mouse := selectedLibraryModel(mouseService)
			mouse, _ = mouseAction(t, mouse, actionIndex(t, mouse, "More"))
			regions := mouse.hitRegions()
			next, cmd = mouse.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
			mouse = next.(Model)
			if cmd == nil || mouseService.batchCalls != 0 || !reflect.DeepEqual(mouse.pendingBatch, keyboard.pendingBatch) {
				t.Fatalf("mouse preview=%+v keyboard=%+v calls=%d cmd=%v", mouse.pendingBatch, keyboard.pendingBatch, mouseService.batchCalls, cmd != nil)
			}
			next, _ = mouse.Update(cmd())
			mouse = next.(Model)

			cancelled, cancelCmd := keyboard.Update(actionKey(tea.KeyEsc))
			if cancelCmd != nil || keyboardService.batchCalls != 0 || cancelled.(Model).Mode == ModeConfirm {
				t.Fatalf("cancel mutated: calls=%d mode=%s cmd=%v", keyboardService.batchCalls, cancelled.(Model).Mode, cancelCmd != nil)
			}

			next, execute := mouse.Update(actionKey(tea.KeyEnter))
			mouse = next.(Model)
			if execute == nil || mouseService.batchCalls != 0 || !mouse.MutationBusy {
				t.Fatalf("confirm not deferred: cmd=%v calls=%d busy=%v", execute != nil, mouseService.batchCalls, mouse.MutationBusy)
			}
			_, duplicate := mouse.Update(actionKey(tea.KeyEnter))
			if duplicate != nil {
				t.Fatal("MutationBusy accepted duplicate submit")
			}
			_ = execute()
			if mouseService.batchCalls != 1 || mouseService.lastBatch.Operation != tt.op || !mouseService.lastBatch.Confirmed {
				t.Fatalf("batch calls=%d request=%+v", mouseService.batchCalls, mouseService.lastBatch)
			}
			if tt.op == app.BatchEnable || tt.op == app.BatchDisable {
				if len(mouseService.lastBatch.Bindings) != 2 {
					t.Fatalf("binding batch=%+v", mouseService.lastBatch.Bindings)
				}
			} else if len(mouseService.lastBatch.SkillIDs) != 2 {
				t.Fatalf("skill batch=%+v", mouseService.lastBatch.SkillIDs)
			}
		})
	}
}

func TestBatchEnableDisableRequireExactScopeChoiceAndPreview(t *testing.T) {
	tests := []struct {
		label       string
		choice      string
		wantAgent   string
		wantProject string
		wantScope   string
	}{
		{"Enable selected", "Global / codex", "codex", "", "Global / codex"},
		{"Disable selected", "Project / aikit / Common", "", "aikit", "Project / aikit / common"},
		{"Enable selected", "Project / aikit / codex", "codex", "aikit", "Project / aikit / codex"},
	}
	for _, tt := range tests {
		t.Run(tt.label+"_"+tt.choice, func(t *testing.T) {
			service := &fakeService{batchPreview: app.BatchPreview{MutationPreview: app.MutationPreview{Title: "Batch preview", Summary: "Exact target", AffectedScopes: []config.Scope{{Project: tt.wantProject, Agent: tt.wantAgent}}, RequiresConfirmation: true}}}
			m := selectedLibraryModel(service)
			m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
			index := actionIndex(t, m, tt.label)
			for i := 0; i < index; i++ {
				next, _ := m.Update(actionKey(tea.KeyRight))
				m = next.(Model)
			}
			next, cmd := m.Update(actionKey(tea.KeyEnter))
			m = next.(Model)
			if cmd != nil || m.Mode != ModeScopePicker || service.previewBatchCalls != 0 || service.batchCalls != 0 {
				t.Fatalf("scope choice mode=%s cmd=%v preview=%d batch=%d", m.Mode, cmd != nil, service.previewBatchCalls, service.batchCalls)
			}
			m, preview := chooseRowByName(t, m, tt.choice, false)
			if preview == nil || service.previewBatchCalls != 0 {
				t.Fatalf("binding preview cmd=%v calls=%d", preview != nil, service.previewBatchCalls)
			}
			next, _ = m.Update(preview())
			m = next.(Model)
			if service.previewBatchCalls != 1 || m.Mode != ModeConfirm || len(m.pendingBatch.Bindings) != 2 || !strings.Contains(m.ViewString(), tt.wantScope) {
				t.Fatalf("preview calls=%d mode=%s bindings=%+v\n%s", service.previewBatchCalls, m.Mode, m.pendingBatch.Bindings, m.ViewString())
			}
			for _, binding := range m.pendingBatch.Bindings {
				if binding.Agent != tt.wantAgent || binding.Project != tt.wantProject {
					t.Fatalf("guessed binding scope: %+v", binding)
				}
			}
			cancelled, cancelCmd := apply(m, "esc")
			if cancelCmd != nil || service.batchCalls != 0 || cancelled.Mode == ModeConfirm {
				t.Fatalf("scope batch cancel mutated: cmd=%v calls=%d mode=%s", cancelCmd != nil, service.batchCalls, cancelled.Mode)
			}
		})
	}
}

func TestBatchScopeChoiceEntryKeyboardMouseParity(t *testing.T) {
	for _, label := range []string{"Enable selected", "Disable selected"} {
		base := selectedLibraryModel(&fakeService{})
		keyboard, _ := keyboardAction(t, base, actionIndex(t, base, "More"))
		index := actionIndex(t, keyboard, label)
		for i := 0; i < index; i++ {
			next, _ := keyboard.Update(actionKey(tea.KeyRight))
			keyboard = next.(Model)
		}
		next, keyboardCmd := keyboard.Update(actionKey(tea.KeyEnter))
		keyboard = next.(Model)

		mouse := selectedLibraryModel(&fakeService{})
		mouse, _ = mouseAction(t, mouse, actionIndex(t, mouse, "More"))
		regions := mouse.hitRegions()
		next, mouseCmd := mouse.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
		mouse = next.(Model)
		if keyboardCmd != nil || mouseCmd != nil || keyboard.Mode != mouse.Mode || keyboard.Input.Kind != mouse.Input.Kind || keyboard.pendingBatch.Operation != mouse.pendingBatch.Operation {
			t.Fatalf("scope choice parity keyboard=(%s,%s,%s) mouse=(%s,%s,%s)", keyboard.Mode, keyboard.Input.Kind, keyboard.pendingBatch.Operation, mouse.Mode, mouse.Input.Kind, mouse.pendingBatch.Operation)
		}
	}
}

func TestBatchRemoveAggregatesPreviewsAndRequiresSecondForceConfirmation(t *testing.T) {
	service := &fakeService{
		batchPreview: app.BatchPreview{MutationPreview: app.MutationPreview{Title: "Remove selected", Summary: "Exact references", References: []string{"agent:codex", "project:aikit:common"}, RequiresForce: true, RequiresConfirmation: true}},
	}
	m := selectedLibraryModel(service)
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	index := actionIndex(t, m, "Remove selected")
	for i := 0; i < index; i++ {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, preview := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if preview == nil || service.previewBatchCalls != 0 || service.batchCalls != 0 {
		t.Fatalf("remove preview cmd=%v previewCalls=%d batchCalls=%d", preview != nil, service.previewBatchCalls, service.batchCalls)
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	if service.previewBatchCalls != 1 || m.Mode != ModeConfirm || m.pendingBatch.Force {
		t.Fatalf("preview calls=%d mode=%s pending=%+v", service.previewBatchCalls, m.Mode, m.pendingBatch)
	}
	for _, wanted := range []string{"agent:codex", "project:aikit:common"} {
		if !strings.Contains(m.ViewString(), wanted) {
			t.Fatalf("aggregated preview missing %q:\n%s", wanted, m.ViewString())
		}
	}

	next, first := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if first != nil || service.batchCalls != 0 || m.Mode != ModeConfirm || m.pendingBatch.Force || m.confirm != ActionForceBatchRemove {
		t.Fatalf("first confirm force=%v action=%s cmd=%v calls=%d", m.pendingBatch.Force, m.confirm, first != nil, service.batchCalls)
	}
	next, execute := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if execute == nil || service.batchCalls != 0 || !m.MutationBusy {
		t.Fatalf("second confirm cmd=%v calls=%d busy=%v", execute != nil, service.batchCalls, m.MutationBusy)
	}
	_ = execute()
	if service.batchCalls != 1 || !service.lastBatch.Force {
		t.Fatalf("forced batch calls=%d request=%+v", service.batchCalls, service.lastBatch)
	}
}

func TestBatchUpdateRejectsHiddenInvalidSelectionBeforeConfirmation(t *testing.T) {
	service := &fakeService{}
	m := selectedLibraryModel(service)
	// Hide beta while retaining its exact selection, then corrupt its update token.
	m.LibraryStateFilter = LibraryStateUpdateAvailable
	for i := range m.Snapshot.Updates.Results {
		if m.Snapshot.Updates.Results[i].SkillID == "acme/beta" {
			m.Snapshot.Updates.Results[i].Remote = ""
		}
	}
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	if strings.Contains(strings.Join(m.primaryActions(), " "), "Update selected") || m.Mode != ModeMore || service.batchCalls != 0 {
		t.Fatalf("invalid hidden update exposed: mode=%s actions=%v calls=%d", m.Mode, m.primaryActions(), service.batchCalls)
	}
}

func TestBatchUpdateExpectedContainsFullCurrentRemoteAndRef(t *testing.T) {
	service := &fakeService{}
	m := selectedLibraryModel(service)
	m, _ = keyboardAction(t, m, actionIndex(t, m, "More"))
	index := actionIndex(t, m, "Update selected")
	for i := 0; i < index; i++ {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd == nil || m.Mode != ModeMore {
		t.Fatalf("valid update mode=%s cmd=%v err=%q", m.Mode, cmd != nil, m.Err)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.Mode != ModeConfirm || service.previewBatchCalls != 1 {
		t.Fatalf("valid update preview mode=%s calls=%d", m.Mode, service.previewBatchCalls)
	}
	for _, skillID := range []string{"acme/alpha", "acme/beta"} {
		expected, ok := m.pendingBatch.Expected[skillID]
		if !ok || expected.Ref == nil || expected.Ref.Kind == "" || expected.Ref.Value == "" || expected.Resolved == "" || expected.Remote == "" {
			t.Fatalf("incomplete Expected[%s]=%+v", skillID, expected)
		}
	}
}

func TestBatchResultItemsAndIssuesRemainTypedAndVisible(t *testing.T) {
	service := &fakeService{batchResult: app.BatchResult{
		Result: app.Result{Changed: true, Exit: app.ExitPartial},
		Items:  []app.BatchItemResult{{Item: "acme/alpha", Changed: true}, {Item: "acme/beta", Issue: &app.OperationIssue{Operation: "enable", Item: "acme/beta", Path: "/work/beta", Message: "occupied"}}},
		Issues: []app.OperationIssue{{Operation: "enable", Item: "acme/beta", Path: "/work/beta", Message: "occupied"}},
	}}
	m := selectedLibraryModel(service)
	request := app.BatchRequest{Operation: app.BatchEnable, Bindings: []app.BindingRequest{
		{SkillID: "acme/alpha", Agent: "codex"}, {SkillID: "acme/beta", Agent: "codex"},
	}}
	m.pendingBatch, m.confirm, m.Mode = request, ActionBatch, ModeConfirm
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("batch confirmation returned no command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.BatchResult.Items) != 2 || len(m.BatchResult.Issues) != 1 {
		t.Fatalf("typed batch result lost: %+v", m.BatchResult)
	}
	for _, wanted := range []string{"acme/alpha", "acme/beta", "/work/beta", "occupied"} {
		if !strings.Contains(m.ViewString(), wanted) {
			t.Fatalf("batch detail missing %q:\n%s", wanted, m.ViewString())
		}
	}
}

func TestBatchResultAndErrorRemainVisibleWithoutClaimingCompletion(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 30
	m.Mode, m.MutationBusy, m.Busy = ModeConfirm, true, true
	issueErr := errors.New("destination is occupied")
	result := app.BatchResult{
		Result: app.Result{Changed: true, Exit: app.ExitPartial},
		Items: []app.BatchItemResult{
			{Item: "acme/alpha", Changed: true},
			{Item: "acme/beta", Issue: &app.OperationIssue{Operation: "enable", Item: "acme/beta", Path: "/work/beta", Message: "occupied", Err: issueErr}},
		},
		Issues: []app.OperationIssue{{Operation: "enable", Item: "acme/beta", Path: "/work/beta", Message: "occupied", Err: issueErr}},
	}

	next, cmd := m.Update(batchOperationMsg{name: "enable", result: result, err: errors.New("batch stopped")})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("failed batch must not start a snapshot before its result is reviewed")
	}
	if !reflect.DeepEqual(m.BatchResult, result) {
		t.Fatalf("typed batch result lost when error is present: got=%+v want=%+v", m.BatchResult, result)
	}
	view := m.ViewString()
	for _, wanted := range []string{"acme/alpha", "acme/beta", "/work/beta", "occupied", "batch stopped"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("failed batch detail missing %q:\n%s", wanted, view)
		}
	}
	if strings.Contains(view, "acme/alpha:Completed") || strings.Contains(view, "Enable completed") {
		t.Fatalf("failed batch is misleadingly reported as completed:\n%s", view)
	}
}

func TestBatchFailureItemStateUsesAggregateChangedAsCommitAuthority(t *testing.T) {
	for _, tt := range []struct {
		name             string
		aggregateChanged bool
		want             string
		reject           string
	}{
		{name: "transaction-not-committed", aggregateChanged: false, want: "acme/alpha: Attempted", reject: "Changed"},
		{name: "changes-before-failure", aggregateChanged: true, want: "acme/alpha: Changed before failure", reject: "acme/alpha: Attempted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 30
			result := app.BatchResult{
				Result: app.Result{Changed: tt.aggregateChanged, Exit: app.ExitPartial},
				Items:  []app.BatchItemResult{{Item: "acme/alpha", Changed: true}},
			}
			next, _ := m.Update(batchOperationMsg{name: "update", result: result, err: errors.New("batch failed")})
			view := next.(Model).ViewString()
			if !strings.Contains(view, tt.want) || strings.Contains(view, tt.reject) {
				t.Fatalf("aggregate Changed=%v must render %q and not %q:\n%s", tt.aggregateChanged, tt.want, tt.reject, view)
			}
		})
	}
}

func TestLibraryFilterActionsUseRealKeyboardAndMouseRegistryPaths(t *testing.T) {
	for _, label := range []string{"State filter", "Source filter"} {
		t.Run(label, func(t *testing.T) {
			base := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			base.Snapshot, base.Width, base.Height = testSnapshot(), 100, 20
			keyboard, _ := keyboardAction(t, base, actionIndex(t, base, "More"))
			index := actionIndex(t, keyboard, label)
			for i := 0; i < index; i++ {
				next, _ := keyboard.Update(actionKey(tea.KeyRight))
				keyboard = next.(Model)
			}
			next, keyboardCmd := keyboard.Update(actionKey(tea.KeyEnter))
			keyboard = next.(Model)

			mouse, _ := mouseAction(t, base, actionIndex(t, base, "More"))
			regions := mouse.hitRegions()
			next, mouseCmd := mouse.Update(click(regions.Actions[index].X, regions.Actions[index].Y))
			mouse = next.(Model)
			if keyboardCmd != nil || mouseCmd != nil || keyboard.LibraryStateFilter != mouse.LibraryStateFilter || keyboard.LibrarySourceFilter != mouse.LibrarySourceFilter {
				t.Fatalf("filter parity keyboard=(%s,%s) mouse=(%s,%s)", keyboard.LibraryStateFilter, keyboard.LibrarySourceFilter, mouse.LibraryStateFilter, mouse.LibrarySourceFilter)
			}
		})
	}
}

func TestLibraryCheckboxMouseMatchesSpaceSelection(t *testing.T) {
	base := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 100, 20
	keyboard, _ := base.Update(actionKey(tea.KeySpace))
	regions := base.hitRegions()
	mouse, cmd := base.Update(click(regions.Checkboxes[0].X, regions.Checkboxes[0].Y))
	if cmd != nil || !reflect.DeepEqual(keyboard.(Model).Selected, mouse.(Model).Selected) {
		t.Fatalf("selection parity keyboard=%v mouse=%v cmd=%v", keyboard.(Model).Selected, mouse.(Model).Selected, cmd != nil)
	}
}
