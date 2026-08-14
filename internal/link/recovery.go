package link

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/pkg/config"
)

type FileOps struct {
	Symlink       func(string, string) error
	MoveNoReplace func(string, string) error
	Remove        func(string) error
	// Test-only crash/race seams around anchored reconcile operations.
	BeforeReconcileMutation  func()
	AfterReconcileMove       func()
	BeforeCleanupMutation    func()
	AfterCleanupMove         func()
	AfterCleanupRollbackMove func()
	FailReconcileSymlink     error
	FailCleanupUnlink        error
	BeforeDeleteQuarantine   func(string)
	AfterDeleteQuarantine    func(string)
}

func defaultFileOps() FileOps {
	return FileOps{Symlink: os.Symlink, MoveNoReplace: moveNoReplace, Remove: os.Remove}
}

func Recover(libraryRoot string, operations []config.PendingOperation, selector Selector, dryRun bool) Result {
	return RecoverWithOps(libraryRoot, operations, selector, dryRun, defaultFileOps())
}

func RecoverWithOps(libraryRoot string, operations []config.PendingOperation, selector Selector, dryRun bool, ops FileOps) Result {
	result := Result{}
	operationIDs := make(map[string]struct{}, len(operations))
	completed := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		operationIDs[operation.ID] = struct{}{}
	}
	for _, op := range operations {
		if !selector.Matches(op.Scope) {
			continue
		}
		action := Action{Kind: ActionRecover, Scope: op.Scope, Path: op.Target, SkillID: op.SkillID, Operation: op.ID, Library: libraryRoot}
		result.Actions = append(result.Actions, action)
		if op.ParentOperationID != "" {
			if _, selected := operationIDs[op.ParentOperationID]; selected {
				if _, done := completed[op.ParentOperationID]; !done {
					result.Issues = append(result.Issues, *recoveryIssue(IssuePendingReconcile, op, "rollback source artifact is unresolved", nil))
					continue
				}
			}
		}
		if dryRun {
			if issue := previewRecoveryOperation(libraryRoot, op); issue != nil {
				result.Issues = append(result.Issues, *issue)
			} else {
				completed[op.ID] = struct{}{}
			}
			continue
		}
		if err := validateOperationScopeTarget(op); err != nil {
			result.Issues = append(result.Issues, *recoveryIssue(IssueUnsafePath, op, err.Error(), err))
			continue
		}
		var complete bool
		var issue *Issue
		switch {
		case op.TransactionPhase == config.TransactionRollbackSource && op.Kind == config.OperationCleanup:
			complete, issue = rollbackCleanup(libraryRoot, op, false, ops)
		case op.TransactionPhase == config.TransactionRollbackSource && op.Kind == config.OperationReconcile:
			complete, issue = rollbackReconcileSource(libraryRoot, op, false, ops)
		case op.Kind == config.OperationCleanup:
			complete, issue = recoverCleanup(libraryRoot, op, ops)
		case op.Kind == config.OperationAdopt:
			complete, issue = recoverAdopt(libraryRoot, op, ops)
		case op.Kind == config.OperationReconcile:
			complete, issue = recoverReconcile(libraryRoot, op, ops)
		default:
			i := Issue{Kind: IssueIO, Scope: op.Scope, Path: op.Target, Operation: op.ID, Message: "unknown pending operation"}
			issue = &i
		}
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
			continue
		}
		if complete {
			completed[op.ID] = struct{}{}
			result.Completed = append(result.Completed, op.ID)
			result.Applied = append(result.Applied, action)
		}
	}
	return result
}

func previewRecoveryOperation(libraryRoot string, op config.PendingOperation) *Issue {
	if err := validateOperationScopeTarget(op); err != nil {
		return recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	if op.TransactionPhase == config.TransactionRollbackSource {
		var complete bool
		var issue *Issue
		if op.Kind == config.OperationCleanup {
			complete, issue = rollbackCleanup(libraryRoot, op, true, FileOps{})
		} else if op.Kind == config.OperationReconcile {
			complete, issue = rollbackReconcileSource(libraryRoot, op, true, FileOps{})
		}
		if issue != nil {
			return issue
		}
		if complete {
			return nil
		}
	}
	state, err := Inspect(op.Target, libraryRoot)
	if err != nil {
		return recoveryIssue(IssueIO, op, err.Error(), err)
	}
	switch op.Kind {
	case config.OperationCleanup:
		if state.Kind != StateAbsent && (state.Kind != StateManagedLink || state.SkillID != op.SkillID) {
			return recoveryIssue(IssuePendingCleanup, op, "target no longer matches the recorded aikit link", nil)
		}
	case config.OperationReconcile:
		if state.Kind == StateManagedLink && state.SkillID == op.SkillID && !state.Broken {
			return nil
		}
		if state.Kind == StateAbsent && op.ExpectedAbsent {
			return nil
		}
		if state.Kind != StateManagedLink || state.SkillID != op.ExpectedSkillID {
			return recoveryIssue(IssuePendingReconcile, op, "target does not match an authenticated reconcile state", nil)
		}
		matches, matchErr := matchesFingerprint(op.Target, op.Expected)
		if matchErr != nil || !matches {
			if matchErr == nil {
				matchErr = fmt.Errorf("managed target fingerprint changed")
			}
			return recoveryIssue(IssuePendingReconcile, op, matchErr.Error(), matchErr)
		}
	}
	return nil
}

func recoverReconcile(libraryRoot string, op config.PendingOperation, ops FileOps) (bool, *Issue) {
	if err := validateOperationScopeTarget(op); err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	lib, safe := libraryPath(libraryRoot, op.SkillID)
	if !safe {
		return false, recoveryIssue(IssuePendingReconcile, op, "unsafe library id", nil)
	}
	info, err := os.Stat(lib)
	if err != nil || !info.IsDir() {
		return false, recoveryIssue(IssuePendingReconcile, op, "library directory is missing", err)
	}
	parent, err := openReconcileParent(op)
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	defer parent.Close()
	targetName, err := reconcileBaseName(op.Target)
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	tombstoneName, err := reconcileBaseName(op.Tombstone)
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	if ops.BeforeReconcileMutation != nil {
		ops.BeforeReconcileMutation()
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		if currentErr == nil {
			currentErr = fmt.Errorf("reconcile parent changed after it was anchored")
		}
		return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
	}
	targetState, err := parent.State(targetName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	tombstoneState, err := parent.State(tombstoneName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	matchesExpected := func(name string, state State) bool {
		if op.Expected == nil || state.Kind != StateManagedLink || state.SkillID != op.ExpectedSkillID {
			return false
		}
		fingerprint, fingerprintErr := parent.Fingerprint(name)
		return fingerprintErr == nil && sameFingerprint(fingerprint, op.Expected)
	}
	desired := func(state State) bool {
		return state.Kind == StateManagedLink && state.SkillID == op.SkillID && !state.Broken && filepath.Clean(state.LinkTarget) == filepath.Clean(lib)
	}
	if present, stateErr := parent.State(deleteQuarantineName(op, "tombstone"), libraryRoot); stateErr != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, stateErr.Error(), stateErr)
	} else if present.Kind != StateAbsent {
		if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, tombstoneName, "tombstone", matchesExpected, ops, IssuePendingReconcile); issue != nil {
			return false, issue
		}
		targetState, err = parent.State(targetName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		tombstoneState, err = parent.State(tombstoneName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
	}
	tombstoneExpected := matchesExpected(tombstoneName, tombstoneState)
	if tombstoneState.Kind != StateAbsent && !tombstoneExpected {
		return false, recoveryIssue(IssuePendingReconcile, op, "reconcile tombstone does not match the authenticated old link", nil)
	}
	if desired(targetState) {
		if tombstoneExpected {
			if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, tombstoneName, "tombstone", matchesExpected, ops, IssuePendingReconcile); issue != nil {
				return false, issue
			}
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			if currentErr == nil {
				currentErr = fmt.Errorf("reconcile parent changed during recovery")
			}
			return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
		}
		return true, nil
	}
	if matchesExpected(targetName, targetState) {
		if tombstoneState.Kind != StateAbsent {
			return false, recoveryIssue(IssuePendingReconcile, op, "reconcile target and tombstone both contain the old link", nil)
		}
		if err := parent.MoveNoReplace(targetName, tombstoneName); err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		if ops.AfterReconcileMove != nil {
			ops.AfterReconcileMove()
		}
		tombstoneState, err = parent.State(tombstoneName, libraryRoot)
		if err != nil || !matchesExpected(tombstoneName, tombstoneState) {
			if err == nil {
				err = fmt.Errorf("quarantined reconcile link changed")
			}
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		tombstoneExpected = true
		targetState = State{Kind: StateAbsent}
	}
	if targetState.Kind != StateAbsent || (!op.ExpectedAbsent && !tombstoneExpected) {
		return false, recoveryIssue(IssuePendingReconcile, op, "target does not match an authenticated reconcile state", nil)
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		if currentErr == nil {
			currentErr = fmt.Errorf("reconcile parent changed before link creation")
		}
		return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
	}
	if ops.FailReconcileSymlink != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, ops.FailReconcileSymlink.Error(), ops.FailReconcileSymlink)
	}
	if err := parent.Symlink(lib, targetName); err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	verified, err := parent.State(targetName, libraryRoot)
	if err != nil || !desired(verified) {
		if err == nil {
			err = fmt.Errorf("reconciled target does not match skill")
		}
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	if tombstoneExpected {
		if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, tombstoneName, "tombstone", matchesExpected, ops, IssuePendingReconcile); issue != nil {
			return false, issue
		}
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		if currentErr == nil {
			currentErr = fmt.Errorf("reconcile parent changed during recovery")
		}
		return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
	}
	return true, nil
}

func recoverCleanup(libraryRoot string, op config.PendingOperation, ops FileOps) (bool, *Issue) {
	parent, err := openReconcileParent(op)
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	defer parent.Close()
	targetName, err := reconcileBaseName(op.Target)
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	tombstoneName, err := reconcileBaseName(op.Tombstone)
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	if ops.BeforeCleanupMutation != nil {
		ops.BeforeCleanupMutation()
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		if currentErr == nil {
			currentErr = fmt.Errorf("cleanup parent changed after it was anchored")
		}
		return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
	}
	targetState, err := parent.State(targetName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	tombstoneState, err := parent.State(tombstoneName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	matchesExpected := func(name string, state State) bool {
		if op.Expected == nil || op.ExpectedSkillID != op.SkillID {
			return false
		}
		if state.Kind == StateExternalLink && state.Broken {
			return filepath.Clean(state.LinkTarget) == filepath.Clean(op.Expected.LinkTarget)
		}
		if state.Kind != StateManagedLink || state.SkillID != op.ExpectedSkillID {
			return false
		}
		fingerprint, fingerprintErr := parent.Fingerprint(name)
		return fingerprintErr == nil && sameFingerprint(fingerprint, op.Expected)
	}
	if present, stateErr := parent.State(deleteQuarantineName(op, "tombstone"), libraryRoot); stateErr != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, stateErr.Error(), stateErr)
	} else if present.Kind != StateAbsent {
		if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, tombstoneName, "tombstone", matchesExpected, ops, IssuePendingCleanup); issue != nil {
			return false, issue
		}
		targetState, err = parent.State(targetName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
		}
		tombstoneState, err = parent.State(tombstoneName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
		}
	}
	tombstoneExpected := matchesExpected(tombstoneName, tombstoneState)
	if tombstoneState.Kind != StateAbsent && !tombstoneExpected {
		return false, recoveryIssue(IssuePendingCleanup, op, "cleanup tombstone does not match the authenticated link", nil)
	}
	if matchesExpected(targetName, targetState) {
		if tombstoneState.Kind != StateAbsent {
			return false, recoveryIssue(IssuePendingCleanup, op, "cleanup target and tombstone both contain the authenticated link", nil)
		}
		if err := parent.MoveNoReplace(targetName, tombstoneName); err != nil {
			return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
		}
		if ops.AfterCleanupMove != nil {
			ops.AfterCleanupMove()
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			if currentErr == nil {
				currentErr = fmt.Errorf("cleanup parent changed after link move")
			}
			return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
		}
		tombstoneState, err = parent.State(tombstoneName, libraryRoot)
		if err != nil || !matchesExpected(tombstoneName, tombstoneState) {
			if err == nil {
				err = fmt.Errorf("quarantined cleanup link changed")
			}
			return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
		}
		tombstoneExpected = true
		targetState = State{Kind: StateAbsent}
	}
	if targetState.Kind != StateAbsent {
		return false, recoveryIssue(IssuePendingCleanup, op, "target no longer matches the authenticated aikit link", nil)
	}
	if tombstoneExpected {
		if ops.FailCleanupUnlink != nil {
			return false, recoveryIssue(IssuePendingCleanup, op, ops.FailCleanupUnlink.Error(), ops.FailCleanupUnlink)
		}
		if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, tombstoneName, "tombstone", matchesExpected, ops, IssuePendingCleanup); issue != nil {
			return false, issue
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			if currentErr == nil {
				currentErr = fmt.Errorf("cleanup parent changed after unlink")
			}
			return false, recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
		}
		return true, nil
	}
	if !op.ExpectedAbsent && op.Expected == nil {
		return false, recoveryIssue(IssuePendingCleanup, op, "absent cleanup target has no authenticated prior evidence", nil)
	}
	return true, nil
}

func RollbackCleanup(libraryRoot string, operations []config.PendingOperation, dryRun bool) Result {
	return RollbackCleanupWithOps(libraryRoot, operations, dryRun, defaultFileOps())
}

func RollbackCleanupWithOps(libraryRoot string, operations []config.PendingOperation, dryRun bool, ops FileOps) Result {
	result := Result{}
	for _, op := range operations {
		action := Action{Kind: ActionRecover, Scope: op.Scope, Path: op.Target, SkillID: op.SkillID, Operation: op.ID, Library: libraryRoot}
		result.Actions = append(result.Actions, action)
		complete, issue := rollbackCleanup(libraryRoot, op, dryRun, ops)
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
			continue
		}
		if complete {
			result.Completed = append(result.Completed, op.ID)
			if !dryRun {
				result.Applied = append(result.Applied, action)
			}
		}
	}
	return result
}

func rollbackCleanup(libraryRoot string, op config.PendingOperation, dryRun bool, ops FileOps) (bool, *Issue) {
	rollbackSource := op.TransactionPhase == config.TransactionRollbackSource
	if op.Kind != config.OperationCleanup || (op.TransactionPhase != "" && !rollbackSource) {
		return false, recoveryIssue(IssuePendingCleanup, op, "cleanup cannot be rolled back in this transaction phase", nil)
	}
	if dryRun && op.ExpectedAbsent {
		if _, err := os.Lstat(filepath.Dir(op.Target)); os.IsNotExist(err) {
			return true, nil
		}
	}
	var parent reconcileParent
	var err error
	if dryRun {
		parent, err = openReconcileParentReadOnly(op)
	} else {
		parent, err = openReconcileParent(op)
	}
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	defer parent.Close()
	targetName, _ := reconcileBaseName(op.Target)
	tombstoneName, _ := reconcileBaseName(op.Tombstone)
	targetState, err := parent.State(targetName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	tombstoneState, err := parent.State(tombstoneName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	matches := func(name string, state State) bool {
		if op.Expected == nil || state.Kind != StateManagedLink || state.SkillID != op.ExpectedSkillID || state.SkillID != op.SkillID {
			return false
		}
		fingerprint, fingerprintErr := parent.Fingerprint(name)
		return fingerprintErr == nil && sameFingerprint(fingerprint, op.Expected)
	}
	if rollbackSource {
		if restored, restoreIssue := restoreDeleteQuarantine(parent, libraryRoot, op, tombstoneName, "tombstone", matches, dryRun); restoreIssue != nil {
			return false, restoreIssue
		} else if restored && !dryRun {
			targetState, err = parent.State(targetName, libraryRoot)
			if err != nil {
				return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
			}
			tombstoneState, err = parent.State(tombstoneName, libraryRoot)
			if err != nil {
				return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
			}
		}
	}
	if matches(targetName, targetState) && tombstoneState.Kind == StateAbsent {
		return true, nil
	}
	if op.ExpectedAbsent && targetState.Kind == StateAbsent && tombstoneState.Kind == StateAbsent {
		return true, nil
	}
	if rollbackSource && targetState.Kind == StateAbsent && tombstoneState.Kind == StateAbsent {
		// The forward cleanup finished before rollback direction was persisted;
		// its authenticated rollback child is responsible for restoring old state.
		return true, nil
	}
	if targetState.Kind == StateAbsent && matches(tombstoneName, tombstoneState) {
		if dryRun {
			return true, nil
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			return false, recoveryIssue(IssueUnsafePath, op, "cleanup parent changed before rollback", currentErr)
		}
		if err := parent.MoveNoReplace(tombstoneName, targetName); err != nil {
			return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
		}
		if ops.AfterCleanupRollbackMove != nil {
			ops.AfterCleanupRollbackMove()
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			return false, recoveryIssue(IssueUnsafePath, op, "cleanup parent changed after rollback move", currentErr)
		}
		if state, stateErr := parent.State(targetName, libraryRoot); stateErr != nil || !matches(targetName, state) {
			return false, recoveryIssue(IssuePendingCleanup, op, "restored cleanup target failed authentication", stateErr)
		}
		return true, nil
	}
	return false, recoveryIssue(IssuePendingCleanup, op, "cleanup execution state cannot be rolled back safely", nil)
}

// rollbackReconcileSource owns artifacts created by an interrupted forward
// reconcile. It authenticates both the old tombstone and any installed
// forward link before restoring the old target; the rollback child may run
// only after this source operation completes.
func rollbackReconcileSource(libraryRoot string, op config.PendingOperation, dryRun bool, ops FileOps) (bool, *Issue) {
	var parent reconcileParent
	var err error
	if dryRun {
		parent, err = openReconcileParentReadOnly(op)
	} else {
		parent, err = openReconcileParent(op)
	}
	if err != nil {
		return false, recoveryIssue(IssueUnsafePath, op, err.Error(), err)
	}
	defer parent.Close()
	targetName, _ := reconcileBaseName(op.Target)
	tombstoneName, _ := reconcileBaseName(op.Tombstone)
	targetState, err := parent.State(targetName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	tombstoneState, err := parent.State(tombstoneName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	matches := func(name string, state State, expectedID string, expected *config.Fingerprint) bool {
		if expected == nil || state.Kind != StateManagedLink || state.SkillID != expectedID {
			return false
		}
		fingerprint, fingerprintErr := parent.Fingerprint(name)
		return fingerprintErr == nil && sameFingerprint(fingerprint, expected)
	}
	oldMatches := func(name string, state State) bool {
		return matches(name, state, op.ExpectedSkillID, op.Expected)
	}
	if restored, restoreIssue := restoreDeleteQuarantine(parent, libraryRoot, op, tombstoneName, "tombstone", oldMatches, dryRun); restoreIssue != nil {
		return false, restoreIssue
	} else if restored && !dryRun {
		targetState, err = parent.State(targetName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		tombstoneState, err = parent.State(tombstoneName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
	}
	oldTarget := matches(targetName, targetState, op.ExpectedSkillID, op.Expected)
	oldTombstone := matches(tombstoneName, tombstoneState, op.ExpectedSkillID, op.Expected)
	forwardTarget := false
	if op.Rollback != nil {
		forwardTarget = matches(targetName, targetState, op.Rollback.ExpectedSkillID, op.Rollback.Expected)
	}
	forwardMatches := func(name string, state State) bool {
		return op.Rollback != nil && matches(name, state, op.Rollback.ExpectedSkillID, op.Rollback.Expected)
	}
	if present, stateErr := parent.State(deleteQuarantineName(op, "target"), libraryRoot); stateErr != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, stateErr.Error(), stateErr)
	} else if present.Kind != StateAbsent {
		if dryRun {
			if !forwardMatches(deleteQuarantineName(op, "target"), present) {
				return false, recoveryIssue(IssuePendingReconcile, op, "delete quarantine failed forward-link authentication", nil)
			}
		} else if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, targetName, "target", forwardMatches, ops, IssuePendingReconcile); issue != nil {
			return false, issue
		}
		targetState, err = parent.State(targetName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		tombstoneState, err = parent.State(tombstoneName, libraryRoot)
		if err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		oldTarget = matches(targetName, targetState, op.ExpectedSkillID, op.Expected)
		oldTombstone = matches(tombstoneName, tombstoneState, op.ExpectedSkillID, op.Expected)
		forwardTarget = forwardMatches(targetName, targetState)
	}
	if tombstoneState.Kind != StateAbsent && !oldTombstone {
		return false, recoveryIssue(IssuePendingReconcile, op, "forward reconcile tombstone failed old-link authentication", nil)
	}
	if oldTarget && tombstoneState.Kind == StateAbsent {
		return true, nil
	}
	if op.ExpectedAbsent && targetState.Kind == StateAbsent && tombstoneState.Kind == StateAbsent {
		return true, nil
	}
	if forwardTarget && tombstoneState.Kind == StateAbsent {
		// Forward completed and consumed its tombstone. The dependent rollback
		// operation owns removing/replacing the authenticated forward target.
		return true, nil
	}
	if targetState.Kind == StateAbsent && oldTombstone {
		if dryRun {
			return true, nil
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			return false, recoveryIssue(IssueUnsafePath, op, "reconcile parent changed before source restore", currentErr)
		}
		if err := parent.MoveNoReplace(tombstoneName, targetName); err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		state, stateErr := parent.State(targetName, libraryRoot)
		if stateErr != nil || !matches(targetName, state, op.ExpectedSkillID, op.Expected) {
			return false, recoveryIssue(IssuePendingReconcile, op, "restored forward tombstone failed authentication", stateErr)
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			return false, recoveryIssue(IssueUnsafePath, op, "reconcile parent changed after source restore", currentErr)
		}
		return true, nil
	}
	if forwardTarget && oldTombstone {
		if dryRun {
			return true, nil
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			return false, recoveryIssue(IssueUnsafePath, op, "reconcile parent changed before source replacement", currentErr)
		}
		if issue := deleteAuthenticatedEntry(parent, libraryRoot, op, targetName, "target", forwardMatches, ops, IssuePendingReconcile); issue != nil {
			return false, issue
		}
		if err := parent.MoveNoReplace(tombstoneName, targetName); err != nil {
			return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
		}
		state, stateErr := parent.State(targetName, libraryRoot)
		if stateErr != nil || !matches(targetName, state, op.ExpectedSkillID, op.Expected) {
			return false, recoveryIssue(IssuePendingReconcile, op, "restored reconcile source failed authentication", stateErr)
		}
		if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
			return false, recoveryIssue(IssueUnsafePath, op, "reconcile parent changed after source replacement", currentErr)
		}
		return true, nil
	}
	return false, recoveryIssue(IssuePendingReconcile, op, "reconcile rollback source state is unknown", nil)
}

func deleteQuarantineName(op config.PendingOperation, role string) string {
	return ".aikit-delete-" + role + "-" + op.ID
}

func restoreDeleteQuarantine(parent reconcileParent, libraryRoot string, op config.PendingOperation, destinationName, role string, matches func(string, State) bool, dryRun bool) (bool, *Issue) {
	quarantineName := deleteQuarantineName(op, role)
	quarantineState, err := parent.State(quarantineName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	if quarantineState.Kind == StateAbsent {
		return false, nil
	}
	if !matches(quarantineName, quarantineState) {
		return false, recoveryIssue(IssuePendingReconcile, op, "delete quarantine failed rollback-source authentication", nil)
	}
	destinationState, err := parent.State(destinationName, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	if destinationState.Kind != StateAbsent {
		return false, recoveryIssue(IssuePendingReconcile, op, "rollback-source destination is occupied while delete quarantine remains", nil)
	}
	if dryRun {
		return true, nil
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		return false, recoveryIssue(IssueUnsafePath, op, "delete quarantine parent changed before rollback restore", currentErr)
	}
	if err := parent.MoveNoReplace(quarantineName, destinationName); err != nil {
		return false, recoveryIssue(IssuePendingReconcile, op, err.Error(), err)
	}
	state, stateErr := parent.State(destinationName, libraryRoot)
	if stateErr != nil || !matches(destinationName, state) {
		return false, recoveryIssue(IssuePendingReconcile, op, "restored delete quarantine failed authentication", stateErr)
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		return false, recoveryIssue(IssueUnsafePath, op, "delete quarantine parent changed after rollback restore", currentErr)
	}
	return true, nil
}

// deleteAuthenticatedEntry never unlinks the authenticated lexical entry
// directly. It atomically moves whichever object currently owns the name to a
// deterministic per-operation quarantine, authenticates that moved object,
// verifies the anchored parent still owns the lexical scope, and only then
// unlinks the quarantine. An unexpected moved object is restored when that can
// be done without replacement; otherwise it remains locatable and pending.
func deleteAuthenticatedEntry(parent reconcileParent, libraryRoot string, op config.PendingOperation, originalName, role string, matches func(string, State) bool, ops FileOps, issueKind IssueKind) *Issue {
	quarantineName := deleteQuarantineName(op, role)
	quarantineState, err := parent.State(quarantineName, libraryRoot)
	if err != nil {
		return recoveryIssue(issueKind, op, err.Error(), err)
	}
	if quarantineState.Kind == StateAbsent {
		originalState, stateErr := parent.State(originalName, libraryRoot)
		if stateErr != nil {
			return recoveryIssue(issueKind, op, stateErr.Error(), stateErr)
		}
		if originalState.Kind == StateAbsent {
			return nil
		}
		if !matches(originalName, originalState) {
			return recoveryIssue(issueKind, op, "delete source no longer matches the authenticated object", nil)
		}
		if ops.BeforeDeleteQuarantine != nil {
			ops.BeforeDeleteQuarantine(role)
		}
		if err := parent.MoveNoReplace(originalName, quarantineName); err != nil {
			return recoveryIssue(issueKind, op, err.Error(), err)
		}
		if ops.AfterDeleteQuarantine != nil {
			ops.AfterDeleteQuarantine(role)
		}
		quarantineState, err = parent.State(quarantineName, libraryRoot)
		if err != nil {
			return recoveryIssue(issueKind, op, err.Error(), err)
		}
	}
	if !matches(quarantineName, quarantineState) {
		originalState, stateErr := parent.State(originalName, libraryRoot)
		if stateErr == nil && originalState.Kind == StateAbsent {
			if current, currentErr := parent.StillCurrent(); currentErr == nil && current {
				_ = parent.MoveNoReplace(quarantineName, originalName)
			}
		}
		return recoveryIssue(issueKind, op, "delete quarantine failed authenticated object validation", nil)
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		if currentErr == nil {
			currentErr = fmt.Errorf("delete quarantine parent changed")
		}
		return recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
	}
	if err := parent.Remove(quarantineName); err != nil {
		return recoveryIssue(issueKind, op, err.Error(), err)
	}
	if current, currentErr := parent.StillCurrent(); currentErr != nil || !current {
		if currentErr == nil {
			currentErr = fmt.Errorf("delete quarantine parent changed after unlink")
		}
		return recoveryIssue(IssueUnsafePath, op, currentErr.Error(), currentErr)
	}
	return nil
}

func validateOperationScopeTarget(op config.PendingOperation) error {
	t := Target{Scope: op.Scope, Dir: filepath.Dir(op.Target)}
	if op.Scope.Project == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		t.Home = home
	}
	if err := validateOverlayDir(t); err != nil {
		return err
	}
	expected := t.Dir
	_, safe := targetPath(expected, filepath.Base(op.Target))
	if !safe {
		return fmt.Errorf("pending target name is unsafe")
	}
	return nil
}

type recoveryObject int

const (
	objectAbsent recoveryObject = iota
	objectOriginal
	objectCorrectLink
	objectOther
)

func recoverAdopt(libraryRoot string, op config.PendingOperation, ops FileOps) (bool, *Issue) {
	if err := validateAdoptPaths(op); err != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	lib, safe := libraryPath(libraryRoot, op.SkillID)
	if !safe {
		return false, recoveryIssue(IssueAdoptRecovery, op, "unsafe library id", nil)
	}
	if info, err := os.Stat(lib); err != nil || !info.IsDir() {
		return false, recoveryIssue(IssueAdoptRecovery, op, "library directory is missing", err)
	}
	target, err := classifyRecoveryPath(op.Target, libraryRoot, op.SkillID, op.Original)
	if err != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	backup, err := classifyRecoveryPath(op.Backup, libraryRoot, op.SkillID, op.Original)
	if err != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	temp, err := classifyRecoveryPath(op.Temp, libraryRoot, op.SkillID, op.Original)
	if err != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	deletePath := adoptDeletePath(op)
	deleteState, err := classifyRecoveryPath(deletePath, libraryRoot, op.SkillID, op.Original)
	if err != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	journalExists, journalErr := deleteJournalArtifactsExist(op)
	if journalErr != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, journalErr.Error(), journalErr)
	}
	if !journalExists && deleteState != objectAbsent && deleteState != objectOriginal {
		return false, recoveryIssue(IssueAdoptRecovery, op, "adopt delete tombstone does not match original", nil)
	}

	// Completed operation whose durable record was not cleared yet.
	if target == objectCorrectLink && backup == objectAbsent && temp == objectAbsent && deleteState == objectAbsent && !journalExists {
		return true, nil
	}
	if target == objectCorrectLink && backup == objectAbsent && temp == objectAbsent && journalExists {
		if err := finalizeAdopt(libraryRoot, lib, op, ops); err != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
		return true, nil
	}
	// Link installed; verify the recorded backup before deleting it.
	if target == objectCorrectLink && backup == objectOriginal && temp == objectAbsent {
		if err := finalizeAdopt(libraryRoot, lib, op, ops); err != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
		return true, nil
	}
	// Interrupted after the original was moved but before temp creation existed:
	// restore the original first, then use the normal retry path.
	if target == objectAbsent && backup == objectOriginal && temp == objectAbsent {
		if err := ops.MoveNoReplace(op.Backup, op.Target); err != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
		target, backup = objectOriginal, objectAbsent
	}
	// Interrupted exactly before installing the prepared link.
	if target == objectAbsent && backup == objectOriginal && temp == objectCorrectLink {
		if err := ops.MoveNoReplace(op.Temp, op.Target); err != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
		if err := finalizeAdopt(libraryRoot, lib, op, ops); err != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
		return true, nil
	}
	if target != objectOriginal || backup != objectAbsent || (temp != objectAbsent && temp != objectCorrectLink) {
		return false, recoveryIssue(IssueAdoptRecovery, op, "target/temp/backup do not match the recorded recovery state", nil)
	}
	if temp == objectAbsent {
		if err := ops.Symlink(lib, op.Temp); err != nil {
			removeExpectedTemp(op.Temp, libraryRoot, op.SkillID, ops)
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
		state, err := Inspect(op.Temp, libraryRoot)
		if err != nil || state.Kind != StateManagedLink || state.SkillID != op.SkillID {
			_ = ops.Remove(op.Temp)
			if err == nil {
				err = fmt.Errorf("created temp link did not match skill")
			}
			return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
		}
	}
	if err := ops.MoveNoReplace(op.Target, op.Backup); err != nil {
		removeExpectedTemp(op.Temp, libraryRoot, op.SkillID, ops)
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	match, verifyErr := matchesFingerprint(op.Backup, op.Original)
	if verifyErr != nil || !match {
		if verifyErr == nil {
			verifyErr = fmt.Errorf("original changed while moving to backup")
		}
		restoreErr := ops.MoveNoReplace(op.Backup, op.Target)
		if restoreErr != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, fmt.Sprintf("%v; changed object retained at %s; restore failed: %v", verifyErr, op.Backup, restoreErr), verifyErr)
		}
		removeExpectedTemp(op.Temp, libraryRoot, op.SkillID, ops)
		return false, recoveryIssue(IssueAdoptRecovery, op, verifyErr.Error(), verifyErr)
	}
	if err := ops.MoveNoReplace(op.Temp, op.Target); err != nil {
		rollbackErr := ops.MoveNoReplace(op.Backup, op.Target)
		if rollbackErr != nil {
			return false, recoveryIssue(IssueAdoptRecovery, op, fmt.Sprintf("install failed: %v; rollback failed: %v", err, rollbackErr), rollbackErr)
		}
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	if err := finalizeAdopt(libraryRoot, lib, op, ops); err != nil {
		return false, recoveryIssue(IssueAdoptRecovery, op, err.Error(), err)
	}
	return true, nil
}

func removeExpectedTemp(path, libraryRoot, id string, ops FileOps) {
	state, err := Inspect(path, libraryRoot)
	if err == nil && state.Kind == StateManagedLink && state.SkillID == id {
		_ = ops.Remove(path)
	}
}

func finalizeAdopt(libraryRoot, libraryPath string, op config.PendingOperation, ops FileOps) error {
	// Re-check all objects immediately before deleting the backup.
	state, err := Inspect(op.Target, libraryRoot)
	if err != nil || state.Kind != StateManagedLink || state.SkillID != op.SkillID {
		if err == nil {
			err = fmt.Errorf("installed target does not match skill")
		}
		return err
	}
	deletePath := adoptDeletePath(op)
	manifestPath := adoptManifestPath(op)
	manifestExists, err := lstatExists(manifestPath)
	if err != nil {
		return err
	}
	if manifestExists {
		return continueDeleteJournal(op.Backup, deletePath, manifestPath, op.Original, op.JournalHash, ops)
	}
	backupState, err := classifyRecoveryPath(op.Backup, libraryRoot, op.SkillID, op.Original)
	if err != nil {
		return err
	}
	deleteState, err := classifyRecoveryPath(deletePath, libraryRoot, op.SkillID, op.Original)
	if err != nil {
		return err
	}
	if !((backupState == objectOriginal && deleteState == objectAbsent) || (backupState == objectAbsent && deleteState == objectOriginal)) {
		return fmt.Errorf("backup/delete tombstone do not match recovery state")
	}
	source := op.Backup
	if deleteState == objectOriginal {
		source = deletePath
	}
	match, err := matchesFingerprint(source, op.Original)
	if err != nil || !match {
		if err == nil {
			err = fmt.Errorf("quarantined backup fingerprint changed")
		}
		return err
	}
	copyFingerprint, err := FingerprintPath(libraryPath)
	if err != nil {
		return fmt.Errorf("fingerprint library copy: %w", err)
	}
	if op.Original == nil || op.Original.Hash == "" || copyFingerprint.Hash != op.Original.Hash {
		return fmt.Errorf("library copy does not match original fingerprint")
	}
	if backupState == objectOriginal {
		if err := prepareDeleteJournal(op.Backup, deletePath, manifestPath, op.Original, op.JournalHash, ops); err != nil {
			return fmt.Errorf("prepare delete journal: %w", err)
		}
	}
	return resumeDeleteJournal(deletePath, manifestPath, op.JournalHash, ops)
}

func adoptDeletePath(op config.PendingOperation) string {
	return filepath.Join(filepath.Dir(op.Backup), ".aikit-adopt-delete-"+safeOperationComponent(op.ID))
}
func adoptManifestPath(op config.PendingOperation) string { return adoptDeletePath(op) + ".manifest" }

// DeleteJournalPaths exposes the deterministic recovery artifacts for status.
func DeleteJournalPaths(op config.PendingOperation) (deleteRoot, manifest string) {
	return adoptDeletePath(op), adoptManifestPath(op)
}

func deleteJournalArtifactsExist(op config.PendingOperation) (bool, error) {
	deleteRoot, manifest := DeleteJournalPaths(op)
	manifestExists, err := lstatExists(manifest)
	if err != nil {
		return false, err
	}
	rootExists, err := lstatExists(deleteRoot)
	if err != nil {
		return false, err
	}
	rootQExists, err := lstatExists(deleteEntryQuarantine(deleteRoot, "."))
	if err != nil {
		return false, err
	}
	if !manifestExists {
		return rootExists || rootQExists, nil
	}
	m, err := loadDeleteManifest(manifest, op.JournalHash)
	if err != nil {
		return true, err
	}
	for _, entry := range m.Entries {
		exists, statErr := lstatExists(deleteEntryQuarantine(deleteRoot, entry.Path))
		if statErr != nil {
			return true, statErr
		}
		if exists {
			return true, nil
		}
	}
	return true, nil
}
func safeOperationComponent(id string) string {
	if len(id) > 64 {
		sum := sha256.Sum256([]byte(id))
		return hex.EncodeToString(sum[:12])
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			sum := sha256.Sum256([]byte(id))
			return hex.EncodeToString(sum[:12])
		}
	}
	if id == "" {
		return "invalid"
	}
	return id
}

func validateAdoptPaths(op config.PendingOperation) error {
	if !filepath.IsAbs(op.Target) || filepath.Clean(op.Target) != op.Target {
		return fmt.Errorf("adopt target must be a clean absolute path")
	}
	parent := filepath.Dir(op.Target)
	if !filepath.IsAbs(op.Temp) || !filepath.IsAbs(op.Backup) || filepath.Clean(op.Temp) != op.Temp || filepath.Clean(op.Backup) != op.Backup || filepath.Dir(op.Temp) != parent || filepath.Dir(op.Backup) != parent {
		return fmt.Errorf("adopt temp and backup must be clean absolute siblings of target")
	}
	if !strings.HasPrefix(filepath.Base(op.Temp), ".aikit-adopt-temp-") || !strings.HasPrefix(filepath.Base(op.Backup), ".aikit-adopt-backup-") {
		return fmt.Errorf("adopt temp and backup do not use reserved names")
	}
	if decoded, err := hex.DecodeString(op.JournalHash); err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("adopt journal hash is invalid")
	}
	return nil
}

func classifyRecoveryPath(path, libraryRoot, id string, original *config.Fingerprint) (recoveryObject, error) {
	state, err := Inspect(path, libraryRoot)
	if err != nil {
		return objectOther, err
	}
	if state.Kind == StateAbsent {
		return objectAbsent, nil
	}
	if state.Kind == StateManagedLink && state.SkillID == id && !state.Broken {
		return objectCorrectLink, nil
	}
	match, err := matchesFingerprint(path, original)
	if err == nil && match {
		return objectOriginal, nil
	}
	if err != nil {
		return objectOther, err
	}
	return objectOther, nil
}

func recoveryIssue(kind IssueKind, op config.PendingOperation, message string, err error) *Issue {
	return &Issue{Kind: kind, Scope: op.Scope, Path: op.Target, SkillID: op.SkillID, Operation: op.ID, Message: message, Err: err}
}
