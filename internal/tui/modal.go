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
	m.confirm = action
	m.enterModal(ModeConfirm)
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
