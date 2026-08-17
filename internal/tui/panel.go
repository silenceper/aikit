package tui

import "github.com/charmbracelet/lipgloss"

// FrameGlyphs defines one-cell characters used by a framed terminal panel.
// Unicode and ASCII profiles intentionally share identical cell geometry.
type FrameGlyphs struct {
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	Horizontal  string
	Vertical    string
}

// PanelLayout is the single source of frame, title, body, and action geometry.
type PanelLayout struct {
	Outer   Rect
	Title   Rect
	Body    Rect
	Actions Rect
}

func unicodeFrameGlyphs() FrameGlyphs {
	return FrameGlyphs{TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘", Horizontal: "─", Vertical: "│"}
}

func asciiFrameGlyphs() FrameGlyphs {
	return FrameGlyphs{TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+", Horizontal: "-", Vertical: "|"}
}

func frameCellWidth(glyphs FrameGlyphs) int {
	return lipgloss.Width(glyphs.TopLeft) + lipgloss.Width(glyphs.TopRight) +
		lipgloss.Width(glyphs.BottomLeft) + lipgloss.Width(glyphs.BottomRight) +
		lipgloss.Width(glyphs.Horizontal) + lipgloss.Width(glyphs.Vertical)
}

func layoutPanel(outer Rect, actions bool) PanelLayout {
	if outer.Width < 2 || outer.Height < 2 {
		return PanelLayout{}
	}
	result := PanelLayout{
		Outer: outer,
		Title: Rect{X: outer.X + min(2, outer.Width-1), Y: outer.Y, Width: max(0, outer.Width-4), Height: 1},
		Body:  Rect{X: outer.X + 1, Y: outer.Y + 1, Width: max(0, outer.Width-2), Height: max(0, outer.Height-2)},
	}
	if actions && result.Body.Height > 0 {
		result.Actions = Rect{X: result.Body.X, Y: result.Body.Bottom() - 1, Width: result.Body.Width, Height: 1}
		result.Body.Height--
	}
	return result
}
