package app

import (
	"context"
	"fmt"
	"os"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) PreviewRecovery(ctx context.Context, request RecoveryRequest) (RecoveryPreview, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryPreview{}, err
	}
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return RecoveryPreview{}, err
	}
	return a.previewRecoveryConfig(cfg, request)
}

func (a *App) previewRecoveryConfig(cfg *config.Config, request RecoveryRequest) (RecoveryPreview, error) {
	selected, err := selectPendingOperations(cfg.PendingOperations, request.OperationIDs)
	if err != nil {
		return RecoveryPreview{}, err
	}
	preview := RecoveryPreview{Operations: make([]RecoveryOperation, 0, len(selected))}
	dry := a.deps.Recover(a.deps.Paths.LibrarySkills, selected, link.Selector{}, true)
	preview.Plan = link.Plan{Actions: dry.Actions, Issues: dry.Issues, Warnings: dry.Warnings}
	preview.Issues = operationIssues("preview-recovery", dry)
	for _, operation := range selected {
		item := RecoveryOperation{Operation: operation, CanResume: true}
		if operation.TransactionPhase == config.TransactionForward {
			item.CanRollback = false
			item.RollbackUnavailable = "forward transaction recovery can only resume its durable ledger direction"
			preview.Operations = append(preview.Operations, item)
			continue
		}
		if operation.TransactionPhase == config.TransactionRollback {
			item.CanRollback = false
			item.RollbackUnavailable = "rollback transaction recovery already converges the durable old state"
			preview.Operations = append(preview.Operations, item)
			continue
		}
		if operation.TransactionPhase == config.TransactionRollbackSource {
			item.CanRollback = false
			item.RollbackUnavailable = "rollback source owns a forward artifact and must resume toward the durable old state"
			preview.Operations = append(preview.Operations, item)
			continue
		}
		switch operation.Kind {
		case config.OperationCleanup:
			rollback := link.RollbackCleanup(a.deps.Paths.LibrarySkills, []config.PendingOperation{operation}, true)
			item.CanRollback = completedWithoutFailures(rollback) && len(rollback.Completed) == 1
			if !item.CanRollback {
				item.RollbackUnavailable = "cleanup target/tombstone do not match an authenticated rollback state"
			}
		case config.OperationReconcile:
			item.CanRollback = false
			item.RollbackUnavailable = "reconcile recovery already represents rollback toward durable config"
		case config.OperationAdopt:
			item.CanRollback, item.RollbackUnavailable = pristineAdoptRollback(operation)
		default:
			item.CanResume = false
			item.RollbackUnavailable = "the pending operation kind is unsupported"
		}
		preview.Operations = append(preview.Operations, item)
	}
	return preview, nil
}

func (a *App) ResumeRecovery(ctx context.Context, request RecoveryRequest) (RecoveryResult, error) {
	if !request.Confirmed {
		return RecoveryResult{}, fmt.Errorf("recovery resume requires confirmation")
	}
	var output RecoveryResult
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		selected, err := selectPendingOperations(tx.Config.PendingOperations, request.OperationIDs)
		if err != nil {
			return err
		}
		if err := a.beforeRecovery(ctx, tx.Config.Library.Skills); err != nil {
			return err
		}
		result := a.deps.Recover(a.deps.Paths.LibrarySkills, selected, link.Selector{}, false)
		removeCompleted(tx.Config, result.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = RecoveryResult{
			Completed: append([]string(nil), result.Completed...),
			Issues:    operationIssues("resume-recovery", result),
			Changed:   len(result.Completed) > 0 || len(result.Applied) > 0,
		}
		return nil
	})
	return output, err
}

func (a *App) RollbackRecovery(ctx context.Context, request RecoveryRequest) (RecoveryResult, error) {
	if !request.Confirmed {
		return RecoveryResult{}, fmt.Errorf("recovery rollback requires confirmation")
	}
	var output RecoveryResult
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		preview, err := a.previewRecoveryConfig(tx.Config, request)
		if err != nil {
			return err
		}
		for _, operation := range preview.Operations {
			if !operation.CanRollback {
				return fmt.Errorf("operation %q cannot be rolled back safely: %s", operation.Operation.ID, operation.RollbackUnavailable)
			}
		}
		if err := a.beforeRecovery(ctx, tx.Config.Library.Skills); err != nil {
			return err
		}
		ids := make([]string, 0, len(preview.Operations))
		standaloneCleanup := make([]config.PendingOperation, 0, len(preview.Operations))
		for _, item := range preview.Operations {
			ids = append(ids, item.Operation.ID)
			if item.Operation.Kind == config.OperationCleanup {
				standaloneCleanup = append(standaloneCleanup, item.Operation)
			}
		}
		if len(standaloneCleanup) > 0 {
			rolledBack := link.RollbackCleanup(a.deps.Paths.LibrarySkills, standaloneCleanup, false)
			if !completedWithoutFailures(rolledBack) || len(rolledBack.Completed) != len(standaloneCleanup) {
				return fmt.Errorf("cleanup rollback could not restore authenticated state")
			}
		}
		removeCompleted(tx.Config, ids)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = RecoveryResult{Completed: ids, Changed: len(ids) > 0}
		return nil
	})
	return output, err
}

func (a *App) beforeRecovery(ctx context.Context, ledger []config.Skill) error {
	if a.deps.LibraryRecovery == nil {
		return fmt.Errorf("library recovery service is required for mutations")
	}
	if err := os.MkdirAll(a.deps.Paths.LibrarySkills, 0o755); err != nil {
		return err
	}
	issues, err := a.deps.LibraryRecovery.RecoverBatches(ctx, ledger)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return fmt.Errorf("library batch recovery requires attention: %s", issues[0].Detail)
	}
	return nil
}

func selectPendingOperations(operations []config.PendingOperation, ids []string) ([]config.PendingOperation, error) {
	if len(ids) == 0 {
		return append([]config.PendingOperation(nil), operations...), nil
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("pending operation id is empty")
		}
		wanted[id] = struct{}{}
	}
	result := make([]config.PendingOperation, 0, len(wanted))
	for _, operation := range operations {
		if _, ok := wanted[operation.ID]; ok {
			result = append(result, operation)
			delete(wanted, operation.ID)
		}
	}
	for id := range wanted {
		return nil, fmt.Errorf("pending operation %q not found", id)
	}
	selectedIDs := make(map[string]struct{}, len(result))
	selectedTransactions := make(map[string]struct{})
	for _, operation := range result {
		selectedIDs[operation.ID] = struct{}{}
		if operation.TransactionID != "" {
			selectedTransactions[operation.TransactionID] = struct{}{}
		}
	}
	for _, operation := range operations {
		if _, grouped := selectedTransactions[operation.TransactionID]; !grouped || operation.TransactionID == "" {
			continue
		}
		if _, selected := selectedIDs[operation.ID]; !selected {
			return nil, fmt.Errorf("transaction %q must be recovered as one group", operation.TransactionID)
		}
	}
	return result, nil
}

func pristineAdoptRollback(operation config.PendingOperation) (bool, string) {
	return false, "adopt rollback is unavailable because the prior binding and library state were not journaled"
}
