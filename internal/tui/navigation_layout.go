package tui

type navigationItem struct {
	View   View
	Label  string
	Rect   Rect
	Active bool
}

type navigationEntryItem struct {
	Entry navigationEntry
	Rect  Rect
}

func layoutNavigationEntries(layout Layout, m Model) []navigationEntryItem {
	if layout.NavigationPanel.Body.Empty() {
		return nil
	}
	entries := navigationEntries(m)
	result := make([]navigationEntryItem, 0, 7)
	y := layout.NavigationPanel.Body.Y
	lastSection := ""
	for _, entry := range entries {
		if entry.Section == "Actions" {
			continue
		}
		if lastSection != "" && entry.Section != lastSection {
			y++
		}
		if y >= layout.NavigationPanel.Body.Bottom() {
			break
		}
		result = append(result, navigationEntryItem{Entry: entry, Rect: Rect{X: layout.NavigationPanel.Body.X, Y: y, Width: layout.NavigationPanel.Body.Width, Height: 1}})
		y++
		lastSection = entry.Section
	}
	return result
}

func layoutNavigation(layout Layout, views []View, active View) []navigationItem {
	if layout.Narrow {
		return []navigationItem{{View: active, Label: viewLabel(active), Rect: layout.Breadcrumb, Active: true}}
	}
	items := make([]navigationItem, 0, len(views))
	for i, view := range views {
		rect := Rect{X: layout.Navigation.X, Y: layout.Navigation.Y + i, Width: layout.Navigation.Width, Height: 1}
		if rect.Bottom() > layout.Navigation.Bottom() {
			break
		}
		label := viewLabel(view)
		items = append(items, navigationItem{View: view, Label: label, Rect: rect, Active: view == active})
	}
	return items
}
