package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/pkg/config"
)

type snapshotMsg struct {
	snapshot app.Snapshot
	err      error
}

type scanMsg struct {
	name   string
	result app.ScanResult
	err    error
}

type skillDetailMsg struct {
	skillID string
	detail  app.SkillDetail
	err     error
}

type mutationPreviewMsg struct {
	name    string
	preview app.MutationPreview
	err     error
}

type batchRemovePreviewMsg struct {
	preview app.MutationPreview
	err     error
}

type syncPreviewMsg struct {
	result app.Result
	err    error
}

type addPreviewMsg struct {
	preview app.AddPreview
	err     error
}

type compareMsg struct {
	result app.CompareResult
	err    error
}

type updateCheckMsg struct {
	result app.Result
	err    error
}

type inventoryMsg struct {
	event app.InventoryEvent
	ok    bool
}

type configurationMsg struct {
	detail app.ConfigurationDetail
	err    error
}

type configurationValidationMsg struct {
	result app.ConfigurationValidation
	err    error
}

type configurationReloadMsg struct {
	detail app.ConfigurationDetail
	err    error
}

type operationMsg struct {
	name   string
	result app.Result
	scan   app.ScanResult
	err    error
}

type batchOperationMsg struct {
	name   string
	result app.BatchResult
	err    error
}

type projectPreviewMsg struct {
	preview app.ProjectEditPreview
	err     error
}

type projectOperationMsg struct {
	name   string
	result app.Result
	err    error
}

type recoveryPreviewMsg struct {
	preview app.RecoveryPreview
	err     error
}

type recoveryOperationMsg struct {
	result app.RecoveryResult
	err    error
}

func snapshotCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return snapshotMsg{err: errUnavailable("app service")}
		}
		snapshot, err := service.Snapshot(ctx, app.StatusRequest{Offline: true})
		return snapshotMsg{snapshot: snapshot, err: err}
	}
}

func refreshSnapshotCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return snapshotMsg{err: errUnavailable("app service")}
		}
		snapshot, err := service.Snapshot(ctx, app.StatusRequest{ForceRefresh: true})
		return snapshotMsg{snapshot: snapshot, err: err}
	}
}

func recoveryPreviewCmd(ctx context.Context, service app.Service, request app.RecoveryRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return recoveryPreviewMsg{err: errUnavailable("app service")}
		}
		preview, err := service.PreviewRecovery(ctx, request)
		return recoveryPreviewMsg{preview: preview, err: err}
	}
}

func resumeRecoveryCmd(ctx context.Context, service app.Service, request app.RecoveryRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return recoveryOperationMsg{err: errUnavailable("app service")}
		}
		result, err := service.ResumeRecovery(ctx, request)
		return recoveryOperationMsg{result: result, err: err}
	}
}

func waitInventoryCmd(events <-chan app.InventoryEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		return inventoryMsg{event: event, ok: ok}
	}
}

func configurationCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return configurationMsg{err: errUnavailable("app service")}
		}
		detail, err := service.Configuration(ctx)
		return configurationMsg{detail: detail, err: err}
	}
}

func validateConfigurationCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return configurationValidationMsg{err: errUnavailable("app service")}
		}
		result, err := service.ValidateConfiguration(ctx)
		return configurationValidationMsg{result: result, err: err}
	}
}

func reloadConfigurationCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return configurationReloadMsg{err: errUnavailable("app service")}
		}
		detail, err := service.Configuration(ctx)
		return configurationReloadMsg{detail: detail, err: err}
	}
}

func offlineSnapshotCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return snapshotMsg{err: errUnavailable("app service")}
		}
		snapshot, err := service.Snapshot(ctx, app.StatusRequest{Offline: true})
		return snapshotMsg{snapshot: snapshot, err: err}
	}
}

func scanCmd(ctx context.Context, migration app.MigrationService, request app.ScanRequest) tea.Cmd {
	return func() tea.Msg {
		if migration == nil {
			return scanMsg{name: "scan", err: errUnavailable("migration service")}
		}
		result, err := migration.Scan(ctx, request)
		return scanMsg{name: "scan", result: result, err: err}
	}
}

func migrationPreviewCmd(ctx context.Context, migration app.MigrationService, request app.ScanRequest) tea.Cmd {
	return func() tea.Msg {
		if migration == nil {
			return scanMsg{name: "migration preview", err: errUnavailable("migration service")}
		}
		result, err := migration.Scan(ctx, request)
		return scanMsg{name: "migration preview", result: result, err: err}
	}
}

func skillDetailCmd(ctx context.Context, service app.Service, skillID string) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return skillDetailMsg{skillID: skillID, err: errUnavailable("app service")}
		}
		detail, err := service.SkillDetail(ctx, skillID)
		return skillDetailMsg{skillID: skillID, detail: detail, err: err}
	}
}

func bindingPreviewCmd(ctx context.Context, service app.Service, request app.BindingPreviewRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return mutationPreviewMsg{name: "binding", err: errUnavailable("app service")}
		}
		preview, err := service.PreviewBinding(ctx, request)
		return mutationPreviewMsg{name: "binding", preview: preview, err: err}
	}
}

func batchBindingPreviewCmd(ctx context.Context, service app.Service, request app.BatchRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return mutationPreviewMsg{name: "batch binding", err: errUnavailable("app service")}
		}
		combined := app.MutationPreview{
			Title: "Library batch " + string(request.Operation), Summary: fmt.Sprintf("%s %d exact binding(s)", title(string(request.Operation)), len(request.Bindings)), RequiresConfirmation: true,
		}
		for _, binding := range request.Bindings {
			preview, err := service.PreviewBinding(ctx, app.BindingPreviewRequest{Binding: binding, Enable: request.Operation == app.BatchEnable})
			if err != nil {
				return mutationPreviewMsg{name: "batch binding", err: err}
			}
			combined.AffectedScopes = append(combined.AffectedScopes, preview.AffectedScopes...)
			if len(preview.AffectedScopes) == 0 {
				combined.AffectedScopes = append(combined.AffectedScopes, config.Scope{Project: binding.Project, Agent: binding.Agent})
			}
			combined.Plan.Actions = append(combined.Plan.Actions, preview.Plan.Actions...)
			combined.Plan.Issues = append(combined.Plan.Issues, preview.Plan.Issues...)
			combined.Plan.Warnings = append(combined.Plan.Warnings, preview.Plan.Warnings...)
			combined.Warnings = append(combined.Warnings, preview.Warnings...)
			combined.Conflicts = append(combined.Conflicts, preview.Conflicts...)
		}
		return mutationPreviewMsg{name: "batch binding", preview: combined}
	}
}

func removePreviewCmd(ctx context.Context, service app.Service, request app.RemoveRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return mutationPreviewMsg{name: "remove", err: errUnavailable("app service")}
		}
		preview, err := service.PreviewRemove(ctx, request)
		return mutationPreviewMsg{name: "remove", preview: preview, err: err}
	}
}

func batchRemovePreviewCmd(ctx context.Context, service app.Service, skillIDs []string) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return batchRemovePreviewMsg{err: errUnavailable("app service")}
		}
		combined := app.MutationPreview{
			Title: "Remove selected skills", Summary: fmt.Sprintf("Remove %d selected library skill(s)", len(skillIDs)), RequiresConfirmation: true,
		}
		for _, skillID := range skillIDs {
			preview, err := service.PreviewRemove(ctx, app.RemoveRequest{SkillID: skillID})
			if err != nil {
				return batchRemovePreviewMsg{err: err}
			}
			combined.References = append(combined.References, preview.References...)
			combined.AffectedScopes = append(combined.AffectedScopes, preview.AffectedScopes...)
			combined.Plan.Actions = append(combined.Plan.Actions, preview.Plan.Actions...)
			combined.Plan.Issues = append(combined.Plan.Issues, preview.Plan.Issues...)
			combined.Plan.Warnings = append(combined.Plan.Warnings, preview.Plan.Warnings...)
			combined.Warnings = append(combined.Warnings, preview.Warnings...)
			combined.Conflicts = append(combined.Conflicts, preview.Conflicts...)
			combined.RequiresForce = combined.RequiresForce || preview.RequiresForce
		}
		return batchRemovePreviewMsg{preview: combined}
	}
}

func syncPreviewCmd(ctx context.Context, service app.Service, request app.SyncRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return syncPreviewMsg{err: errUnavailable("app service")}
		}
		result, err := service.Sync(ctx, request)
		return syncPreviewMsg{result: result, err: err}
	}
}

func addPreviewCmd(ctx context.Context, service app.Service, request app.AddPreviewRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return addPreviewMsg{err: errUnavailable("app service")}
		}
		preview, err := service.PreviewAdd(ctx, request)
		return addPreviewMsg{preview: preview, err: err}
	}
}

func addCmd(ctx context.Context, service app.Service, request app.AddRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "add", err: errUnavailable("app service")}
		}
		result, err := service.Add(ctx, request)
		return operationMsg{name: "add", result: result, err: err}
	}
}

func presetMutationPreviewCmd(ctx context.Context, service app.Service, request app.PresetMutationRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return mutationPreviewMsg{name: "preset", err: errUnavailable("app service")}
		}
		preview, err := service.PreviewPresetMutation(ctx, request)
		return mutationPreviewMsg{name: "preset", preview: preview, err: err}
	}
}

func mutatePresetCmd(ctx context.Context, service app.Service, request app.PresetMutationRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "preset", err: errUnavailable("app service")}
		}
		result, err := service.MutatePreset(ctx, request)
		return operationMsg{name: "preset", result: result, err: err}
	}
}

func compareCmd(ctx context.Context, service app.Service, request app.CompareRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return compareMsg{err: errUnavailable("app service")}
		}
		result, err := service.Compare(ctx, request)
		return compareMsg{result: result, err: err}
	}
}

func batchCmd(ctx context.Context, service app.Service, request app.BatchRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return batchOperationMsg{name: "batch " + string(request.Operation), err: errUnavailable("app service")}
		}
		result, err := service.Batch(ctx, request)
		return batchOperationMsg{name: "batch " + string(request.Operation), result: result, err: err}
	}
}

func updateCheckCmd(ctx context.Context, service app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return updateCheckMsg{err: errUnavailable("app service")}
		}
		result, err := service.Update(ctx, app.UpdateRequest{CheckOnly: true, Refresh: true})
		return updateCheckMsg{result: result, err: err}
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
			return projectOperationMsg{name: "remove project", err: errUnavailable("app service")}
		}
		result, err := service.RemoveProject(ctx, request)
		return projectOperationMsg{name: "remove project", result: result, err: err}
	}
}

func projectPreviewCmd(ctx context.Context, service app.Service, request app.ProjectEditRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return projectPreviewMsg{err: errUnavailable("app service")}
		}
		preview, err := service.PreviewProjectEdit(ctx, request)
		return projectPreviewMsg{preview: preview, err: err}
	}
}

func projectRemovePreviewCmd(ctx context.Context, service app.Service, request app.ProjectRemoveRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return mutationPreviewMsg{name: "remove project", err: errUnavailable("app service")}
		}
		preview, err := service.PreviewProjectRemove(ctx, request)
		return mutationPreviewMsg{name: "remove project", preview: preview, err: err}
	}
}

func projectEditCmd(ctx context.Context, service app.Service, request app.ProjectEditRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return projectOperationMsg{name: "project edit", err: errUnavailable("app service")}
		}
		result, err := service.EditProject(ctx, request)
		return projectOperationMsg{name: "project edit", result: result, err: err}
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

func syncCmd(ctx context.Context, service app.Service, request app.SyncRequest) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return operationMsg{name: "sync", err: errUnavailable("app service")}
		}
		result, err := service.Sync(ctx, request)
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
