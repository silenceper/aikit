package tui

type overlayPanelLayout struct {
	Panel     Rect
	Title     Rect
	Body      Rect
	Actions   Rect
	ActionBar actionBarLayout
}

func layoutOverlayPanel(layout Layout, actions []string, focused bool, selected int) overlayPanelLayout {
	panel := layout.Overlay
	result := overlayPanelLayout{Panel: panel}
	if panel.Empty() {
		return result
	}
	result.Title = Rect{X: panel.X, Y: panel.Y, Width: panel.Width, Height: min(1, panel.Height)}
	actionRows := 0
	if len(actions) > 0 && panel.Height > 1 {
		actionRows = 1
		result.Actions = Rect{X: panel.X, Y: panel.Bottom() - 1, Width: panel.Width, Height: 1}
		result.ActionBar = layoutActionBar(actions, focused, selected, panel.Width)
	}
	result.Body = Rect{X: panel.X, Y: result.Title.Bottom(), Width: panel.Width, Height: max(0, panel.Height-result.Title.Height-actionRows)}
	return result
}

func (m Model) overlayPanelActions() []string {
	switch m.Mode {
	case ModeConfirm:
		return []string{"Confirm", "Cancel"}
	case ModeInput:
		return []string{"Apply", "Cancel"}
	case ModeConfiguration, ModeErrorDetail:
		return m.primaryActions()
	default:
		return nil
	}
}

func overlayBodyCapacity(layout Layout, bodyLength int) int {
	capacity := layoutOverlayPanel(layout, []string{"action"}, false, 0).Body.Height
	if bodyLength > capacity {
		capacity = max(1, capacity-1)
	}
	return capacity
}
