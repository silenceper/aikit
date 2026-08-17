package tui

type overlayPanelLayout struct {
	Panel     Rect
	Title     Rect
	Body      Rect
	Actions   Rect
	ActionBar actionBarLayout
}

func layoutOverlayPanel(layout Layout, actions []string, focused bool, selected int) overlayPanelLayout {
	framed := framedOverlayPanel(layout, len(actions) > 0)
	result := overlayPanelLayout{Panel: framed.Outer, Title: framed.Title, Body: framed.Body, Actions: framed.Actions}
	if framed.Outer.Empty() {
		return result
	}
	if len(actions) > 0 && !result.Actions.Empty() {
		result.ActionBar = layoutActionBar(actions, focused, selected, result.Actions.Width)
	}
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
