package tui

// enterModal is the single entry point for action-bearing overlays. A modal
// never inherits an action selection or scroll position from its parent.
func (m *Model) enterModal(mode Mode) {
	m.Mode = mode
	m.Focus = FocusActions
	m.ActionIndex = 0
	m.OverlayScroll = 0
}

func (m *Model) enterConfirm(action Action) {
	if m.confirmReturnReady {
		m.confirmReturnReady = false
	} else if m.Mode != ModeConfirm {
		m.captureConfirmReturn()
	}
	m.confirm = action
	m.enterModal(ModeConfirm)
}

func (m *Model) prepareConfirmReturn() {
	m.captureConfirmReturn()
	m.confirmReturnReady = true
}

func (m *Model) captureConfirmReturn() {
	selected := make(map[string]bool, len(m.Selected))
	for key, value := range m.Selected {
		selected[key] = value
	}
	m.confirmReturn = confirmReturnState{
		Valid:         true,
		ActiveView:    m.ActiveView,
		Mode:          m.Mode,
		Focus:         m.Focus,
		ActionIndex:   m.ActionIndex,
		Cursor:        m.Cursor,
		Scroll:        m.Scroll,
		DetailScroll:  m.DetailScroll,
		OverlayScroll: m.OverlayScroll,
		Selected:      selected,
	}
}

func (m *Model) restoreConfirmReturn() bool {
	state := m.confirmReturn
	if !state.Valid {
		return false
	}
	m.ActiveView = state.ActiveView
	m.Mode = state.Mode
	m.Focus = state.Focus
	m.ActionIndex = state.ActionIndex
	m.Cursor = state.Cursor
	m.Scroll = state.Scroll
	m.DetailScroll = state.DetailScroll
	m.OverlayScroll = state.OverlayScroll
	m.Selected = state.Selected
	m.confirmReturn = confirmReturnState{}
	m.confirmReturnReady = false
	return true
}

func (m *Model) enterInput(input inputState) {
	m.Input = input
	m.enterModal(ModeInput)
}

func (m *Model) enterConfiguration() { m.enterModal(ModeConfiguration) }

func (m *Model) enterMore() { m.enterModal(ModeMore) }

func (m *Model) enterErrorDetail(value string) {
	parent := m.Mode
	m.enterModal(ModeErrorDetail)
	m.errorDetailParent = parent
	m.FullError = value
}

// currentActions is the shared keyboard/mouse dispatcher registry. Modal
// action bars take precedence over the collection actions behind them.
func (m Model) currentActions() []string {
	switch m.Mode {
	case ModeConfirm, ModeInput, ModeConfiguration, ModeErrorDetail:
		return m.overlayPanelActions()
	default:
		return m.primaryActions()
	}
}
