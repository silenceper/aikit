package tui

import "testing"

func TestModernShellResponsiveBreakpoints(t *testing.T) {
	for _, tt := range []struct {
		width       int
		wantWide    bool
		wantCompact bool
		wantNarrow  bool
	}{
		{120, true, false, false}, {96, true, false, false},
		{95, false, true, false}, {80, false, true, false}, {60, false, true, false},
		{59, false, false, true}, {38, false, false, true}, {24, false, false, true},
	} {
		for _, height := range []int{8, 12, 16, 30} {
			layout := ComputeLayout(tt.width, height)
			if layout.Wide != tt.wantWide || layout.Compact != tt.wantCompact || layout.Narrow != tt.wantNarrow {
				t.Fatalf("%dx%d mode wide=%v compact=%v narrow=%v", tt.width, height, layout.Wide, layout.Compact, layout.Narrow)
			}
			if layout.Main.Empty() || layout.Header.Empty() || layout.Footer.Empty() {
				t.Fatalf("%dx%d lost shell region: %+v", tt.width, height, layout)
			}
			if !layout.Narrow && (layout.Navigation.Empty() || layout.Navigation.Overlaps(layout.Main)) {
				t.Fatalf("%dx%d invalid rail/main: %+v", tt.width, height, layout)
			}
			if layout.Narrow && (!layout.Navigation.Empty() || layout.Breadcrumb.Empty()) {
				t.Fatalf("%dx%d narrow shell must use breadcrumb: %+v", tt.width, height, layout)
			}
		}
	}
}

func TestLayoutNavigationUsesResponsiveLabelsAndRects(t *testing.T) {
	for _, tt := range []struct {
		width int
		count int
		label string
	}{
		{120, len(topViews), "Workspaces"},
		{80, len(topViews), "Workspaces"},
		{59, 1, "Library"},
	} {
		layout := ComputeLayout(tt.width, 20)
		items := layoutNavigation(layout, topViews, ViewLibrary)
		if len(items) != tt.count {
			t.Fatalf("width %d navigation count=%d want=%d", tt.width, len(items), tt.count)
		}
		found := false
		for _, item := range items {
			if item.Label == tt.label {
				found = true
			}
			if item.Rect.Empty() || item.Rect.Right() > layout.Width || item.Rect.Bottom() > layout.Height {
				t.Fatalf("width %d invalid navigation item: %+v", tt.width, item)
			}
		}
		if !found {
			t.Fatalf("width %d missing label %q: %+v", tt.width, tt.label, items)
		}
	}
}

func TestLayoutResponsiveRectsStayInsideTerminal(t *testing.T) {
	for _, width := range []int{20, 38, 59, 60, 80, 120} {
		for _, height := range []int{8, 12, 16, 30} {
			layout := ComputeLayout(width, height)
			for name, rect := range map[string]Rect{
				"header": layout.Header, "tabs": layout.Tabs, "breadcrumb": layout.Breadcrumb,
				"list": layout.List, "detail": layout.Detail, "status": layout.Status, "footer": layout.Footer,
			} {
				if rect.Empty() {
					continue
				}
				if rect.X < 0 || rect.Y < 0 || rect.Right() > width || rect.Bottom() > height {
					t.Fatalf("%dx%d %s escaped terminal: %+v", width, height, name, rect)
				}
			}
			if width < 60 {
				if !layout.Narrow || !layout.Detail.Empty() || layout.Breadcrumb.Empty() {
					t.Fatalf("%dx%d should be single-pane with breadcrumb: %+v", width, height, layout)
				}
			} else if layout.Narrow || layout.Navigation.Empty() || layout.Main.Empty() || layout.Navigation.Overlaps(layout.Main) {
				t.Fatalf("%dx%d should have non-overlapping rail and main: %+v", width, height, layout)
			}
		}
	}
}

func TestLayoutVisibleRangeKeepsAbsoluteCursorVisible(t *testing.T) {
	tests := []struct {
		total, cursor, offset, capacity int
		wantStart, wantEnd              int
	}{
		{20, 0, 0, 5, 0, 5},
		{20, 7, 0, 5, 3, 8},
		{20, 7, 6, 5, 6, 11},
		{3, 2, 10, 5, 0, 3},
		{0, 0, 0, 5, 0, 0},
	}
	for _, tt := range tests {
		start, end := VisibleRange(tt.total, tt.cursor, tt.offset, tt.capacity)
		if start != tt.wantStart || end != tt.wantEnd {
			t.Fatalf("VisibleRange(%d,%d,%d,%d)=(%d,%d), want (%d,%d)", tt.total, tt.cursor, tt.offset, tt.capacity, start, end, tt.wantStart, tt.wantEnd)
		}
	}
}
