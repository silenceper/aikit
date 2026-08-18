package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func enterRemoveConfirmFromMore(t *testing.T, service *fakeService) Model {
	t.Helper()
	m := selectedLibraryModel(service)
	m.Mode = ModeMore
	m.Focus, m.ActionIndex, m.OverlayScroll = FocusActions, actionIndex(t, m, "Remove selected"), 4
	next, preview := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if preview == nil {
		t.Fatal("Remove selected returned no preview command")
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || m.Focus != FocusActions || m.ActionIndex != 0 || m.OverlayScroll != 0 || !strings.Contains(stripANSI(m.ViewString()), "{Confirm}") {
		t.Fatalf("remove confirm inherited modal state: mode=%s focus=%s action=%d scroll=%d\n%s", m.Mode, m.Focus, m.ActionIndex, m.OverlayScroll, m.ViewString())
	}
	return m
}

func TestModalConfirmEnterUsesVisibleActionAndMouseParity(t *testing.T) {
	keyboardService := &fakeService{}
	keyboard := enterRemoveConfirmFromMore(t, keyboardService)
	next, _ := keyboard.Update(actionKey(tea.KeyRight))
	keyboard = next.(Model)
	if keyboard.ActionIndex != 1 || !strings.Contains(stripANSI(keyboard.ViewString()), "{Cancel}") {
		t.Fatalf("Cancel not visibly selected: action=%d\n%s", keyboard.ActionIndex, keyboard.ViewString())
	}
	next, cmd := keyboard.Update(actionKey(tea.KeyEnter))
	keyboard = next.(Model)
	if cmd != nil || keyboardService.batchCalls != 0 || keyboard.MutationBusy || keyboard.Mode == ModeConfirm {
		t.Fatalf("Cancel Enter mutated: mode=%s busy=%v cmd=%v calls=%d", keyboard.Mode, keyboard.MutationBusy, cmd != nil, keyboardService.batchCalls)
	}

	mouseService := &fakeService{}
	mouse := enterRemoveConfirmFromMore(t, mouseService)
	regions := mouse.hitRegions()
	next, cmd = mouse.Update(click(regions.Cancel.X, regions.Cancel.Y))
	mouse = next.(Model)
	if cmd != nil || mouseService.batchCalls != 0 || mouse.MutationBusy || mouse.Mode != keyboard.Mode {
		t.Fatalf("mouse Cancel mismatch: mode=%s busy=%v cmd=%v calls=%d", mouse.Mode, mouse.MutationBusy, cmd != nil, mouseService.batchCalls)
	}

	keyboardService = &fakeService{}
	keyboard = enterRemoveConfirmFromMore(t, keyboardService)
	next, keyboardCmd := keyboard.Update(actionKey(tea.KeyEnter))
	keyboard = next.(Model)
	mouseService = &fakeService{}
	mouse = enterRemoveConfirmFromMore(t, mouseService)
	regions = mouse.hitRegions()
	next, mouseCmd := mouse.Update(click(regions.Confirm.X, regions.Confirm.Y))
	mouse = next.(Model)
	if keyboardCmd == nil || mouseCmd == nil || !keyboard.MutationBusy || !mouse.MutationBusy || !reflect.DeepEqual(keyboard.pendingBatch, mouse.pendingBatch) {
		t.Fatalf("Confirm parity keyboard=(cmd=%v busy=%v request=%+v) mouse=(cmd=%v busy=%v request=%+v)", keyboardCmd != nil, keyboard.MutationBusy, keyboard.pendingBatch, mouseCmd != nil, mouse.MutationBusy, mouse.pendingBatch)
	}
	_ = keyboardCmd()
	_ = mouseCmd()
	if keyboardService.batchCalls != 1 || mouseService.batchCalls != 1 || !reflect.DeepEqual(keyboardService.lastBatch, mouseService.lastBatch) {
		t.Fatalf("Confirm request mismatch keyboard=%+v mouse=%+v", keyboardService.lastBatch, mouseService.lastBatch)
	}
}

func TestModalInputAndConfigurationResetLeakedMoreState(t *testing.T) {
	service := &fakeService{}
	m := selectedLibraryModel(service)
	delete(m.Selected, "library:acme/beta")
	m.Mode = ModeMore
	m.Focus, m.ActionIndex, m.OverlayScroll = FocusActions, actionIndex(t, m, "Change ref"), 5
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd != nil || m.Mode != ModeInput || m.Focus != FocusActions || m.ActionIndex != 0 || m.OverlayScroll != 0 || !strings.Contains(stripANSI(m.ViewString()), "{Apply}") {
		t.Fatalf("input inherited More state: mode=%s focus=%s action=%d scroll=%d cmd=%v\n%s", m.Mode, m.Focus, m.ActionIndex, m.OverlayScroll, cmd != nil, m.ViewString())
	}
	m.Input.Value = "branch:next"
	next, _ = m.Update(actionKey(tea.KeyTab))
	m = next.(Model)
	if m.ActionIndex != 1 || !strings.Contains(stripANSI(m.ViewString()), "{Cancel}") {
		t.Fatalf("input Cancel not selected: action=%d\n%s", m.ActionIndex, m.ViewString())
	}
	next, cmd = m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if cmd != nil || service.updateCalls != 0 || m.Mode == ModeInput {
		t.Fatalf("input Cancel Enter mutated: mode=%s cmd=%v calls=%d", m.Mode, cmd != nil, service.updateCalls)
	}

	mouseService := &fakeService{}
	mouse := selectedLibraryModel(mouseService)
	delete(mouse.Selected, "library:acme/beta")
	mouse.enterMore()
	mouse.ActionIndex, mouse.OverlayScroll = actionIndex(t, mouse, "Change ref"), 5
	regions := mouse.hitRegions()
	visible := -1
	for i, index := range regions.ActionIndexes {
		if index == mouse.ActionIndex {
			visible = i
			break
		}
	}
	if visible < 0 {
		t.Fatalf("Change ref has no mouse region: indexes=%v", regions.ActionIndexes)
	}
	next, cmd = mouse.Update(click(regions.Actions[visible].X, regions.Actions[visible].Y))
	mouse = next.(Model)
	if cmd != nil || mouse.Mode != ModeInput || mouse.ActionIndex != 0 || mouse.OverlayScroll != 0 {
		t.Fatalf("mouse input entry mode=%s action=%d scroll=%d cmd=%v", mouse.Mode, mouse.ActionIndex, mouse.OverlayScroll, cmd != nil)
	}
	regions = mouse.hitRegions()
	next, cmd = mouse.Update(click(regions.Cancel.X, regions.Cancel.Y))
	mouse = next.(Model)
	if cmd != nil || mouseService.updateCalls != 0 || mouse.Mode != m.Mode {
		t.Fatalf("mouse input Cancel mismatch: mode=%s cmd=%v calls=%d", mouse.Mode, cmd != nil, mouseService.updateCalls)
	}

	m = selectedLibraryModel(service)
	m.Mode, m.Focus, m.ActionIndex, m.OverlayScroll = ModeTable, FocusActions, 7, 6
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = next.(Model)
	if cmd == nil || m.Mode != ModeConfiguration || m.Focus != FocusActions || m.ActionIndex != 0 || m.OverlayScroll != 0 || !strings.Contains(stripANSI(m.ViewString()), "{Validate}") {
		t.Fatalf("configuration inherited state: mode=%s focus=%s action=%d scroll=%d cmd=%v\n%s", m.Mode, m.Focus, m.ActionIndex, m.OverlayScroll, cmd != nil, m.ViewString())
	}
}

func TestPresetSavePreviewEntersFreshConfirmModal(t *testing.T) {
	service := &fakeService{}
	m := NewModel(nil, service, &fakeMigration{}, ViewPresets, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 22
	m.Scope = Scope{Preset: "review", Level: "preset-skills"}
	m.Selected["acme/alpha"] = true
	m.Focus, m.ActionPane, m.ActionIndex, m.OverlayScroll = FocusCollectionActions, actionPaneCollection, 0, 7
	next, preview := m.Update(actionKey(tea.KeyEnter))
	m = next.(Model)
	if preview == nil {
		t.Fatal("Save returned no preview command")
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || m.Focus != FocusActions || m.ActionIndex != 0 || m.OverlayScroll != 0 || !strings.Contains(stripANSI(m.ViewString()), "{Confirm}") {
		t.Fatalf("Save confirm inherited action state: mode=%s focus=%s action=%d scroll=%d\n%s", m.Mode, m.Focus, m.ActionIndex, m.OverlayScroll, m.ViewString())
	}
}
