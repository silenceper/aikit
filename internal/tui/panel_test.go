package tui

import "testing"

func TestThreePaneResponsiveGeometry(t *testing.T) {
	tests := []struct {
		name                   string
		width, height          int
		wide, compact, narrow  bool
		lowHeight, tooShort    bool
		navigation, detail     bool
		fullAppAndFooterFrames bool
	}{
		{name: "wide", width: 120, height: 30, wide: true, navigation: true, detail: true, fullAppAndFooterFrames: true},
		{name: "wide boundary", width: 96, height: 12, wide: true, navigation: true, detail: true, fullAppAndFooterFrames: true},
		{name: "compact", width: 95, height: 16, compact: true, navigation: true, fullAppAndFooterFrames: true},
		{name: "compact boundary", width: 60, height: 12, compact: true, navigation: true, fullAppAndFooterFrames: true},
		{name: "narrow", width: 59, height: 16, narrow: true, fullAppAndFooterFrames: true},
		{name: "phone width", width: 24, height: 12, narrow: true, fullAppAndFooterFrames: true},
		{name: "low height", width: 120, height: 11, wide: true, lowHeight: true},
		{name: "minimum height", width: 38, height: 8, narrow: true, lowHeight: true},
		{name: "too short", width: 80, height: 7, compact: true, tooShort: true},
		{name: "very short", width: 24, height: 5, narrow: true, tooShort: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := ComputeLayout(tt.width, tt.height)
			if layout.Wide != tt.wide || layout.Compact != tt.compact || layout.Narrow != tt.narrow || layout.LowHeight != tt.lowHeight || layout.TooShort != tt.tooShort {
				t.Fatalf("flags=%+v", layout)
			}
			if tt.tooShort {
				for name, panel := range map[string]PanelLayout{"app": layout.AppBar, "nav": layout.NavigationPanel, "collection": layout.CollectionPanel, "detail": layout.DetailPanel, "footer": layout.FooterPanel} {
					if !panel.Outer.Empty() {
						t.Fatalf("too-short %s panel=%+v", name, panel)
					}
				}
				return
			}
			if layout.CollectionPanel.Outer.Empty() {
				t.Fatal("collection panel is empty")
			}
			if got := !layout.NavigationPanel.Outer.Empty(); got != tt.navigation {
				t.Fatalf("navigation present=%v want=%v", got, tt.navigation)
			}
			if got := !layout.DetailPanel.Outer.Empty(); got != tt.detail {
				t.Fatalf("detail present=%v want=%v", got, tt.detail)
			}
			if tt.fullAppAndFooterFrames {
				if layout.AppBar.Outer.Height != 3 || layout.FooterPanel.Outer.Height != 3 {
					t.Fatalf("app/footer=%+v/%+v", layout.AppBar.Outer, layout.FooterPanel.Outer)
				}
			} else if !tt.lowHeight || layout.Header.Height != 1 || layout.Footer.Height != 1 {
				t.Fatalf("low height app/footer=%+v/%+v", layout.Header, layout.Footer)
			}
			assertPanelInside(t, layout.CollectionPanel, tt.width, tt.height)
			assertPanelInside(t, layout.NavigationPanel, tt.width, tt.height)
			assertPanelInside(t, layout.DetailPanel, tt.width, tt.height)
		})
	}
}

func TestPanelGeometryAndGlyphProfilesShareCellBounds(t *testing.T) {
	outer := Rect{X: 3, Y: 2, Width: 32, Height: 10}
	withActions := layoutPanel(outer, true)
	withoutActions := layoutPanel(outer, false)
	if withActions.Outer != outer || withActions.Title.Y != outer.Y || withActions.Actions.Y != outer.Bottom()-2 {
		t.Fatalf("panel geometry=%+v", withActions)
	}
	if withActions.Body.Bottom() > withActions.Actions.Y || withoutActions.Body.Bottom() != outer.Bottom()-1 {
		t.Fatalf("body geometry with=%+v without=%+v", withActions.Body, withoutActions.Body)
	}
	if frameCellWidth(unicodeFrameGlyphs()) != frameCellWidth(asciiFrameGlyphs()) {
		t.Fatal("unicode and ASCII frames do not share cell geometry")
	}
}

func TestTooShortLayoutHasNoInteractiveHitRegions(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Width, m.Height = 80, 7
	regions := m.hitRegions()
	if !regions.Layout.TooShort {
		t.Fatal("layout is not marked too short")
	}
	if len(regions.Tabs)+len(regions.Rows)+len(regions.Checkboxes)+len(regions.Actions) != 0 || !regions.ActionBar.Empty() || !regions.Confirm.Empty() || !regions.Cancel.Empty() || !regions.Back.Empty() {
		t.Fatalf("too-short interactive regions=%+v", regions)
	}
}

func TestTooShortLayoutRejectsWorkflowKeys(t *testing.T) {
	for _, input := range []string{"enter", "space", "a", "r", "ctrl+k", "u", "n", "d", "x", "S", "A", "s"} {
		t.Run(input, func(t *testing.T) {
			service := &fakeService{}
			m := NewModel(nil, service, &fakeMigration{}, ViewLibrary, ActionNone)
			m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 7
			beforeMode, beforeView, beforeScope := m.Mode, m.ActiveView, m.Scope
			next, cmd := m.Update(key(input))
			got := next.(Model)
			if cmd != nil || got.Mode != beforeMode || got.ActiveView != beforeView || got.Scope != beforeScope || got.Busy || got.MutationBusy || len(got.Selected) != 0 {
				t.Fatalf("key %q escaped too-short guard: mode=%s view=%s scope=%+v busy=%v mutation=%v selected=%v cmd=%v", input, got.Mode, got.ActiveView, got.Scope, got.Busy, got.MutationBusy, got.Selected, cmd != nil)
			}
		})
	}
}

func assertPanelInside(t *testing.T, panel PanelLayout, width, height int) {
	t.Helper()
	if panel.Outer.Empty() {
		return
	}
	if panel.Outer.X < 0 || panel.Outer.Y < 0 || panel.Outer.Right() > width || panel.Outer.Bottom() > height {
		t.Fatalf("panel escapes %dx%d: %+v", width, height, panel)
	}
	for name, rect := range map[string]Rect{"title": panel.Title, "body": panel.Body, "actions": panel.Actions} {
		if rect.Empty() {
			continue
		}
		if rect.X < panel.Outer.X || rect.Y < panel.Outer.Y || rect.Right() > panel.Outer.Right() || rect.Bottom() > panel.Outer.Bottom() {
			t.Fatalf("%s escapes panel: panel=%+v rect=%+v", name, panel.Outer, rect)
		}
	}
}
