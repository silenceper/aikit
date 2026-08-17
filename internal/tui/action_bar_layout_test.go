package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderedActionBarClickBoundariesAndWhitespace(t *testing.T) {
	for _, width := range []int{100, 38} {
		name := "wide"
		if width < 60 {
			name = "narrow"
		}
		t.Run(name, func(t *testing.T) {
			base := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
			base.Snapshot, base.Width, base.Height = testSnapshot(), width, 20
			if width < 60 {
				base.Detail = true
			}
			plain := strings.Split(stripANSI(base.ViewString()), "\n")
			regions := base.hitRegions()
			y := regions.ActionBar.Y
			if y < 0 || y >= len(plain) {
				t.Fatalf("action row y=%d outside %d rendered lines", y, len(plain))
			}
			line := plain[y]
			actions := base.primaryActions()
			starts := make([]int, len(actions))
			ends := make([]int, len(actions))
			for i, label := range actions {
				button := "[" + label + "]"
				byteStart := strings.Index(line, button)
				if byteStart < 0 {
					t.Fatalf("rendered action %q missing from row %q", label, line)
				}
				starts[i] = lipgloss.Width(line[:byteStart])
				ends[i] = starts[i] + lipgloss.Width(button) - 1
				for _, x := range []int{starts[i], ends[i]} {
					m := base
					next, cmd := m.Update(click(x, y))
					assertLibraryActionTriggered(t, label, x, y, next.(Model), cmd)
				}
			}

			for i := 0; i+1 < len(actions); i++ {
				for x := ends[i] + 1; x < starts[i+1]; x++ {
					assertActionBarClickNoOp(t, base, x, y)
				}
			}
			if x := ends[len(ends)-1] + 1; x < width {
				assertActionBarClickNoOp(t, base, x, y)
			}
		})
	}
}

func TestNarrowConfigurationActionViewportKeepsEveryFocusedActionReachable(t *testing.T) {
	for wanted, label := range []string{"Validate", "Reload", "Show paths", "Close"} {
		m := configurationModel(&fakeService{})
		m.Width, m.Height, m.ActionIndex = 24, 12, wanted
		plain := stripANSI(m.ViewString())
		if !strings.Contains(plain, "["+label+"]") && !strings.Contains(plain, "{"+label+"}") {
			t.Fatalf("focused action %d %q hidden:\n%s", wanted, label, m.ViewString())
		}
		regions := m.hitRegions()
		visible := -1
		for i, actionIndex := range regions.ActionIndexes {
			if actionIndex == wanted {
				visible = i
				break
			}
		}
		if visible < 0 {
			t.Fatalf("focused action %d missing hit region: indexes=%v", wanted, regions.ActionIndexes)
		}
		next, cmd := m.Update(click(regions.Actions[visible].X, regions.Actions[visible].Y))
		got := next.(Model)
		switch label {
		case "Validate", "Reload":
			if cmd == nil || !got.Busy {
				t.Fatalf("%s mouse action cmd=%v busy=%v", label, cmd != nil, got.Busy)
			}
		case "Show paths":
			if cmd != nil || got.Mode != ModeErrorDetail {
				t.Fatalf("Show path mode=%s cmd=%v", got.Mode, cmd != nil)
			}
		case "Close":
			if cmd != nil || got.Mode != ModeTable {
				t.Fatalf("Close mode=%s cmd=%v", got.Mode, cmd != nil)
			}
		}
	}
}

func TestNarrowConfigurationActionViewportIsFullyMouseReachableFromDefault(t *testing.T) {
	for _, label := range []string{"Validate", "Reload", "Show paths", "Close"} {
		t.Run(label, func(t *testing.T) {
			m := configurationModel(&fakeService{})
			m.Width, m.Height = 24, 12
			if m.ActionIndex != 0 {
				t.Fatalf("default action index=%d, want 0", m.ActionIndex)
			}
			wanted := actionIndex(t, m, label)
			for steps := 0; steps < len(m.primaryActions()); steps++ {
				regions := m.hitRegions()
				for visible, index := range regions.ActionIndexes {
					if index != wanted {
						continue
					}
					next, cmd := m.Update(click(regions.Actions[visible].X, regions.Actions[visible].Y))
					got := next.(Model)
					switch label {
					case "Validate", "Reload":
						if cmd == nil || !got.Busy {
							t.Fatalf("%s mouse result cmd=%v busy=%v", label, cmd != nil, got.Busy)
						}
					case "Show paths":
						if cmd != nil || got.Mode != ModeErrorDetail {
							t.Fatalf("Show path mode=%s cmd=%v", got.Mode, cmd != nil)
						}
					case "Close":
						if cmd != nil || got.Mode != ModeTable {
							t.Fatalf("Close mode=%s cmd=%v", got.Mode, cmd != nil)
						}
					}
					return
				}
				if regions.ActionNext.Empty() || !strings.Contains(stripANSI(m.ViewString()), ">") {
					t.Fatalf("%s unavailable from action viewport: indexes=%v\n%s", label, regions.ActionIndexes, m.ViewString())
				}
				next, cmd := m.Update(click(regions.ActionNext.X, regions.ActionNext.Y))
				if cmd != nil {
					t.Fatalf("Next unexpectedly returned a command")
				}
				m = next.(Model)
			}
			t.Fatalf("%s never became mouse-visible", label)
		})
	}

	m := configurationModel(&fakeService{})
	m.Width, m.Height = 24, 12
	regions := m.hitRegions()
	next, _ := m.Update(click(regions.ActionNext.X, regions.ActionNext.Y))
	m = next.(Model)
	regions = m.hitRegions()
	if regions.ActionPrev.Empty() || m.ActionIndex == 0 {
		t.Fatalf("Next did not expose Prev: index=%d prev=%+v", m.ActionIndex, regions.ActionPrev)
	}
	next, _ = m.Update(click(regions.ActionPrev.X, regions.ActionPrev.Y))
	m = next.(Model)
	if m.ActionIndex != 0 {
		t.Fatalf("Prev returned action index=%d, want 0", m.ActionIndex)
	}
}

func TestNarrowCollectionActionViewportMouseWheelMovesVisibleWindow(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 24, 20
	regions := m.hitRegions()
	if regions.ActionNext.Empty() {
		t.Fatalf("narrow action bar has no Next control:\n%s", m.ViewString())
	}
	x, y := regions.ActionBar.X, regions.ActionBar.Y
	next, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown})
	m = next.(Model)
	if cmd != nil || m.ActionIndex != 1 {
		t.Fatalf("wheel down index=%d cmd=%v", m.ActionIndex, cmd != nil)
	}
	regions = m.hitRegions()
	visible := false
	for _, index := range regions.ActionIndexes {
		visible = visible || index == m.ActionIndex
	}
	if !visible {
		t.Fatalf("selected action not visible after wheel: selected=%d indexes=%v", m.ActionIndex, regions.ActionIndexes)
	}
	next, cmd = m.Update(tea.MouseMsg{X: regions.ActionBar.X, Y: regions.ActionBar.Y, Button: tea.MouseButtonWheelUp})
	m = next.(Model)
	if cmd != nil || m.ActionIndex != 0 {
		t.Fatalf("wheel up index=%d cmd=%v", m.ActionIndex, cmd != nil)
	}
}

func TestPagedActionBarSeparatorOutsideAndBusyClicksAreNoOp(t *testing.T) {
	m := configurationModel(&fakeService{})
	m.Width, m.Height = 24, 12
	regions := m.hitRegions()
	if len(regions.Actions) == 0 || regions.ActionNext.Empty() {
		t.Fatalf("paged controls unavailable: actions=%d next=%+v", len(regions.Actions), regions.ActionNext)
	}
	for _, point := range []struct{ x, y int }{
		{regions.Actions[0].Right(), regions.ActionBar.Y},
		{regions.ActionBar.Right(), regions.ActionBar.Y},
		{regions.ActionBar.X, regions.ActionBar.Y - 1},
	} {
		next, cmd := m.Update(click(point.x, point.y))
		got := next.(Model)
		if cmd != nil || got.ActionIndex != m.ActionIndex || got.Mode != m.Mode {
			t.Fatalf("blank click (%d,%d) changed mode=%s index=%d cmd=%v", point.x, point.y, got.Mode, got.ActionIndex, cmd != nil)
		}
	}

	m.Busy = true
	next, cmd := m.Update(click(regions.ActionNext.X, regions.ActionNext.Y))
	got := next.(Model)
	if cmd != nil || got.ActionIndex != 0 {
		t.Fatalf("Busy accepted Next click: index=%d cmd=%v", got.ActionIndex, cmd != nil)
	}
}

func TestShortMoreActionViewportKeyboardAndMouseReachEveryVisibleItem(t *testing.T) {
	for _, height := range []int{8, 12} {
		t.Run(fmt.Sprintf("height-%d", height), func(t *testing.T) {
			m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewStatus, ActionNone)
			m.Snapshot, m.Width, m.Height, m.Mode = testSnapshot(), 80, height, ModeMore
			actions := m.primaryActions()
			for wanted, label := range actions {
				for m.ActionIndex < wanted {
					next, _ := m.Update(actionKey(tea.KeyDown))
					m = next.(Model)
				}
				regions := m.hitRegions()
				found := false
				for _, index := range regions.ActionIndexes {
					found = found || index == wanted
				}
				if !found || !strings.Contains(stripANSI(m.ViewString()), label) {
					t.Fatalf("More action %d %q hidden: selected=%d scroll=%d indexes=%v\n%s", wanted, label, m.ActionIndex, m.OverlayScroll, regions.ActionIndexes, m.ViewString())
				}
				if len(regions.Actions) != len(regions.ActionIndexes) {
					t.Fatalf("visible action rect/index mismatch: %d/%d", len(regions.Actions), len(regions.ActionIndexes))
				}
			}

			m.ActionIndex, m.OverlayScroll = 0, 0
			layout := ComputeLayout(m.Width, m.Height)
			next, _ := m.Update(tea.MouseMsg{X: layout.Overlay.X, Y: layout.Overlay.Y + 1, Button: tea.MouseButtonWheelDown})
			mouse := next.(Model)
			if mouse.ActionIndex != 1 || mouse.OverlayScroll > mouse.ActionIndex {
				t.Fatalf("More mouse wheel selected=%d scroll=%d", mouse.ActionIndex, mouse.OverlayScroll)
			}
		})
	}
}

func assertLibraryActionTriggered(t *testing.T, label string, x, y int, got Model, cmd tea.Cmd) {
	t.Helper()
	switch label {
	case "Open":
		if cmd == nil || got.pendingDetailID != "acme/alpha" {
			t.Fatalf("Open boundary (%d,%d) did not trigger detail: cmd=%v pending=%q", x, y, cmd != nil, got.pendingDetailID)
		}
	case "Add source":
		if cmd != nil || got.Mode != ModeInput || got.Input.Kind != inputAddSource {
			t.Fatalf("Add boundary result mode=%s input=%s cmd=%v", got.Mode, got.Input.Kind, cmd != nil)
		}
	case "More":
		if cmd != nil || got.Mode != ModeMore {
			t.Fatalf("More boundary result mode=%s cmd=%v", got.Mode, cmd != nil)
		}
	default:
		t.Fatalf("unexpected action %q", label)
	}
}

func assertActionBarClickNoOp(t *testing.T, base Model, x, y int) {
	t.Helper()
	next, cmd := base.Update(click(x, y))
	got := next.(Model)
	if cmd != nil || got.Mode != base.Mode || got.Focus != base.Focus || got.ActionIndex != base.ActionIndex || got.pendingDetailID != "" {
		t.Fatalf("action-bar whitespace (%d,%d) changed state: mode=%s focus=%s action=%d cmd=%v", x, y, got.Mode, got.Focus, got.ActionIndex, cmd != nil)
	}
}
