package tui

// Rect is a terminal cell rectangle. Right and Bottom are exclusive.
type Rect struct {
	X, Y          int
	Width, Height int
}

func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }
func (r Rect) Right() int  { return r.X + max(0, r.Width) }
func (r Rect) Bottom() int { return r.Y + max(0, r.Height) }
func (r Rect) Contains(x, y int) bool {
	return !r.Empty() && x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom()
}
func (r Rect) Overlaps(other Rect) bool {
	return !r.Empty() && !other.Empty() && r.X < other.Right() && other.X < r.Right() && r.Y < other.Bottom() && other.Y < r.Bottom()
}

type Layout struct {
	Width, Height   int
	Wide            bool
	Compact         bool
	Narrow          bool
	LowHeight       bool
	TooShort        bool
	AppBar          PanelLayout
	NavigationPanel PanelLayout
	CollectionPanel PanelLayout
	DetailPanel     PanelLayout
	FooterPanel     PanelLayout
	Header          Rect
	Tabs            Rect
	Navigation      Rect
	Main            Rect
	Breadcrumb      Rect
	List            Rect
	Detail          Rect
	Status          Rect
	Footer          Rect
	Overlay         Rect
}

// ComputeLayout is the single source of terminal geometry for rendering and
// mouse hit testing. Width 55 is the first two-pane layout.
func ComputeLayout(width, height int) Layout {
	width = max(0, width)
	height = max(0, height)
	layout := Layout{
		Width: width, Height: height,
		Wide: width >= 96, Compact: width >= 60 && width < 96, Narrow: width < 60,
		LowHeight: height >= 8 && height < 12, TooShort: height > 0 && height < 8,
	}
	if width == 0 || height == 0 {
		return layout
	}
	layout.Header = Rect{Width: width, Height: min(1, height)}
	footerHeight := min(1, max(0, height-layout.Header.Height))
	statusHeight := 0
	if height >= 10 {
		statusHeight = 1
	}
	bodyY := layout.Header.Bottom()
	bodyHeight := max(0, height-bodyY-footerHeight-statusHeight)
	if statusHeight > 0 {
		layout.Status = Rect{Y: height - footerHeight - statusHeight, Width: width, Height: statusHeight}
	}
	if footerHeight > 0 {
		layout.Footer = Rect{Y: height - footerHeight, Width: width, Height: footerHeight}
	}
	if layout.Narrow {
		layout.Breadcrumb = Rect{Y: bodyY, Width: width, Height: min(1, bodyHeight)}
		layout.Main = Rect{Y: layout.Breadcrumb.Bottom(), Width: width, Height: max(0, bodyHeight-layout.Breadcrumb.Height)}
	} else {
		navigationWidth := 16
		if layout.Wide {
			navigationWidth = 20
		}
		layout.Navigation = Rect{Y: bodyY, Width: min(navigationWidth, width), Height: bodyHeight}
		layout.Tabs = layout.Navigation
		mainX := min(width, layout.Navigation.Right()+1)
		layout.Main = Rect{X: mainX, Y: bodyY, Width: max(0, width-mainX), Height: bodyHeight}
	}
	layout.List = layout.Main
	if layout.Wide && layout.Main.Width >= 72 {
		gap := 1
		available := layout.Main.Width - gap
		listWidth := min(55, max(38, available/2))
		layout.List = Rect{X: layout.Main.X, Y: layout.Main.Y, Width: listWidth, Height: layout.Main.Height}
		layout.Detail = Rect{X: layout.List.Right() + gap, Y: layout.Main.Y, Width: max(0, layout.Main.Right()-layout.List.Right()-gap), Height: layout.Main.Height}
	}
	overlayWidth := min(layout.Main.Width, max(20, layout.Main.Width-2))
	overlayHeight := min(layout.Main.Height, max(3, layout.Main.Height*2/3))
	layout.Overlay = Rect{
		X:      layout.Main.X + max(0, (layout.Main.Width-overlayWidth)/2),
		Y:      layout.Main.Y + max(0, (layout.Main.Height-overlayHeight)/2),
		Width:  overlayWidth,
		Height: overlayHeight,
	}
	layout.computeFramedPanels()
	return layout
}

func (layout *Layout) computeFramedPanels() {
	if layout.Width <= 0 || layout.Height <= 0 || layout.TooShort {
		return
	}
	if layout.LowHeight {
		layout.CollectionPanel = layoutPanel(Rect{X: 0, Y: 1, Width: layout.Width, Height: max(0, layout.Height-2)}, true)
		return
	}

	layout.AppBar = layoutPanel(Rect{X: 0, Y: 0, Width: layout.Width, Height: 3}, false)
	layout.FooterPanel = layoutPanel(Rect{X: 0, Y: layout.Height - 3, Width: layout.Width, Height: 3}, false)
	body := Rect{X: 0, Y: 3, Width: layout.Width, Height: max(0, layout.Height-6)}
	if body.Height < 2 {
		return
	}
	if layout.Narrow {
		layout.CollectionPanel = layoutPanel(body, true)
		return
	}

	navigationWidth := 18
	if layout.Wide {
		navigationWidth = 20
	}
	navigationWidth = min(navigationWidth, max(2, body.Width-3))
	layout.NavigationPanel = layoutPanel(Rect{X: body.X, Y: body.Y, Width: navigationWidth, Height: body.Height}, false)
	contentX := layout.NavigationPanel.Outer.Right() + 1
	contentWidth := max(0, body.Right()-contentX)
	if !layout.Wide {
		layout.CollectionPanel = layoutPanel(Rect{X: contentX, Y: body.Y, Width: contentWidth, Height: body.Height}, true)
		return
	}

	collectionWidth := max(32, contentWidth*48/100)
	collectionWidth = min(collectionWidth, max(2, contentWidth-3))
	layout.CollectionPanel = layoutPanel(Rect{X: contentX, Y: body.Y, Width: collectionWidth, Height: body.Height}, true)
	detailX := layout.CollectionPanel.Outer.Right() + 1
	layout.DetailPanel = layoutPanel(Rect{X: detailX, Y: body.Y, Width: max(0, body.Right()-detailX), Height: body.Height}, true)
}

// VisibleRange returns an end-exclusive window and preserves a valid explicit
// offset when possible. Cursor is an absolute row index.
func VisibleRange(total, cursor, offset, capacity int) (start, end int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	capacity = min(capacity, total)
	cursor = min(max(0, cursor), total-1)
	maxStart := total - capacity
	start = min(max(0, offset), maxStart)
	if cursor < start {
		start = cursor
	} else if cursor >= start+capacity {
		start = cursor - capacity + 1
	}
	start = min(max(0, start), maxStart)
	return start, min(total, start+capacity)
}
