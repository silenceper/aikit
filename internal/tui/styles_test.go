package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSemanticThemeModesKeepFocusAndSeveritySignals(t *testing.T) {
	modes := []themeMode{themeDark, themeLight, themeReduced, themeNoColor}
	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			theme := newSemanticTheme(mode)
			focus := stripANSI(theme.focused("Library"))
			if !strings.Contains(focus, ">") || !strings.Contains(focus, "Library") {
				t.Fatalf("focus lacks non-color signal: %q", focus)
			}
			for severity, marker := range map[semanticSeverity]string{
				severitySuccess: "OK", severityWarning: "WARN", severityError: "ERR",
			} {
				got := stripANSI(theme.severity(severity, "state"))
				if !strings.Contains(got, marker) || !strings.Contains(got, "state") {
					t.Fatalf("%s lacks marker %q: %q", severity, marker, got)
				}
			}
		})
	}
}

func TestClipPlainUsesDisplayCellsAndPreservesGraphemes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
		want  string
	}{
		{"cjk", "技能管理", 5, "技能…"},
		{"emoji-zwj", "👩‍💻tools", 4, "👩‍💻t…"},
		{"mixed", "A界B", 3, "A…"},
		{"one-cell", "界", 1, "…"},
		{"fits", "技能", 4, "技能"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clipPlain(tt.value, tt.width)
			if got != tt.want || lipgloss.Width(got) > tt.width {
				t.Fatalf("clipPlain(%q,%d)=%q width=%d want=%q", tt.value, tt.width, got, lipgloss.Width(got), tt.want)
			}
		})
	}
}

func TestCJKAndEmojiRowsStayWithinTerminalCells(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Snapshot.Config.Library.Skills[0].Name = "技能👩‍💻管理器"
	m.Width, m.Height = 24, 12
	for i, line := range strings.Split(m.ViewString(), "\n") {
		if width := lipgloss.Width(line); width > m.Width {
			t.Fatalf("line %d width=%d > %d: %q", i, width, m.Width, line)
		}
	}
}

func TestNoColorEnvironmentSelectsNoColorTheme(t *testing.T) {
	old, present := os.LookupEnv("NO_COLOR")
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("NO_COLOR", old)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	})
	if err := os.Setenv("NO_COLOR", "1"); err != nil {
		t.Fatal(err)
	}
	theme := defaultSemanticTheme()
	if theme.mode != themeNoColor {
		t.Fatalf("NO_COLOR selected %s", theme.mode)
	}
	if strings.Contains(theme.focused("Status"), "\x1b[") {
		t.Fatalf("NO_COLOR emitted ANSI: %q", theme.focused("Status"))
	}
}

func TestDefaultSemanticThemeIsTerminalAdaptive(t *testing.T) {
	old, present := os.LookupEnv("NO_COLOR")
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("NO_COLOR", old)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	})
	_ = os.Unsetenv("NO_COLOR")
	if got := defaultSemanticTheme().mode; got != themeAdaptive {
		t.Fatalf("default theme=%s, want terminal-adaptive", got)
	}
}
