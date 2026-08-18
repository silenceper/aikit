package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

type themeMode uint8

const (
	themeAdaptive themeMode = iota
	themeDark
	themeLight
	themeReduced
	themeNoColor
)

func (m themeMode) String() string {
	switch m {
	case themeAdaptive:
		return "adaptive"
	case themeDark:
		return "dark"
	case themeLight:
		return "light"
	case themeReduced:
		return "reduced"
	case themeNoColor:
		return "no-color"
	default:
		return "unknown"
	}
}

type semanticSeverity string

const (
	severitySuccess semanticSeverity = "success"
	severityWarning semanticSeverity = "warning"
	severityError   semanticSeverity = "error"
)

type semanticTheme struct {
	mode             themeMode
	appTitle         lipgloss.Style
	navigation       lipgloss.Style
	selected         lipgloss.Style
	muted            lipgloss.Style
	reading          lipgloss.Style
	network          lipgloss.Style
	mutating         lipgloss.Style
	success          lipgloss.Style
	warning          lipgloss.Style
	error            lipgloss.Style
	badge            lipgloss.Style
	panelTitle       lipgloss.Style
	primaryAction    lipgloss.Style
	actionMnemonic   lipgloss.Style
	disabledAction   lipgloss.Style
	disabledMnemonic lipgloss.Style
}

func adaptive(lightTrue, darkTrue, ansi256, ansi string) lipgloss.TerminalColor {
	return lipgloss.CompleteAdaptiveColor{
		Light: lipgloss.CompleteColor{TrueColor: lightTrue, ANSI256: ansi256, ANSI: ansi},
		Dark:  lipgloss.CompleteColor{TrueColor: darkTrue, ANSI256: ansi256, ANSI: ansi},
	}
}

func newSemanticTheme(mode themeMode) semanticTheme {
	accent := adaptive("#5B4DD8", "#9B8CFF", "99", "5")
	muted := adaptive("#596273", "#8B93A7", "245", "8")
	green := adaptive("#147D45", "#55D68B", "35", "2")
	amber := adaptive("#9A6700", "#F0B429", "178", "3")
	red := adaptive("#B42318", "#FF6B6B", "167", "1")
	blue := adaptive("#1D4ED8", "#60A5FA", "33", "4")
	cyan := adaptive("#0E7490", "#22D3EE", "37", "6")
	purple := adaptive("#7E22CE", "#C084FC", "135", "5")
	if mode == themeDark {
		accent, muted = lipgloss.CompleteColor{TrueColor: "#9B8CFF", ANSI256: "99", ANSI: "5"}, lipgloss.CompleteColor{TrueColor: "#8B93A7", ANSI256: "245", ANSI: "8"}
		green, amber, red = lipgloss.CompleteColor{TrueColor: "#55D68B", ANSI256: "35", ANSI: "2"}, lipgloss.CompleteColor{TrueColor: "#F0B429", ANSI256: "178", ANSI: "3"}, lipgloss.CompleteColor{TrueColor: "#FF6B6B", ANSI256: "167", ANSI: "1"}
		blue, cyan, purple = lipgloss.CompleteColor{TrueColor: "#60A5FA", ANSI256: "33", ANSI: "4"}, lipgloss.CompleteColor{TrueColor: "#22D3EE", ANSI256: "37", ANSI: "6"}, lipgloss.CompleteColor{TrueColor: "#C084FC", ANSI256: "135", ANSI: "5"}
	} else if mode == themeLight {
		accent, muted = lipgloss.CompleteColor{TrueColor: "#5B4DD8", ANSI256: "62", ANSI: "5"}, lipgloss.CompleteColor{TrueColor: "#596273", ANSI256: "240", ANSI: "8"}
		green, amber, red = lipgloss.CompleteColor{TrueColor: "#147D45", ANSI256: "28", ANSI: "2"}, lipgloss.CompleteColor{TrueColor: "#9A6700", ANSI256: "136", ANSI: "3"}, lipgloss.CompleteColor{TrueColor: "#B42318", ANSI256: "124", ANSI: "1"}
		blue, cyan, purple = lipgloss.CompleteColor{TrueColor: "#1D4ED8", ANSI256: "26", ANSI: "4"}, lipgloss.CompleteColor{TrueColor: "#0E7490", ANSI256: "30", ANSI: "6"}, lipgloss.CompleteColor{TrueColor: "#7E22CE", ANSI256: "92", ANSI: "5"}
	} else if mode == themeReduced {
		accent, muted = lipgloss.Color("5"), lipgloss.Color("8")
		green, amber, red = lipgloss.Color("2"), lipgloss.Color("3"), lipgloss.Color("1")
		blue, cyan, purple = lipgloss.Color("4"), lipgloss.Color("6"), lipgloss.Color("5")
	}
	base := func(color lipgloss.TerminalColor) lipgloss.Style {
		style := lipgloss.NewStyle()
		if mode != themeNoColor {
			style = style.Foreground(color)
		}
		return style
	}
	return semanticTheme{
		mode:             mode,
		appTitle:         base(accent).Bold(true),
		navigation:       base(muted),
		selected:         base(accent).Bold(true),
		muted:            base(muted),
		reading:          base(blue),
		network:          base(cyan),
		mutating:         base(purple).Bold(true),
		success:          base(green),
		warning:          base(amber),
		error:            base(red).Bold(true),
		badge:            base(muted),
		panelTitle:       base(accent).Bold(true),
		primaryAction:    base(accent).Bold(true).Underline(true),
		actionMnemonic:   base(accent).Bold(true).Underline(true),
		disabledAction:   base(muted),
		disabledMnemonic: base(muted).Underline(true),
	}
}

func (t semanticTheme) activity(kind ActivityKind, label string) string {
	style := t.muted
	switch kind {
	case ActivityReading:
		style = t.reading
	case ActivityNetwork:
		style = t.network
	case ActivityMutating:
		style = t.mutating
	case ActivitySuccess:
		style = t.success
	case ActivityWarning:
		style = t.warning
	case ActivityError:
		style = t.error
	}
	return style.Render(label)
}

func defaultSemanticTheme() semanticTheme {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return newSemanticTheme(themeNoColor)
	}
	return newSemanticTheme(themeAdaptive)
}

func (t semanticTheme) focused(label string) string {
	return t.selected.Render("> " + label)
}

func (t semanticTheme) severity(level semanticSeverity, label string) string {
	marker, style := "INFO", t.badge
	switch level {
	case severitySuccess:
		marker, style = "OK", t.success
	case severityWarning:
		marker, style = "WARN", t.warning
	case severityError:
		marker, style = "ERR", t.error
	}
	return style.Render("[" + marker + "] " + label)
}

var (
	uiTheme     = defaultSemanticTheme()
	activeStyle = uiTheme.panelTitle
	mutedStyle  = uiTheme.muted
	errorStyle  = uiTheme.error
	selectStyle = uiTheme.selected
)

func clip(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	return clipPlain(stripANSI(value), width)
}

func clipPlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	limit := width - lipgloss.Width("…")
	var clipped strings.Builder
	used := 0
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		cluster := graphemes.Str()
		cells := lipgloss.Width(cluster)
		if used+cells > limit {
			break
		}
		clipped.WriteString(cluster)
		used += cells
	}
	return clipped.String() + "…"
}

func stripANSI(value string) string {
	var b strings.Builder
	escape := false
	for _, r := range value {
		if r == '\x1b' {
			escape = true
			continue
		}
		if escape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				escape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
