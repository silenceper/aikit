package tui

type moreActionsLayout struct {
	Start   int
	End     int
	Indexes []int
	Rects   []Rect
}

func layoutMoreActions(layout Layout, actions []string, selected, scroll int) moreActionsLayout {
	capacity := max(0, layout.Overlay.Height-1)
	start, end := VisibleRange(len(actions), selected, scroll, capacity)
	result := moreActionsLayout{Start: start, End: end, Indexes: make([]int, 0, end-start), Rects: make([]Rect, 0, end-start)}
	for index := start; index < end; index++ {
		result.Indexes = append(result.Indexes, index)
		result.Rects = append(result.Rects, Rect{X: layout.Overlay.X, Y: layout.Overlay.Y + 1 + index - start, Width: layout.Overlay.Width, Height: 1})
	}
	return result
}
