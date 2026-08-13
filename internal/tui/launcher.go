package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

// Launcher is the command layer's injection seam for the full-screen TUI.
// Service owns all durable mutations; Launcher only drives the event loop.
type Launcher struct {
	Service        app.Service
	Migration      app.MigrationService
	ProgramOptions []tea.ProgramOption
}

func (l Launcher) Launch(ctx context.Context, initialView View, initialAction Action) error {
	options := append([]tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}, l.ProgramOptions...)
	_, err := tea.NewProgram(NewModel(ctx, l.Service, l.Migration, initialView, initialAction), options...).Run()
	return err
}
