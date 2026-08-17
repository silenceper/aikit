package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const actionBarSeparator = " "

const (
	actionBarPrevious = "‹"
	actionBarNext     = "›"
)

type actionBarLayout struct {
	Text          string
	Bar           Rect
	Buttons       []Rect
	ButtonIndexes []int
	Previous      Rect
	Next          Rect
	PreviousIndex int
	NextIndex     int
}

// layoutActionBar is the single source of horizontal action-bar geometry.
// Rendering consumes Text while mouse hit testing translates every returned
// button and paging control rectangle.
func layoutActionBar(actions []string, focused bool, selected, width int) actionBarLayout {
	width = max(0, width)
	result := actionBarLayout{PreviousIndex: -1, NextIndex: -1}
	if width == 0 || len(actions) == 0 {
		return result
	}
	result.Bar = Rect{Width: width, Height: 1}
	selected = min(max(0, selected), len(actions)-1)
	start, end, previous := actionBarPage(actions, selected, width)
	if start > 0 {
		result.PreviousIndex = previous
	}
	if end < len(actions) {
		result.NextIndex = end
	}

	var rendered strings.Builder
	cursor := 0
	appendSeparator := func() bool {
		remaining := width - cursor
		if remaining <= 0 {
			return false
		}
		separator := strings.Repeat(" ", min(lipgloss.Width(actionBarSeparator), remaining))
		rendered.WriteString(separator)
		cursor += lipgloss.Width(separator)
		return cursor < width
	}
	if start > 0 {
		control := clipPlain(actionBarPrevious, width)
		result.Previous = Rect{X: cursor, Width: lipgloss.Width(control), Height: 1}
		rendered.WriteString(control)
		cursor += result.Previous.Width
	}
	for index := start; index < end; index++ {
		if cursor > 0 && !appendSeparator() {
			break
		}
		remaining := width - cursor
		reserveNext := 0
		if index == end-1 && end < len(actions) {
			reserveNext = lipgloss.Width(actionBarSeparator) + lipgloss.Width(actionBarNext)
		}
		remaining -= reserveNext
		if remaining <= 0 {
			break
		}
		button := "[" + actions[index] + "]"
		if focused && selected == index {
			button = "{" + actions[index] + "}"
		}
		buttonWidth := lipgloss.Width(button)
		visible := min(buttonWidth, remaining)
		if visible <= 0 {
			break
		}
		visibleButton := button
		if visible < buttonWidth {
			visibleButton = clip(button, visible)
		}
		if focused && selected == index {
			visibleButton = selectStyle.Render(visibleButton)
		}
		rendered.WriteString(visibleButton)
		result.Buttons = append(result.Buttons, Rect{X: cursor, Width: visible, Height: 1})
		result.ButtonIndexes = append(result.ButtonIndexes, index)
		cursor += visible
		if visible < buttonWidth {
			break
		}
	}
	if end < len(actions) && cursor < width && appendSeparator() {
		control := clipPlain(actionBarNext, width-cursor)
		result.Next = Rect{X: cursor, Width: lipgloss.Width(control), Height: 1}
		rendered.WriteString(control)
	}
	result.Text = rendered.String()
	return result
}

// actionBarPage returns the stable page containing selected. Each page reserves
// visible cells for its mouse-operable previous/next controls.
func actionBarPage(actions []string, selected, width int) (start, end, previous int) {
	start, previous = 0, -1
	for {
		end = actionBarPageEnd(actions, start, width)
		if selected < end || end >= len(actions) {
			return start, end, previous
		}
		previous, start = start, end
	}
}

func actionBarPageEnd(actions []string, start, width int) int {
	used := 0
	if start > 0 {
		used = lipgloss.Width(actionBarPrevious)
	}
	end := start
	for index := start; index < len(actions); index++ {
		separator := 0
		if used > 0 {
			separator = lipgloss.Width(actionBarSeparator)
		}
		buttonWidth := lipgloss.Width("[" + actions[index] + "]")
		nextReserve := 0
		if index+1 < len(actions) {
			nextReserve = lipgloss.Width(actionBarSeparator) + lipgloss.Width(actionBarNext)
		}
		if used+separator+buttonWidth+nextReserve > width {
			if end == start && width-used-separator-nextReserve > 0 {
				end = index + 1
			}
			break
		}
		used += separator + buttonWidth
		end = index + 1
	}
	if end == start {
		// Even extremely narrow terminals make progress; rendering clips the
		// selected button while retaining any control cells that fit.
		end = min(len(actions), start+1)
	}
	return end
}
