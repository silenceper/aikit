package tui

type navigationItem struct {
	View   View
	Label  string
	Rect   Rect
	Active bool
}

func layoutNavigation(layout Layout, views []View, active View) []navigationItem {
	if layout.Narrow {
		return []navigationItem{{View: active, Label: viewLabel(active), Rect: layout.Breadcrumb, Active: true}}
	}
	labels := map[View]string{
		ViewOverview:   "Ovr",
		ViewLibrary:    "Lib",
		ViewWorkspaces: "Work",
		ViewPresets:    "Pre",
		ViewMigration:  "Mig",
		ViewStatus:     "Stat",
	}
	items := make([]navigationItem, 0, len(views))
	for i, view := range views {
		rect := Rect{X: layout.Navigation.X, Y: layout.Navigation.Y + i, Width: layout.Navigation.Width, Height: 1}
		if rect.Bottom() > layout.Navigation.Bottom() {
			break
		}
		label := viewLabel(view)
		if layout.Compact {
			label = labels[view]
		}
		items = append(items, navigationItem{View: view, Label: label, Rect: rect, Active: view == active})
	}
	return items
}
