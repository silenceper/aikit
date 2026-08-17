package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func (a *App) Batch(ctx context.Context, request BatchRequest) (BatchResult, error) {
	if !request.Confirmed {
		return BatchResult{}, fmt.Errorf("batch mutation requires confirmation")
	}
	switch request.Operation {
	case BatchEnable, BatchDisable:
		return a.batchBindings(ctx, request)
	case BatchRemove:
		return a.batchRemove(ctx, request)
	case BatchUpdate:
		return a.batchUpdate(ctx, request)
	default:
		return BatchResult{}, fmt.Errorf("unknown batch operation %q", request.Operation)
	}
}

func (a *App) PreviewBatch(ctx context.Context, request BatchRequest) (BatchPreview, error) {
	cfg, err := a.deps.Store.Load(ctx)
	if err != nil {
		return BatchPreview{}, err
	}
	switch request.Operation {
	case BatchEnable, BatchDisable:
		preflight, err := a.preflightBatchBindings(cfg, request)
		if err != nil {
			return BatchPreview{}, err
		}
		preview := BatchPreview{MutationPreview: MutationPreview{
			Title:                "Batch " + string(request.Operation),
			Summary:              fmt.Sprintf("%s %d exact binding(s)", request.Operation, len(preflight.items)),
			AffectedScopes:       affectedScopes(cfg, preflight.selectors),
			Plan:                 preflight.plan,
			RequiresConfirmation: true,
		}, Items: preflight.items}
		addPlanDiagnostics(&preview.MutationPreview)
		return preview, nil
	case BatchRemove:
		preflight, err := a.preflightBatchRemove(cfg, request, true)
		if err != nil {
			return BatchPreview{}, err
		}
		preview := BatchPreview{MutationPreview: MutationPreview{
			Title:                "Remove selected skills",
			Summary:              fmt.Sprintf("Remove %d selected library skill(s)", len(preflight.items)),
			AffectedScopes:       preflight.scopes,
			References:           preflight.references,
			Plan:                 preflight.plan,
			RequiresForce:        len(preflight.references) > 0 && !request.Force,
			RequiresConfirmation: true,
		}, Items: preflight.items}
		addPlanDiagnostics(&preview.MutationPreview)
		return preview, nil
	case BatchUpdate:
		preflight, err := a.preflightBatchUpdate(cfg, request)
		if err != nil {
			return BatchPreview{}, err
		}
		parts := make([]string, 0, len(preflight.skills))
		for _, skill := range preflight.skills {
			expected := request.Expected[skill.ID]
			parts = append(parts, fmt.Sprintf("%s %s -> %s", skill.ID, shortBatchOID(skill.Resolved), shortBatchOID(expected.Remote)))
		}
		return BatchPreview{MutationPreview: MutationPreview{
			Title:                "Update selected skills",
			Summary:              strings.Join(parts, "; "),
			RequiresConfirmation: true,
		}, Items: preflight.items}, nil
	default:
		return BatchPreview{}, fmt.Errorf("unknown batch operation %q", request.Operation)
	}
}

type batchBindingPreflight struct {
	next      *config.Config
	selectors []link.Selector
	plan      link.Plan
	items     []BatchItemResult
}

func (a *App) preflightBatchBindings(cfg *config.Config, request BatchRequest) (batchBindingPreflight, error) {
	if len(request.Bindings) == 0 {
		return batchBindingPreflight{}, fmt.Errorf("binding batch is empty")
	}
	next := cloneConfig(cfg)
	var selectors []link.Selector
	var items []BatchItemResult
	for _, binding := range request.Bindings {
		if (binding.SkillID == "") == (binding.Preset == "") {
			return batchBindingPreflight{}, fmt.Errorf("exactly one skill or preset is required")
		}
		resolved := binding
		if binding.SkillID != "" {
			skill, err := findSkill(next, binding.SkillID)
			if err != nil {
				return batchBindingPreflight{}, err
			}
			resolved.SkillID = skill.ID
		} else if _, err := findPreset(next, binding.Preset); err != nil {
			return batchBindingPreflight{}, err
		}
		itemSelectors, err := bindingSelectors(next, resolved)
		if err != nil {
			return batchBindingPreflight{}, err
		}
		selectors = append(selectors, itemSelectors...)
		if err := mutateBinding(next, resolved, request.Operation == BatchEnable); err != nil {
			return batchBindingPreflight{}, err
		}
		items = append(items, BatchItemResult{Item: bindingItem(resolved), Changed: true})
	}
	if err := next.Validate(); err != nil {
		return batchBindingPreflight{}, err
	}
	if err := validateEffective(next); err != nil {
		return batchBindingPreflight{}, err
	}
	selectors = uniqueSelectors(selectors)
	plan := plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(next, a.deps.UserHome), nil, selectors)
	if err := rejectPlanIssues(request.Operation, plan); err != nil {
		return batchBindingPreflight{}, err
	}
	return batchBindingPreflight{next: next, selectors: selectors, plan: plan, items: items}, nil
}

func (a *App) batchBindings(ctx context.Context, request BatchRequest) (BatchResult, error) {
	var output BatchResult
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		oldConfig := cloneConfig(tx.Config)
		preflight, err := a.preflightBatchBindings(tx.Config, request)
		if err != nil {
			return err
		}
		output.Items = append(output.Items, preflight.items...)
		plan := preflight.plan
		forwardOperations, err := recoveryOperationsForPlan(plan, config.TransactionForward, "batch binding forward")
		if err != nil {
			return err
		}
		if err := attachRollbackIntents(plan, forwardOperations, "batch binding rollback"); err != nil {
			return err
		}
		preflight.next.PendingOperations = append(preflight.next.PendingOperations, forwardOperations...)
		*tx.Config = *preflight.next
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		executed := a.deps.Recover(a.deps.Paths.LibrarySkills, forwardOperations, link.Selector{}, false)
		if !completedWithoutFailures(executed) {
			return a.rollbackConfigMutation(tx, oldConfig, forwardOperations, executed)
		}
		removeCompleted(tx.Config, executed.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output.Result = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		output.Issues = operationIssues(string(request.Operation), executed)
		return nil
	})
	return output, err
}

func (a *App) batchRemove(ctx context.Context, request BatchRequest) (BatchResult, error) {
	if a.deps.Library == nil {
		return BatchResult{}, fmt.Errorf("library mutation service is required")
	}
	var output BatchResult
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		originalConfig := cloneConfig(tx.Config)
		preflight, err := a.preflightBatchRemove(tx.Config, request, false)
		if err != nil {
			return err
		}
		output.Items = append(output.Items, preflight.items...)
		rollbackOperations, err := rollbackOperationsForCleanup(a.deps.Paths.LibrarySkills, preflight.cleanup, "batch remove rollback")
		if err != nil {
			return err
		}
		forwardOperations := operationsWithTransaction(preflight.cleanup, config.TransactionForward)
		for i := range forwardOperations {
			rollbackOperations[i].TransactionID = forwardOperations[i].TransactionID
			rollbackOperations[i].TransactionPhase = config.TransactionRollback
			forwardOperations[i].Rollback = &rollbackOperations[i]
		}
		finalConfig := cloneConfig(preflight.next)
		finalConfig.PendingOperations = append(finalConfig.PendingOperations, forwardOperations...)
		mutation, err := a.deps.Library.PrepareRemoveBatch(ctx, preflight.skills)
		if err != nil {
			return err
		}
		if a.deps.AfterRemovePrepare != nil {
			a.deps.AfterRemovePrepare()
		}
		*tx.Config = *finalConfig
		if err := tx.Checkpoint(); err != nil {
			_ = mutation.Abort()
			return err
		}
		if a.deps.AfterRemoveCheckpoint != nil {
			a.deps.AfterRemoveCheckpoint()
		}
		committed := false
		defer func() {
			if !committed {
				_ = mutation.Abort()
			}
		}()
		recovered := a.deps.Recover(a.deps.Paths.LibrarySkills, forwardOperations, link.Selector{}, false)
		if !completedWithoutFailures(recovered) {
			pending := rollbackTransactionOperations(forwardOperations)
			rollbackConfig := cloneConfig(originalConfig)
			rollbackConfig.PendingOperations = append(rollbackConfig.PendingOperations, pending...)
			*tx.Config = *rollbackConfig
			if checkpointErr := tx.Checkpoint(); checkpointErr != nil {
				return fmt.Errorf("checkpoint batch remove rollback direction: %w", checkpointErr)
			}
			rollback := a.deps.Recover(a.deps.Paths.LibrarySkills, pending, link.Selector{}, false)
			removeCompleted(tx.Config, rollback.Completed)
			if checkpointErr := tx.Checkpoint(); checkpointErr != nil {
				return fmt.Errorf("checkpoint batch remove cleanup rollback: %w", checkpointErr)
			}
			if !completedWithoutFailures(rollback) {
				return pendingRecoveryError(tx.Config.PendingOperations)
			}
			return fmt.Errorf("batch remove cleanup failed and was rolled back: %w", resultError(recovered))
		}
		if err := mutation.Commit(ctx); err != nil {
			pending := rollbackTransactionOperations(forwardOperations)
			restored := cloneConfig(originalConfig)
			restored.PendingOperations = append(restored.PendingOperations, pending...)
			*tx.Config = *restored
			if restoreErr := tx.Checkpoint(); restoreErr != nil {
				return fmt.Errorf("commit batch remove: %v; restore original config: %w", err, restoreErr)
			}
			if recoverErr := a.recoverLibrary(ctx, originalConfig.Library.Skills); recoverErr != nil {
				return errors.Join(err, recoverErr, pendingRecoveryError(tx.Config.PendingOperations))
			}
			reconciled := a.deps.Recover(a.deps.Paths.LibrarySkills, pending, link.Selector{}, false)
			removeCompleted(tx.Config, reconciled.Completed)
			if checkpointErr := tx.Checkpoint(); checkpointErr != nil {
				return fmt.Errorf("checkpoint batch remove rollback: %w", checkpointErr)
			}
			if !completedWithoutFailures(reconciled) {
				return errors.Join(err, pendingRecoveryError(tx.Config.PendingOperations))
			}
			return err
		}
		committed = true
		output.Result = Result{Plan: cleanupPlan(a.deps.Paths.LibrarySkills, forwardOperations), Link: recovered, Changed: true, Exit: ExitOK}
		removeOperationRecords(tx.Config, forwardOperations)
		return tx.Checkpoint()
	})
	return output, err
}

type batchRemovePreflight struct {
	next       *config.Config
	skills     []config.Skill
	cleanup    []config.PendingOperation
	items      []BatchItemResult
	references []string
	scopes     []config.Scope
	plan       link.Plan
}

func (a *App) preflightBatchRemove(cfg *config.Config, request BatchRequest, allowForcePreview bool) (batchRemovePreflight, error) {
	ids := uniqueStrings(request.SkillIDs)
	if len(ids) == 0 {
		return batchRemovePreflight{}, fmt.Errorf("remove batch is empty")
	}
	next := cloneConfig(cfg)
	result := batchRemovePreflight{next: next}
	for _, requested := range ids {
		skill, err := findSkill(next, requested)
		if err != nil {
			return batchRemovePreflight{}, err
		}
		if skill.ID != requested {
			return batchRemovePreflight{}, fmt.Errorf("batch skill %q must be a full id", requested)
		}
		refs := skillReferences(next, skill.ID)
		if len(refs) > 0 && !request.Force && !allowForcePreview {
			return batchRemovePreflight{}, fmt.Errorf("skill %q is still referenced by %v", skill.ID, refs)
		}
		for _, ref := range refs {
			result.references = append(result.references, skill.ID+": "+ref)
		}
		selectors := selectorsForSkill(next, skill.ID)
		result.scopes = append(result.scopes, affectedScopes(next, selectors)...)
		operations, err := cleanupForSkill(next, a.deps.UserHome, skill.ID, selectors)
		if err != nil {
			return batchRemovePreflight{}, err
		}
		result.cleanup = append(result.cleanup, operations...)
		if request.Force || allowForcePreview {
			pruneSkillReferences(next, skill.ID)
		}
		result.skills = append(result.skills, skill)
		result.items = append(result.items, BatchItemResult{Item: skill.ID, Changed: true})
	}
	for _, skill := range result.skills {
		if index := skillIndex(next, skill.ID); index >= 0 {
			next.Library.Skills = append(next.Library.Skills[:index], next.Library.Skills[index+1:]...)
		}
	}
	if err := next.Validate(); err != nil {
		return batchRemovePreflight{}, err
	}
	if err := preflightCleanup(a.deps.Paths.LibrarySkills, result.cleanup); err != nil {
		return batchRemovePreflight{}, err
	}
	result.scopes = uniqueScopes(result.scopes)
	sort.Strings(result.references)
	result.plan = cleanupPlan(a.deps.Paths.LibrarySkills, result.cleanup)
	return result, nil
}

func rollbackOperationsForCleanup(libraryRoot string, cleanup []config.PendingOperation, reason string) ([]config.PendingOperation, error) {
	operations := make([]config.PendingOperation, 0, len(cleanup))
	for _, cleanupOperation := range cleanup {
		operation, err := link.NewReconcileOperation("", cleanupOperation.Scope, cleanupOperation.Target, cleanupOperation.SkillID, libraryRoot, reason)
		if err != nil {
			return nil, err
		}
		// Cleanup can have completed before rollback begins. Absence is therefore
		// an authenticated state in addition to the exact pre-cleanup link.
		operation.ExpectedAbsent = true
		operations = append(operations, operation)
	}
	if len(operations) == 0 {
		return nil, nil
	}
	return operationsWithTransaction(operations, config.TransactionRollback), nil
}

func operationsWithTransaction(operations []config.PendingOperation, phase config.TransactionPhase) []config.PendingOperation {
	result := append([]config.PendingOperation(nil), operations...)
	if len(result) == 0 {
		return result
	}
	transactionID := "tx-" + result[0].ID
	for i := range result {
		result[i].TransactionID = transactionID
		result[i].TransactionPhase = phase
	}
	return result
}

func (a *App) batchUpdate(ctx context.Context, request BatchRequest) (BatchResult, error) {
	if a.deps.Library == nil {
		return BatchResult{}, fmt.Errorf("library mutation service is required")
	}
	var output BatchResult
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		oldConfig := cloneConfig(tx.Config)
		preflight, err := a.preflightBatchUpdate(tx.Config, request)
		if err != nil {
			return err
		}
		items := make([]UpdatePrepareItem, 0, len(preflight.skills))
		for _, skill := range preflight.skills {
			items = append(items, UpdatePrepareItem{Skill: skill, Ref: request.Ref})
		}
		output.Items = append(output.Items, preflight.items...)
		mutation, err := a.deps.Library.PrepareUpdate(ctx, items)
		if err != nil {
			return err
		}
		checkpointed := false
		defer func() {
			if !checkpointed {
				_ = mutation.Abort()
			}
		}()
		updated := mutation.Entries()
		if len(updated) != len(items) {
			return fmt.Errorf("prepared update returned %d entries for %d selections", len(updated), len(items))
		}
		for _, skill := range updated {
			index := skillIndex(tx.Config, skill.ID)
			if index < 0 {
				return fmt.Errorf("prepared update returned unknown skill %q", skill.ID)
			}
			if expected := request.Expected[skill.ID]; expected.Remote != "" && skill.Resolved != expected.Remote {
				return fmt.Errorf("skill %q remote changed after confirmation", skill.ID)
			}
			tx.Config.Library.Skills[index] = skill
		}
		if err := tx.Config.Validate(); err != nil {
			return err
		}
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		checkpointed = true
		if err := mutation.Commit(ctx); err != nil {
			*tx.Config = *cloneConfig(oldConfig)
			if restoreErr := tx.Checkpoint(); restoreErr != nil {
				return fmt.Errorf("commit batch update: %v; restore old ledger: %w", err, restoreErr)
			}
			if recoverErr := a.recoverLibrary(ctx, oldConfig.Library.Skills); recoverErr != nil {
				return fmt.Errorf("commit batch update: %v; recover old library: %w", err, recoverErr)
			}
			return err
		}
		output.Result = Result{Skills: updated, Changed: true, Exit: ExitOK}
		return tx.Checkpoint()
	})
	return output, err
}

type batchUpdatePreflight struct {
	skills []config.Skill
	items  []BatchItemResult
}

func (a *App) preflightBatchUpdate(cfg *config.Config, request BatchRequest) (batchUpdatePreflight, error) {
	ids := uniqueStrings(request.SkillIDs)
	if len(ids) == 0 {
		return batchUpdatePreflight{}, fmt.Errorf("update batch is empty")
	}
	result := batchUpdatePreflight{}
	for _, requested := range ids {
		skill, err := findSkill(cfg, requested)
		if err != nil {
			return batchUpdatePreflight{}, err
		}
		if skill.ID != requested {
			return batchUpdatePreflight{}, fmt.Errorf("batch skill %q must be a full id", requested)
		}
		expected, ok := request.Expected[skill.ID]
		if !ok {
			return batchUpdatePreflight{}, fmt.Errorf("skill %q is missing its confirmation token", skill.ID)
		}
		if expected.Resolved == "" || expected.Remote == "" {
			return batchUpdatePreflight{}, fmt.Errorf("skill %q confirmation token requires current resolved and remote identities", skill.ID)
		}
		if skill.Resolved != expected.Resolved {
			return batchUpdatePreflight{}, fmt.Errorf("skill %q changed after confirmation", skill.ID)
		}
		if !sameRef(skill.Ref, expected.Ref) {
			return batchUpdatePreflight{}, fmt.Errorf("skill %q ref changed after confirmation", skill.ID)
		}
		result.skills = append(result.skills, skill)
		result.items = append(result.items, BatchItemResult{Item: skill.ID, Changed: true})
	}
	return result, nil
}

func shortBatchOID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func (a *App) MutatePreset(ctx context.Context, request PresetMutationRequest) (Result, error) {
	if !request.Confirmed {
		return Result{}, fmt.Errorf("preset mutation requires confirmation")
	}
	if request.Name == "" {
		return Result{}, fmt.Errorf("preset name is required")
	}
	var output Result
	err := a.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := a.beforeMutation(ctx, tx.Config); err != nil {
			return err
		}
		oldConfig := cloneConfig(tx.Config)
		next := cloneConfig(tx.Config)
		beforeSelectors := selectorsForPreset(next, request.Name)
		if err := mutatePresetConfig(next, request); err != nil {
			return err
		}
		if err := next.Validate(); err != nil {
			return err
		}
		if err := validateEffective(next); err != nil {
			return err
		}
		name := request.Name
		if request.Operation == PresetRename || request.Operation == PresetDuplicate {
			name = request.NewName
		}
		selectors := uniqueSelectors(append(beforeSelectors, selectorsForPreset(next, name)...))
		plan := plansForSelectors(a.deps.Paths.LibrarySkills, buildTargets(next, a.deps.UserHome), nil, selectors)
		if err := rejectPlanIssues(request.Operation, plan); err != nil {
			return err
		}
		forwardOperations, err := recoveryOperationsForPlan(plan, config.TransactionForward, "preset mutation forward")
		if err != nil {
			return err
		}
		if err := attachRollbackIntents(plan, forwardOperations, "preset mutation rollback"); err != nil {
			return err
		}
		next.PendingOperations = append(next.PendingOperations, forwardOperations...)
		*tx.Config = *next
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		executed := a.deps.Recover(a.deps.Paths.LibrarySkills, forwardOperations, link.Selector{}, false)
		if !completedWithoutFailures(executed) {
			return a.rollbackConfigMutation(tx, oldConfig, forwardOperations, executed)
		}
		removeCompleted(tx.Config, executed.Completed)
		if err := tx.Checkpoint(); err != nil {
			return err
		}
		output = Result{Plan: plan, Link: executed, Changed: true, Exit: classify(executed)}
		return nil
	})
	return output, err
}

func (a *App) rollbackConfigMutation(tx *config.Tx, oldConfig *config.Config, forwardOperations []config.PendingOperation, forward link.Result) error {
	pending := rollbackTransactionOperations(forwardOperations)
	restored := cloneConfig(oldConfig)
	restored.PendingOperations = append(restored.PendingOperations, pending...)
	*tx.Config = *restored
	if err := tx.Checkpoint(); err != nil {
		return fmt.Errorf("restore old config and rollback intent after partial execution: %w", err)
	}
	rollback := a.deps.Recover(a.deps.Paths.LibrarySkills, pending, link.Selector{}, false)
	removeCompleted(tx.Config, rollback.Completed)
	if completedWithoutFailures(rollback) {
		if err := tx.Checkpoint(); err != nil {
			return fmt.Errorf("clear completed rollback journal: %w", err)
		}
		return fmt.Errorf("forward execution failed and was rolled back: %w", resultError(forward))
	}
	if err := tx.Checkpoint(); err != nil {
		return fmt.Errorf("checkpoint incomplete rollback recovery: %w", err)
	}
	return pendingRecoveryError(tx.Config.PendingOperations)
}

func attachRollbackIntents(plan link.Plan, forward []config.PendingOperation, reason string) error {
	if len(plan.Actions) != len(forward) {
		return fmt.Errorf("forward recovery operations do not cover every action")
	}
	for i, action := range plan.Actions {
		var rollback config.PendingOperation
		var err error
		switch action.Kind {
		case link.ActionCreate:
			rollback, err = link.NewCleanupOperation(forward[i].ID+"-rollback", action.Scope, action.Path, action.SkillID, reason)
			if err == nil {
				rollback.ExpectedSkillID = action.SkillID
				rollback.Expected, err = link.ExpectedManagedFingerprint(action.Library, action.SkillID)
			}
		case link.ActionRemove:
			rollback, err = link.NewReconcileOperation(forward[i].ID+"-rollback", action.Scope, action.Path, action.SkillID, action.Library, reason)
			rollback.ExpectedAbsent = true
		case link.ActionReplace:
			rollback, err = link.NewReconcileOperation(forward[i].ID+"-rollback", action.Scope, action.Path, action.PreviousID, action.Library, reason)
			if err == nil {
				rollback.ExpectedSkillID = action.SkillID
				rollback.Expected, err = link.ExpectedManagedFingerprint(action.Library, action.SkillID)
			}
		}
		if err != nil {
			return err
		}
		rollback.TransactionID = forward[i].TransactionID
		rollback.TransactionPhase = config.TransactionRollback
		forward[i].Rollback = &rollback
	}
	return nil
}

func rollbackOperations(forward []config.PendingOperation) []config.PendingOperation {
	result := make([]config.PendingOperation, 0, len(forward))
	for _, operation := range forward {
		if operation.Rollback != nil {
			result = append(result, *operation.Rollback)
		}
	}
	return result
}

// rollbackTransactionOperations retains each forward record as the exclusive
// owner of its deterministic tombstone. Its authenticated rollback child is
// persisted separately and cannot run until the source record is resolved.
func rollbackTransactionOperations(forward []config.PendingOperation) []config.PendingOperation {
	result := make([]config.PendingOperation, 0, len(forward)*2)
	for _, operation := range forward {
		source := operation
		source.TransactionPhase = config.TransactionRollbackSource
		result = append(result, source)
	}
	for _, operation := range forward {
		if operation.Rollback == nil {
			continue
		}
		rollback := *operation.Rollback
		rollback.ParentOperationID = operation.ID
		result = append(result, rollback)
	}
	return result
}

func recoveryOperationsForPlan(plan link.Plan, phase config.TransactionPhase, reason string) ([]config.PendingOperation, error) {
	operations := make([]config.PendingOperation, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		var operation config.PendingOperation
		var err error
		switch action.Kind {
		case link.ActionRemove:
			operation, err = link.NewCleanupOperation("", action.Scope, action.Path, action.SkillID, reason)
		case link.ActionCreate, link.ActionReplace:
			operation, err = link.NewReconcileOperation("", action.Scope, action.Path, action.SkillID, action.Library, reason)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	if len(operations) == 0 {
		return nil, nil
	}
	return operationsWithTransaction(operations, phase), nil
}

func removeOperationRecords(cfg *config.Config, operations []config.PendingOperation) {
	ids := make([]string, 0, len(operations))
	for _, operation := range operations {
		ids = append(ids, operation.ID)
	}
	removeCompleted(cfg, ids)
}

func mutatePresetConfig(cfg *config.Config, request PresetMutationRequest) error {
	canonicalSkills := func(values []string) ([]string, error) {
		result := make([]string, 0, len(values))
		for _, value := range values {
			skill, err := findSkill(cfg, value)
			if err != nil {
				return nil, err
			}
			result = appendUnique(result, skill.ID)
		}
		sort.Strings(result)
		return result, nil
	}
	switch request.Operation {
	case PresetCreate:
		if _, err := findPreset(cfg, request.Name); err == nil {
			return fmt.Errorf("preset %q already exists", request.Name)
		}
		skills, err := canonicalSkills(request.Skills)
		if err != nil {
			return err
		}
		cfg.Presets = append(cfg.Presets, config.Preset{Name: request.Name, Skills: skills})
	case PresetDuplicate:
		if request.NewName == "" {
			return fmt.Errorf("new preset name is required")
		}
		source, err := findPreset(cfg, request.Name)
		if err != nil {
			return err
		}
		if _, err := findPreset(cfg, request.NewName); err == nil {
			return fmt.Errorf("preset %q already exists", request.NewName)
		}
		cfg.Presets = append(cfg.Presets, config.Preset{Name: request.NewName, Skills: append([]string(nil), source.Skills...)})
	case PresetRename:
		if request.NewName == "" {
			return fmt.Errorf("new preset name is required")
		}
		preset, err := findPreset(cfg, request.Name)
		if err != nil {
			return err
		}
		if _, err := findPreset(cfg, request.NewName); err == nil {
			return fmt.Errorf("preset %q already exists", request.NewName)
		}
		preset.Name = request.NewName
		renamePresetReferences(cfg, request.Name, request.NewName)
	case PresetEditMembers:
		preset, err := findPreset(cfg, request.Name)
		if err != nil {
			return err
		}
		skills, err := canonicalSkills(request.Skills)
		if err != nil {
			return err
		}
		preset.Skills = skills
	case PresetDelete:
		if refs := selectorsForPreset(cfg, request.Name); len(refs) > 0 && !request.Force {
			return fmt.Errorf("preset %q is still referenced", request.Name)
		}
		if request.Force {
			prunePresetReferences(cfg, request.Name)
		}
		index := presetIndex(cfg, request.Name)
		if index < 0 {
			return fmt.Errorf("preset %q not found", request.Name)
		}
		cfg.Presets = append(cfg.Presets[:index], cfg.Presets[index+1:]...)
	case PresetApply:
		if _, err := findPreset(cfg, request.Name); err != nil {
			return err
		}
		binding := request.Binding
		binding.SkillID = ""
		binding.Preset = request.Name
		if binding.Agent == "" && binding.Project == "" {
			return fmt.Errorf("preset application requires a target")
		}
		if err := mutateBinding(cfg, binding, true); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown preset mutation %q", request.Operation)
	}
	return nil
}

func renamePresetReferences(cfg *config.Config, oldName, newName string) {
	replace := func(values []string) []string {
		for i := range values {
			if values[i] == oldName {
				values[i] = newName
			}
		}
		return values
	}
	for name, binding := range cfg.Agents {
		binding.Presets = replace(binding.Presets)
		cfg.Agents[name] = binding
	}
	for i := range cfg.Projects {
		cfg.Projects[i].Presets = replace(cfg.Projects[i].Presets)
		for name, binding := range cfg.Projects[i].AgentBindings {
			binding.Presets = replace(binding.Presets)
			cfg.Projects[i].AgentBindings[name] = binding
		}
	}
}

func bindingItem(request BindingRequest) string {
	item := request.SkillID
	if item == "" {
		item = "preset:" + request.Preset
	}
	return request.Project + "\x00" + request.Agent + "\x00" + item
}

func rejectPlanIssues(operation any, plan link.Plan) error {
	if len(plan.Issues) == 0 {
		return nil
	}
	issue := plan.Issues[0]
	return fmt.Errorf("%v batch preflight conflict at %s: %s", operation, issue.Path, issue.Message)
}

func preflightCleanup(libraryRoot string, operations []config.PendingOperation) error {
	for _, operation := range operations {
		state, err := link.Inspect(operation.Target, libraryRoot)
		if err != nil {
			return err
		}
		if state.Kind == link.StateAbsent {
			continue
		}
		if state.Kind != link.StateManagedLink || state.SkillID != operation.SkillID {
			return fmt.Errorf("remove batch preflight conflict at %s", operation.Target)
		}
	}
	return nil
}

func abortMutations(mutations []LibraryMutation) {
	for i := len(mutations) - 1; i >= 0; i-- {
		_ = mutations[i].Abort()
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func operationIssues(operation string, result link.Result) []OperationIssue {
	issues := make([]OperationIssue, 0, len(result.Issues)+len(result.Failures))
	for _, issue := range append(append([]link.Issue(nil), result.Issues...), result.Failures...) {
		issues = append(issues, OperationIssue{Operation: operation, Item: issue.SkillID, Path: issue.Path, Message: issue.Message, Err: issue.Err})
	}
	return issues
}
