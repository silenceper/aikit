package tui

type moreActionsLayout struct {
	Start   int
	End     int
	Indexes []int
	Rects   []Rect
}

func layoutMoreActions(layout Layout, actions []string, selected, scroll int) moreActionsLayout {
	panel := framedOverlayPanel(layout, false)
	capacity := max(0, panel.Body.Height)
	start, end := VisibleRange(len(actions), selected, scroll, capacity)
	result := moreActionsLayout{Start: start, End: end, Indexes: make([]int, 0, end-start), Rects: make([]Rect, 0, end-start)}
	for index := start; index < end; index++ {
		result.Indexes = append(result.Indexes, index)
		result.Rects = append(result.Rects, Rect{X: panel.Body.X, Y: panel.Body.Y + index - start, Width: panel.Body.Width, Height: 1})
	}
	return result
}
