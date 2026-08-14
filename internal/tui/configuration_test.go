package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/silenceper/aikit/internal/app"
)

func configurationModel(service *fakeService) Model {
	m := NewModel(nil, service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 100, 24
	m.Mode = ModeConfiguration
	m.Config = app.ConfigurationDetail{Config: "/home/aikit/config.yaml", Library: "/home/aikit/library", Cache: "/home/aikit/cache"}
	return m
}

func invokeConfigurationAction(t *testing.T, m Model, label string, mouse bool) (Model, tea.Cmd) {
	t.Helper()
	index := actionIndex(t, m, label)
	if mouse {
		region := m.hitRegions().Actions[index]
		next, cmd := m.Update(click(region.X, region.Y))
		return next.(Model), cmd
	}
	for m.ActionIndex != index {
		next, _ := m.Update(actionKey(tea.KeyRight))
		m = next.(Model)
	}
	next, cmd := m.Update(actionKey(tea.KeyEnter))
	return next.(Model), cmd
}

func TestConfigurationReadOnlyActionsKeyboardMouseParity(t *testing.T) {
	for _, label := range []string{"Validate", "Reload", "Show path", "Close"} {
		for _, mouse := range []bool{false, true} {
			t.Run(label+map[bool]string{false: "-keyboard", true: "-mouse"}[mouse], func(t *testing.T) {
				service := &fakeService{configuration: app.ConfigurationDetail{Config: "/new/config.yaml", Library: "/new/library", Cache: "/new/cache"}, configurationValidation: app.ConfigurationValidation{Path: "/home/aikit/config.yaml", Valid: true}}
				m, cmd := invokeConfigurationAction(t, configurationModel(service), label, mouse)
				switch label {
				case "Validate":
					if cmd == nil || service.validateConfigurationCalls != 0 || !m.Busy {
						t.Fatalf("Validate cmd=%v calls=%d busy=%v", cmd != nil, service.validateConfigurationCalls, m.Busy)
					}
					next, _ := m.Update(cmd())
					m = next.(Model)
					if service.validateConfigurationCalls != 1 || !strings.Contains(m.ViewString(), "Configuration valid") {
						t.Fatalf("validation result missing:\n%s", m.ViewString())
					}
				case "Reload":
					if cmd == nil || service.configurationCalls != 0 || service.snapshotCalls != 0 || !m.Busy {
						t.Fatalf("Reload cmd=%v config=%d snapshot=%d busy=%v", cmd != nil, service.configurationCalls, service.snapshotCalls, m.Busy)
					}
					next, follow := m.Update(cmd())
					m = next.(Model)
					if follow == nil {
						t.Fatal("Reload did not sequence offline snapshot")
					}
					next, _ = m.Update(follow())
					m = next.(Model)
					if service.configurationCalls != 1 || service.snapshotCalls != 1 || !service.lastSnapshot.Offline || service.lastSnapshot.ForceRefresh {
						t.Fatalf("Reload config=%d snapshot=%d request=%+v", service.configurationCalls, service.snapshotCalls, service.lastSnapshot)
					}
				case "Show path":
					if cmd != nil || m.Mode != ModeErrorDetail || !strings.Contains(m.ViewString(), "/home/aikit/config.yaml") || strings.Contains(strings.ToLower(m.Status), "copied") {
						t.Fatalf("Show path invalid:\n%s", m.ViewString())
					}
				case "Close":
					if cmd != nil || m.Mode != ModeTable {
						t.Fatalf("Close mode=%s cmd=%v", m.Mode, cmd != nil)
					}
				}
			})
		}
	}
}

func TestConfigurationActionsExposeNoClipboardOrEditor(t *testing.T) {
	m := configurationModel(&fakeService{})
	actions := m.primaryActions()
	if !reflect.DeepEqual(actions, []string{"Validate", "Reload", "Show path", "Close"}) {
		t.Fatalf("configuration actions=%v", actions)
	}
	joined := strings.ToLower(strings.Join(actions, " "))
	for _, forbidden := range []string{"copy", "edit", "editor"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("configuration exposed unavailable %q action: %v", forbidden, actions)
		}
	}
}

func TestConfigurationShowPathDetailCloseKeyboardMouseParity(t *testing.T) {
	for _, closeWithMouse := range []bool{false, true} {
		name := map[bool]string{false: "escape", true: "mouse-close"}[closeWithMouse]
		t.Run(name, func(t *testing.T) {
			m, cmd := invokeConfigurationAction(t, configurationModel(&fakeService{}), "Show path", true)
			if cmd != nil || m.Mode != ModeErrorDetail {
				t.Fatalf("Show path mode=%s cmd=%v", m.Mode, cmd != nil)
			}
			if actions := m.primaryActions(); !reflect.DeepEqual(actions, []string{"Close"}) {
				t.Fatalf("error detail actions=%v", actions)
			}
			if !strings.Contains(stripANSI(m.ViewString()), "Close") {
				t.Fatalf("error detail did not render Close:\n%s", m.ViewString())
			}

			var next tea.Model
			if closeWithMouse {
				regions := m.hitRegions()
				if len(regions.Actions) != 1 {
					t.Fatalf("error detail mouse actions=%d", len(regions.Actions))
				}
				next, cmd = m.Update(click(regions.Actions[0].X, regions.Actions[0].Y))
			} else {
				next, cmd = m.Update(actionKey(tea.KeyEsc))
			}
			m = next.(Model)
			if cmd != nil || m.Mode != ModeConfiguration {
				t.Fatalf("Close mode=%s cmd=%v, want configuration", m.Mode, cmd != nil)
			}
		})
	}
}

func TestFullErrorDetailMouseCloseReturnsToParentOverlay(t *testing.T) {
	m := configurationModel(&fakeService{})
	m.Mode, m.errorDetailParent, m.FullError = ModeErrorDetail, ModeMore, strings.Repeat("complete error detail ", 200)
	regions := m.hitRegions()
	if len(regions.Actions) != 1 || !strings.Contains(stripANSI(m.ViewString()), "[Close]") {
		t.Fatalf("full error Close unavailable: actions=%d\n%s", len(regions.Actions), m.ViewString())
	}
	next, cmd := m.Update(click(regions.Actions[0].X, regions.Actions[0].Y))
	m = next.(Model)
	if cmd != nil || m.Mode != ModeMore || m.FullError != "" {
		t.Fatalf("mouse Close mode=%s fullError=%q cmd=%v", m.Mode, m.FullError, cmd != nil)
	}
}

func TestWrapTextPreservesGraphemesAndDisplayWidth(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		width int
	}{
		{"ascii", "alpha-beta-gamma", 5},
		{"cjk", "配置路径错误详情", 6},
		{"emoji-combining", "A界👩‍💻e\u0301B🚀尾", 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lines := wrapText(tt.value, tt.width)
			if got := strings.Join(lines, ""); got != tt.value {
				t.Fatalf("wrapped content=%q, want exact %q", got, tt.value)
			}
			for _, line := range lines {
				if width := lipgloss.Width(line); width > tt.width {
					t.Fatalf("line %q width=%d > %d", line, width, tt.width)
				}
				if strings.HasSuffix(line, "\u200d") || strings.HasPrefix(line, "\u0301") {
					t.Fatalf("split grapheme boundary in %q", line)
				}
			}
		})
	}
}

func TestErrorDetailBodyScrollKeyboardAndMouseReachesEveryWrappedLine(t *testing.T) {
	parts := make([]string, 0, 18)
	for i := 0; i < 18; i++ {
		parts = append(parts, fmt.Sprintf("segment-%02d界👩‍💻e\u0301/", i))
	}
	m := configurationModel(&fakeService{})
	m.Width, m.Height, m.Mode = 40, 8, ModeErrorDetail
	m.FullError = strings.Join(parts, "") + "FINAL段🚀"
	layout := ComputeLayout(m.Width, m.Height)
	panel := layoutOverlayPanel(layout, m.overlayPanelActions(), true, m.ActionIndex)
	wantLines := wrapText(m.FullError, panel.Body.Width)
	initialRegions := m.hitRegions()
	if len(initialRegions.Actions) != 1 || !strings.Contains(stripANSI(m.ViewString()), "Error details") {
		t.Fatalf("fixed title/action missing:\n%s", m.ViewString())
	}

	seen := make(map[string]bool, len(wantLines))
	for step := 0; step < len(wantLines)+3; step++ {
		view := stripANSI(m.ViewString())
		for _, line := range wantLines {
			if strings.Contains(view, line) {
				seen[line] = true
			}
		}
		next, cmd := m.Update(actionKey(tea.KeyDown))
		if cmd != nil {
			t.Fatal("error detail Down returned command")
		}
		m = next.(Model)
	}
	for _, line := range wantLines {
		if !seen[line] {
			t.Fatalf("wrapped line never reachable %q at scroll=%d/%d:\n%s", line, m.OverlayScroll, len(wantLines), m.ViewString())
		}
	}
	if m.OverlayScroll == 0 || !strings.Contains(stripANSI(m.ViewString()), "FINAL段🚀") {
		t.Fatalf("final error detail unreachable at scroll=%d:\n%s", m.OverlayScroll, m.ViewString())
	}
	finalRegions := m.hitRegions()
	if finalRegions.Actions[0].Y != initialRegions.Actions[0].Y || !strings.Contains(stripANSI(m.ViewString()), "[Close]") {
		t.Fatalf("action bar moved while body scrolled: initial=%+v final=%+v\n%s", initialRegions.Actions[0], finalRegions.Actions[0], m.ViewString())
	}

	before := m.OverlayScroll
	next, _ := m.Update(actionKey(tea.KeyPgUp))
	m = next.(Model)
	if m.OverlayScroll >= before {
		t.Fatalf("PageUp did not scroll back: %d -> %d", before, m.OverlayScroll)
	}
	before = m.OverlayScroll
	next, _ = m.Update(tea.MouseMsg{X: panel.Body.X, Y: panel.Body.Y, Button: tea.MouseButtonWheelDown})
	m = next.(Model)
	if m.OverlayScroll <= before {
		t.Fatalf("mouse wheel did not scroll body: %d -> %d", before, m.OverlayScroll)
	}
}
