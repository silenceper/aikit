package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
)

type snapshotMsg struct {
	snapshot app.Snapshot
	err      error
}

type scanMsg struct {
	result app.ScanResult
	err    error
}

type operationMsg struct {
	name   string
	result app.Result
	scan   app.ScanResult
	err    error
}

func snapshotCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return snapshotMsg{err: errUnavailable("app service")}
		}
		snapshot, err := service.Snapshot(ctx, app.StatusRequest{})
		return snapshotMsg{snapshot: snapshot, err: err}
	}
}

func scanCmd(ctx context.Context, migration app.MigrationService, request app.ScanRequest) tea.Cmd {
	return func() tea.Msg {
		if migration == nil {
			return scanMsg{err: errUnavailable("migration service")}
		}
		result, err := migration.Scan(ctx, request)
		return scanMsg{result: result, err: err}
	}
}

func updateCmd(ctx context.Context, service app.Service, request app.UpdateRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "update", err: errUnavailable("app service")}
		}
		result, err := service.Update(ctx, request)
		return operationMsg{name: "update", result: result, err: err}
	}
}

func bindingCmd(ctx context.Context, service app.Service, request app.BindingRequest, enable bool) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "binding", err: errUnavailable("app service")}
		}
		var result app.Result
		var err error
		if enable {
			result, err = service.Enable(ctx, request)
		} else {
			result, err = service.Disable(ctx, request)
		}
		return operationMsg{name: "binding", result: result, err: err}
	}
}

func removeSkillCmd(ctx context.Context, service app.Service, request app.RemoveRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "remove skill", err: errUnavailable("app service")}
		}
		result, err := service.Remove(ctx, request)
		return operationMsg{name: "remove skill", result: result, err: err}
	}
}

func removeProjectCmd(ctx context.Context, service app.Service, request app.ProjectRemoveRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "remove project", err: errUnavailable("app service")}
		}
		result, err := service.RemoveProject(ctx, request)
		return operationMsg{name: "remove project", result: result, err: err}
	}
}

func presetCmd(ctx context.Context, service app.Service, request app.PresetRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "preset", err: errUnavailable("app service")}
		}
		result, err := service.PutPreset(ctx, request)
		return operationMsg{name: "preset", result: result, err: err}
	}
}

func syncCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "sync", err: errUnavailable("app service")}
		}
		result, err := service.Sync(ctx, app.SyncRequest{})
		return operationMsg{name: "sync", result: result, err: err}
	}
}

func adoptCmd(ctx context.Context, migration app.MigrationService, request app.ScanRequest) tea.Cmd {
	return func() tea.Msg {
		if migration == nil {
			return operationMsg{name: "adopt", err: errUnavailable("migration service")}
		}
		result, err := migration.Scan(ctx, request)
		return operationMsg{name: "adopt", scan: result, err: err}
	}
}

type unavailableError string

func (e unavailableError) Error() string { return string(e) + " is unavailable" }
func errUnavailable(name string) error   { return unavailableError(name) }
