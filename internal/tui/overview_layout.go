package tui

type overviewLayout struct {
	Outer      Rect
	Quick      PanelLayout
	Updates    PanelLayout
	Local      PanelLayout
	Health     PanelLayout
	SectionBar Rect
	Previous   Rect
	Next       Rect
	Active     overviewSectionID
	Rows       map[overviewSectionID]visibleRowsLayout
}

func (layout overviewLayout) ActivePanel() PanelLayout {
	switch layout.Active {
	case overviewLocal:
		return layout.Local
	case overviewHealth:
		return layout.Health
	default:
		return layout.Updates
	}
}

func (layout overviewLayout) panel(section overviewSectionID) PanelLayout {
	switch section {
	case overviewQuick:
		return layout.Quick
	case overviewLocal:
		return layout.Local
	case overviewHealth:
		return layout.Health
	default:
		return layout.Updates
	}
}

func (m Model) overviewLayout(shell Layout) overviewLayout {
	outer := shell.Main
	if outer.Empty() {
		outer = shell.CollectionPanel.Outer
	}
	active := m.OverviewSection
	if active != overviewUpdates && active != overviewLocal && active != overviewHealth {
		active = overviewUpdates
	}
	result := overviewLayout{Outer: outer, Active: active, Rows: make(map[overviewSectionID]visibleRowsLayout)}
	if outer.Width < 2 || outer.Height < 3 {
		return result
	}
	quickHeight := min(outer.Height, 3+len(m.overviewMetricLines(max(0, outer.Width-2))))
	result.Quick = layoutPanel(Rect{X: outer.X, Y: outer.Y, Width: outer.Width, Height: quickHeight}, true)
	nextY := result.Quick.Outer.Bottom()

	if shell.Narrow {
		if nextY < outer.Bottom() {
			result.SectionBar = Rect{X: outer.X, Y: nextY, Width: outer.Width, Height: 1}
			labelWidth := min(outer.Width, len([]rune(overviewSectionBarLabel(active))))
			if labelWidth > 0 {
				result.Previous = Rect{X: outer.X, Y: nextY, Width: 1, Height: 1}
				result.Next = Rect{X: outer.X + labelWidth - 1, Y: nextY, Width: 1, Height: 1}
			}
			nextY++
		}
		panel := layoutPanel(Rect{X: outer.X, Y: nextY, Width: outer.Width, Height: max(0, outer.Bottom()-nextY)}, true)
		switch active {
		case overviewLocal:
			result.Local = panel
		case overviewHealth:
			result.Health = panel
		default:
			result.Updates = panel
		}
		m.layoutOverviewRows(&result)
		return result
	}

	if shell.Wide && nextY < outer.Bottom() {
		nextY++
	}
	available := max(0, outer.Bottom()-nextY)
	if shell.Wide {
		upperHeight := max(2, (available-1)/2)
		upperHeight = min(upperHeight, available)
		gap := 1
		leftWidth := max(2, (outer.Width-gap)/2)
		leftWidth = min(leftWidth, outer.Width)
		result.Updates = overviewTaskPanel(Rect{X: outer.X, Y: nextY, Width: leftWidth, Height: upperHeight})
		result.Local = overviewTaskPanel(Rect{X: outer.X + leftWidth + gap, Y: nextY, Width: max(0, outer.Width-leftWidth-gap), Height: upperHeight})
		healthY := nextY + upperHeight
		if healthY < outer.Bottom() {
			healthY++
		}
		result.Health = overviewTaskPanel(Rect{X: outer.X, Y: healthY, Width: outer.Width, Height: max(0, outer.Bottom()-healthY)})
	} else {
		heights := splitOverviewHeights(available, 3)
		sections := []*PanelLayout{&result.Updates, &result.Local, &result.Health}
		for index, destination := range sections {
			*destination = overviewTaskPanel(Rect{X: outer.X, Y: nextY, Width: outer.Width, Height: heights[index]})
			nextY += heights[index]
		}
	}
	m.layoutOverviewRows(&result)
	return result
}

func splitOverviewHeights(total, count int) []int {
	result := make([]int, count)
	remaining := max(0, total)
	for index := range result {
		parts := count - index
		result[index] = remaining / parts
		remaining -= result[index]
	}
	return result
}

func overviewTaskPanel(rect Rect) PanelLayout {
	return layoutPanel(rect, rect.Height >= 4)
}

func (m Model) layoutOverviewRows(layout *overviewLayout) {
	dashboard := m.overviewDashboard()
	for _, section := range []overviewSectionID{overviewUpdates, overviewLocal, overviewHealth} {
		panel := layout.panel(section)
		if panel.Body.Empty() {
			continue
		}
		cursor, scroll := 0, 0
		if section == layout.Active {
			cursor, scroll = m.Cursor, m.Scroll
		}
		layout.Rows[section] = layoutVisibleRows(panel.Body, len(dashboard.tasks(section)), cursor, scroll, 0, 0, 1)
	}
}
