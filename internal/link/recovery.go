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
}

func defaultFileOps() FileOps {
	return FileOps{Symlink: os.Symlink, MoveNoReplace: moveNoReplace, Remove: os.Remove}
}

func Recover(libraryRoot string, operations []config.PendingOperation, selector Selector, dryRun bool) Result {
	return RecoverWithOps(libraryRoot, operations, selector, dryRun, defaultFileOps())
}

func RecoverWithOps(libraryRoot string, operations []config.PendingOperation, selector Selector, dryRun bool, ops FileOps) Result {
	result := Result{}
	for _, op := range operations {
		if !selector.Matches(op.Scope) {
			continue
		}
		action := Action{Kind: ActionRecover, Scope: op.Scope, Path: op.Target, SkillID: op.SkillID, Operation: op.ID, Library: libraryRoot}
		result.Actions = append(result.Actions, action)
		if dryRun {
			continue
		}
		if err := validateOperationScopeTarget(op); err != nil {
			result.Issues = append(result.Issues, *recoveryIssue(IssueUnsafePath, op, err.Error(), err))
			continue
		}
		var complete bool
		var issue *Issue
		switch op.Kind {
		case config.OperationCleanup:
			complete, issue = recoverCleanup(libraryRoot, op, ops)
		case config.OperationAdopt:
			complete, issue = recoverAdopt(libraryRoot, op, ops)
		default:
			i := Issue{Kind: IssueIO, Scope: op.Scope, Path: op.Target, Operation: op.ID, Message: "unknown pending operation"}
			issue = &i
		}
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
			continue
		}
		if complete {
			result.Completed = append(result.Completed, op.ID)
			result.Applied = append(result.Applied, action)
		}
	}
	return result
}

func recoverCleanup(libraryRoot string, op config.PendingOperation, ops FileOps) (bool, *Issue) {
	state, err := Inspect(op.Target, libraryRoot)
	if err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	if state.Kind == StateAbsent {
		return true, nil
	}
	if state.Kind != StateManagedLink || state.SkillID != op.SkillID {
		return false, recoveryIssue(IssuePendingCleanup, op, "target no longer matches the recorded aikit link", nil)
	}
	tombstone, err := quarantineManaged(op.Target, op.SkillID, libraryRoot, ops, ".aikit-cleanup-delete-")
	if err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	if err := ops.Remove(tombstone); err != nil {
		return false, recoveryIssue(IssuePendingCleanup, op, err.Error(), err)
	}
	return true, nil
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
