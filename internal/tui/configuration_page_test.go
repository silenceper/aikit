package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

func configurationNavigationRect(t *testing.T, m Model, label string) Rect {
	t.Helper()
	for _, item := range m.hitRegions().Navigation {
		if item.Entry.Label == label {
			return item.Rect
		}
	}
	t.Fatalf("navigation item %q not found", label)
	return Rect{}
}

func TestConfigurationValidationLifecycle(t *testing.T) {
	m := configurationModel(&fakeService{})
	if got := m.configurationValidationLabel(); got != "Not validated" {
		t.Fatalf("initial validation=%q", got)
	}

	next, _ := m.Update(configurationValidationMsg{result: app.ConfigurationValidation{Path: "/home/aikit/config.yaml", Valid: true}})
	m = next.(Model)
	if !m.ConfigValidationDisplay.Attempted || !m.ConfigValidationDisplay.Valid || m.configurationValidationLabel() != "Valid" || !strings.Contains(m.ConfigValidationDisplay.Message, "/home/aikit/config.yaml") {
		t.Fatalf("valid display=%+v label=%q", m.ConfigValidationDisplay, m.configurationValidationLabel())
	}

	next, _ = m.Update(configurationValidationMsg{err: errors.New("yaml: invalid projects entry")})
	m = next.(Model)
	if !m.ConfigValidationDisplay.Attempted || m.ConfigValidationDisplay.Valid || m.configurationValidationLabel() != "Invalid" || m.ConfigValidationDisplay.Message != "yaml: invalid projects entry" {
		t.Fatalf("invalid display=%+v label=%q", m.ConfigValidationDisplay, m.configurationValidationLabel())
	}

	m.switchDestination(navigationEntry{Key: "library", Label: "Library", Kind: navigationView, View: ViewLibrary})
	m.switchDestination(configurationNavigationEntry())
	if m.configurationValidationLabel() != "Invalid" || m.ConfigValidationDisplay.Message != "yaml: invalid projects entry" {
		t.Fatalf("validation state lost after route round trip: %+v", m.ConfigValidationDisplay)
	}
}

func TestConfigurationLateValidationResultKeepsRoute(t *testing.T) {
	service := &fakeService{configurationValidation: app.ConfigurationValidation{Path: "/config.yaml", Valid: true}}
	m := configurationModel(service)
	m, validateCmd := invokeConfigurationAction(t, m, "Validate", false)
	if validateCmd == nil || !m.Busy || m.Activity.Kind != ActivityReading {
		t.Fatalf("validate cmd=%v busy=%v activity=%v", validateCmd != nil, m.Busy, m.Activity.Kind)
	}

	library := configurationNavigationRect(t, m, "Library")
	next, navigationCmd := m.Update(click(library.X, library.Y))
	m = next.(Model)
	if navigationCmd != nil || m.ActiveView != ViewLibrary {
		t.Fatalf("validate navigation view=%s cmd=%v", m.ActiveView, navigationCmd != nil)
	}

	result := validateCmd()
	next, _ = m.Update(result)
	m = next.(Model)
	if m.ActiveView != ViewLibrary || !m.ConfigValidationDisplay.Valid {
		t.Fatalf("late validation route=%s display=%+v", m.ActiveView, m.ConfigValidationDisplay)
	}
}

func TestConfigurationLateReloadResultKeepsRoute(t *testing.T) {
	service := &fakeService{
		configuration: app.ConfigurationDetail{Config: "/reloaded/config.yaml", Library: "/reloaded/library", Cache: "/reloaded/cache"},
		snapshot:      testSnapshot(),
	}
	m := configurationModel(service)
	m, reloadCmd := invokeConfigurationAction(t, m, "Reload", false)
	if reloadCmd == nil || !m.Busy || m.Activity.Kind != ActivityReading {
		t.Fatalf("reload cmd=%v busy=%v activity=%v", reloadCmd != nil, m.Busy, m.Activity.Kind)
	}

	next, navigationCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(Model)
	if navigationCmd != nil || m.ActiveView != ViewPresets {
		t.Fatalf("reload keyboard navigation view=%s cmd=%v", m.ActiveView, navigationCmd != nil)
	}

	result := reloadCmd()
	next, follow := m.Update(result)
	m = next.(Model)
	if follow == nil || m.ActiveView != ViewPresets || m.Config.Config != "/reloaded/config.yaml" {
		t.Fatalf("late reload view=%s config=%q follow=%v", m.ActiveView, m.Config.Config, follow != nil)
	}

	next, _ = m.Update(follow())
	m = next.(Model)
	if m.ActiveView != ViewPresets {
		t.Fatalf("late reload snapshot changed route to %s", m.ActiveView)
	}
}

func TestConfigurationPagePresentationAndResponsiveActions(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 24}, {80, 16}, {38, 12}, {38, 8}} {
		m := configurationModel(&fakeService{})
		m.Width, m.Height = size.width, size.height
		view := stripANSI(m.ViewString())
		for _, want := range []string{"Configuration", "Config file", "Library", "Cache"} {
			if !strings.Contains(view, want) {
				t.Fatalf("size=%dx%d missing %q:\n%s", size.width, size.height, want, view)
			}
		}
		m.Cursor = 3
		m.ensureVisible()
		if validationView := stripANSI(m.ViewString()); !strings.Contains(validationView, "Not validated") {
			t.Fatalf("size=%dx%d validation row unreachable:\n%s", size.width, size.height, validationView)
		}
		if strings.Contains(view, "[Close]") || m.hasOverlay() {
			t.Fatalf("size=%dx%d rendered Configuration as overlay:\n%s", size.width, size.height, view)
		}
		for _, action := range []string{"Validate", "Reload", "Show paths"} {
			if actionIndex(t, m, action) < 0 {
				t.Fatalf("size=%dx%d action %q missing", size.width, size.height, action)
			}
		}
	}
}

func openConfigurationWithMouse(t *testing.T, service *fakeService) (Model, tea.Cmd) {
	t.Helper()
	m := NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	region := configurationNavigationRect(t, m, "Configuration")
	next, cmd := m.Update(click(region.X, region.Y))
	return next.(Model), cmd
}

func TestConfigurationNormalRoute(t *testing.T) {
	service := &fakeService{configuration: app.ConfigurationDetail{Config: "/config.yaml", Library: "/library", Cache: "/cache"}}
	m, cmd := openConfigurationWithMouse(t, service)
	if cmd == nil || m.ActiveView != ViewConfiguration || m.Mode != ModeTable || m.hasOverlay() {
		t.Fatalf("configuration route view=%s mode=%s overlay=%v cmd=%v", m.ActiveView, m.Mode, m.hasOverlay(), cmd != nil)
	}
	if !m.Busy || m.Activity.Kind != ActivityReading {
		t.Fatalf("configuration load busy=%v activity=%v", m.Busy, m.Activity.Kind)
	}

	m = NewModel(context.Background(), service, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 120, 24
	next, keyboardCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}, Alt: false})
	_ = next
	_ = keyboardCmd
	// Ctrl+K is represented explicitly because a plain "k" moves the cursor.
	next, keyboardCmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = next.(Model)
	if keyboardCmd == nil || m.ActiveView != ViewConfiguration || m.Mode != ModeTable || m.hasOverlay() {
		t.Fatalf("keyboard configuration route view=%s mode=%s overlay=%v cmd=%v", m.ActiveView, m.Mode, m.hasOverlay(), keyboardCmd != nil)
	}
}

func TestConfigurationNavigationWhileReading(t *testing.T) {
	service := &fakeService{configuration: app.ConfigurationDetail{Config: "/late/config.yaml", Library: "/late/library", Cache: "/late/cache"}}
	m, loadCmd := openConfigurationWithMouse(t, service)
	if loadCmd == nil || !m.Busy {
		t.Fatalf("configuration load cmd=%v busy=%v", loadCmd != nil, m.Busy)
	}

	configuration := configurationNavigationRect(t, m, "Configuration")
	next, duplicate := m.Update(click(configuration.X, configuration.Y))
	m = next.(Model)
	if duplicate != nil {
		t.Fatal("duplicate Configuration load command was submitted while reading")
	}

	library := configurationNavigationRect(t, m, "Library")
	next, navigationCmd := m.Update(click(library.X, library.Y))
	m = next.(Model)
	if navigationCmd != nil || m.ActiveView != ViewLibrary {
		t.Fatalf("reading navigation view=%s cmd=%v", m.ActiveView, navigationCmd != nil)
	}

	configuration = configurationNavigationRect(t, m, "Configuration")
	next, duplicate = m.Update(click(configuration.X, configuration.Y))
	m = next.(Model)
	if duplicate != nil || m.ActiveView != ViewConfiguration {
		t.Fatalf("reading re-entry view=%s duplicate cmd=%v", m.ActiveView, duplicate != nil)
	}

	result := loadCmd()
	if _, ok := result.(activityResultMsg); !ok {
		t.Fatalf("configuration load was not generation wrapped: %T", result)
	}
	next, _ = m.Update(result)
	m = next.(Model)
	if m.ActiveView != ViewConfiguration {
		t.Fatalf("late Configuration result changed route to %s", m.ActiveView)
	}
	if m.Config.Config != "/late/config.yaml" {
		t.Fatalf("late Configuration detail not cached: %+v", m.Config)
	}
}
