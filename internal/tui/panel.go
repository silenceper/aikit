package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

func renderPanel(panel PanelLayout, title string, focused bool, body []string, action string, glyphs FrameGlyphs) []string {
	if panel.Outer.Empty() {
		return nil
	}
	width, height := panel.Outer.Width, panel.Outer.Height
	lines := make([]string, height)
	if width < 2 || height < 2 {
		return lines
	}

	titlePrefix := " "
	if focused {
		titlePrefix = " > "
	}
	titleText := clipPlain(titlePrefix+title+" ", max(0, width-2))
	topFill := max(0, width-2-lipgloss.Width(titleText))
	lines[0] = glyphs.TopLeft + titleText + strings.Repeat(glyphs.Horizontal, topFill) + glyphs.TopRight
	lines[height-1] = glyphs.BottomLeft + strings.Repeat(glyphs.Horizontal, max(0, width-2)) + glyphs.BottomRight

	bodyStart := panel.Body.Y - panel.Outer.Y
	actionRow := panel.Actions.Y - panel.Outer.Y
	for row := 1; row < height-1; row++ {
		value := ""
		if !panel.Actions.Empty() && row == actionRow {
			value = action
		} else if index := row - bodyStart; index >= 0 && index < len(body) {
			value = body[index]
		}
		lines[row] = glyphs.Vertical + padCells(value, max(0, width-2)) + glyphs.Vertical
	}
	return lines
}

func padCells(value string, width int) string {
	value = clip(value, width)
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}

type positionedLines struct {
	X, Y  int
	Lines []string
}

func composePositioned(width, height int, groups ...positionedLines) []string {
	result := make([]string, max(0, height))
	for y := range result {
		type segment struct {
			x     int
			value string
		}
		segments := make([]segment, 0, len(groups))
		for _, group := range groups {
			index := y - group.Y
			if index >= 0 && index < len(group.Lines) {
				segments = append(segments, segment{x: group.X, value: group.Lines[index]})
			}
		}
		sort.SliceStable(segments, func(i, j int) bool { return segments[i].x < segments[j].x })
		var line strings.Builder
		cells := 0
		for _, current := range segments {
			if current.x > cells {
				line.WriteString(strings.Repeat(" ", current.x-cells))
				cells = current.x
			}
			if current.x < cells {
				continue
			}
			value := clip(current.value, max(0, width-cells))
			line.WriteString(value)
			cells += lipgloss.Width(value)
		}
		result[y] = clip(line.String(), width)
	}
	return result
}

func framedOverlayPanel(layout Layout, actions bool) PanelLayout {
	content := layout.CollectionPanel.Outer
	if !layout.DetailPanel.Outer.Empty() {
		content.Width = layout.DetailPanel.Outer.Right() - content.X
	}
	if content.Empty() {
		return PanelLayout{}
	}
	width := min(content.Width, max(20, content.Width-4))
	height := min(content.Height, max(4, content.Height*2/3))
	outer := Rect{
		X:     content.X + max(0, (content.Width-width)/2),
		Y:     content.Y + max(0, (content.Height-height)/2),
		Width: width, Height: height,
	}
	return layoutPanel(outer, actions)
}
