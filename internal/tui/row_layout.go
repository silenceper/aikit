package tui

const (
	collectionRowHeight  = 2
	collectionHeaderRows = 1
	overviewHeaderRows   = 4
)

type visibleRowsLayout struct {
	Start int
	End   int
	Rects []Rect
}

func layoutVisibleRows(layout Layout, total, cursor, scroll, headerRows, reservedBottom int) visibleRowsLayout {
	capacity := max(0, (layout.Main.Height-headerRows-reservedBottom)/collectionRowHeight)
	start, end := VisibleRange(total, cursor, scroll, capacity)
	geometry := visibleRowsLayout{Start: start, End: end, Rects: make([]Rect, 0, end-start)}
	for i := start; i < end; i++ {
		y := layout.Main.Y + headerRows + collectionRowHeight*(i-start)
		geometry.Rects = append(geometry.Rects, Rect{X: layout.Main.X, Y: y, Width: layout.Main.Width, Height: min(collectionRowHeight, max(0, layout.Main.Bottom()-y))})
	}
	return geometry
}

func (m Model) visibleRowsLayout(layout Layout) visibleRowsLayout {
	if m.ActiveView == ViewOverview && m.Mode == ModeTable {
		return layoutVisibleRows(layout, len(m.rows()), m.Cursor, m.Scroll, overviewHeaderRows, 0)
	}
	return layoutVisibleRows(layout, len(m.rows()), m.Cursor, m.Scroll, collectionHeaderRows, 1)
}
