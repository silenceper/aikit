package tui

const (
	collectionRowHeight  = 1
	collectionHeaderRows = 1
	overviewHeaderRows   = 4
)

type visibleRowsLayout struct {
	Start int
	End   int
	Rects []Rect
}

func layoutVisibleRows(area Rect, total, cursor, scroll, headerRows, reservedBottom, rowHeight int) visibleRowsLayout {
	rowHeight = max(1, rowHeight)
	capacity := max(0, (area.Height-headerRows-reservedBottom)/rowHeight)
	start, end := VisibleRange(total, cursor, scroll, capacity)
	geometry := visibleRowsLayout{Start: start, End: end, Rects: make([]Rect, 0, end-start)}
	for i := start; i < end; i++ {
		y := area.Y + headerRows + rowHeight*(i-start)
		geometry.Rects = append(geometry.Rects, Rect{X: area.X, Y: y, Width: area.Width, Height: min(rowHeight, max(0, area.Bottom()-y))})
	}
	return geometry
}

func (m Model) visibleRowsLayout(layout Layout) visibleRowsLayout {
	if !layout.CollectionPanel.Outer.Empty() {
		area := layout.CollectionPanel.Body
		if m.ActiveView == ViewOverview && m.Mode == ModeTable {
			rowHeight := collectionRowHeight
			if area.Height <= 5 {
				rowHeight = 1
			}
			return layoutVisibleRows(area, len(m.rows()), m.Cursor, m.Scroll, m.overviewHeaderHeight(area.Width, area.Height), 0, rowHeight)
		}
		return layoutVisibleRows(area, len(m.rows()), m.Cursor, m.Scroll, 0, 0, collectionRowHeight)
	}
	if m.ActiveView == ViewOverview && m.Mode == ModeTable {
		rowHeight := collectionRowHeight
		if layout.Main.Height <= 5 {
			rowHeight = 1
		}
		return layoutVisibleRows(layout.Main, len(m.rows()), m.Cursor, m.Scroll, m.overviewHeaderHeight(layout.Main.Width, layout.Main.Height), 0, rowHeight)
	}
	if !layout.Detail.Empty() {
		return layoutVisibleRows(layout.List, len(m.rows()), m.Cursor, m.Scroll, collectionHeaderRows, 1, 1)
	}
	return layoutVisibleRows(layout.List, len(m.rows()), m.Cursor, m.Scroll, collectionHeaderRows, 1, collectionRowHeight)
}

func (m Model) overviewHeaderHeight(width, height int) int {
	metrics := m.overviewMetricLines(width)
	if height <= 5 {
		return 1 + len(metrics)
	}
	header := 2 + len(metrics)
	if height >= len(metrics)+4 {
		header++
	}
	return header
}
