package library

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/pkg/config"
)

const batchJournalVersion = 1

type RecoveryIssue struct {
	Journal string `json:"journal,omitempty"`
	Path    string `json:"path,omitempty"`
	Action  string `json:"action"`
	Detail  string `json:"detail"`
}

type journalOwner struct {
	Identity    string `json:"identity"`
	Fingerprint string `json:"fingerprint"`
}

type journalCopy struct {
	Operation   string        `json:"operation,omitempty"`
	Destination string        `json:"destination"`
	Staging     string        `json:"staging"`
	Backup      string        `json:"backup,omitempty"`
	Quarantine  string        `json:"quarantine,omitempty"`
	Existed     bool          `json:"existed"`
	Old         journalOwner  `json:"old,omitempty"`
	New         journalOwner  `json:"new"`
	OldSkill    *config.Skill `json:"old_skill,omitempty"`
	NewSkill    *config.Skill `json:"new_skill,omitempty"`
}

type batchJournal struct {
	Version   int           `json:"version"`
	ID        string        `json:"id"`
	Operation string        `json:"operation,omitempty"`
	Phase     string        `json:"phase"`
	Copies    []journalCopy `json:"copies"`
	Digest    string        `json:"digest"`
}

type journalWriteError struct {
	err     error
	visible bool
}

func (e *journalWriteError) Error() string { return e.err.Error() }
func (e *journalWriteError) Unwrap() error { return e.err }

func journalErrorVisible(err error) bool {
	var writeErr *journalWriteError
	return errors.As(err, &writeErr) && writeErr.visible
}

func newBatchID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func ownerForJournal(owner ownedTree) journalOwner {
	return journalOwner{Identity: owner.identity, Fingerprint: owner.fingerprint}
}

func ownerFromJournal(owner journalOwner) ownedTree {
	return ownedTree{identity: owner.Identity, fingerprint: owner.Fingerprint}
}

func (b *Batch) journal(phase string) batchJournal {
	copies := make([]journalCopy, len(b.Copies))
	for index, copy := range b.Copies {
		copies[index] = journalCopy{
			Operation:   copy.operation,
			Destination: copy.Destination,
			Staging:     copy.Staging,
			Backup:      copy.Backup,
			Quarantine:  copy.Quarantine,
			Existed:     copy.existed,
			Old:         ownerForJournal(copy.destOwner),
			New:         ownerForJournal(copy.stageOwner),
			OldSkill:    copy.oldSkill,
			NewSkill:    copy.newSkill,
		}
	}
	return batchJournal{Version: batchJournalVersion, ID: b.journalID, Phase: phase, Copies: copies}
}

func journalDigest(journal batchJournal) (string, error) {
	journal.Digest = ""
	payload, err := json.Marshal(journal)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func writeBatchJournal(path string, journal batchJournal) error {
	_, err := writeBatchJournalWithSync(path, journal, syncDirectory)
	return err
}

func writeBatchJournalWithSync(path string, journal batchJournal, syncDir func(string) error) (bool, error) {
	digest, err := journalDigest(journal)
	if err != nil {
		return false, err
	}
	journal.Digest = digest
	payload, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return false, err
	}
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".aikit-batch-journal-tmp-")
	if err != nil {
		return false, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return false, err
	}
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := replaceJournalFile(tempName, path); err != nil {
		return false, err
	}
	if syncDir == nil {
		syncDir = syncDirectory
	}
	if err := syncDir(directory); err != nil {
		return true, &journalWriteError{err: err, visible: true}
	}
	return true, nil
}

func readBatchJournal(root, path string) (batchJournal, ownedTree, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return batchJournal{}, ownedTree{}, err
	}
	root = resolvedRoot
	path, err = canonicalRecoveryPath(path)
	if err != nil {
		return batchJournal{}, ownedTree{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return batchJournal{}, ownedTree{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return batchJournal{}, ownedTree{}, fmt.Errorf("batch journal is not a regular no-follow file")
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return batchJournal{}, ownedTree{}, fmt.Errorf("batch journal escapes library root")
	}
	payload, err := secureReadRegular(root, relative, info)
	if err != nil {
		return batchJournal{}, ownedTree{}, err
	}
	var journal batchJournal
	if err := json.Unmarshal(payload, &journal); err != nil {
		return batchJournal{}, ownedTree{}, err
	}
	if journal.Version != batchJournalVersion || journal.ID == "" || len(journal.Copies) == 0 {
		return batchJournal{}, ownedTree{}, fmt.Errorf("invalid batch journal schema")
	}
	digest, err := journalDigest(journal)
	if err != nil || digest != journal.Digest {
		return batchJournal{}, ownedTree{}, fmt.Errorf("batch journal digest mismatch")
	}
	base := filepath.Base(path)
	if base != ".aikit-batch-"+journal.ID+".journal" && base != ".aikit-batch-journal-"+journal.ID+".tombstone" {
		return batchJournal{}, ownedTree{}, fmt.Errorf("batch journal id does not match filename")
	}
	identity, err := pathIdentity(path)
	if err != nil {
		return batchJournal{}, ownedTree{}, err
	}
	contentHash := sha256.Sum256(payload)
	return journal, ownedTree{identity: identity, fingerprint: hex.EncodeToString(contentHash[:])}, nil
}

func journalOwnerAt(root, path string) (ownedTree, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return ownedTree{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ownedTree{}, fmt.Errorf("batch journal is not a regular no-follow file")
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return ownedTree{}, err
	}
	payload, err := secureReadRegular(root, relative, info)
	if err != nil {
		return ownedTree{}, err
	}
	identity, err := pathIdentity(path)
	if err != nil {
		return ownedTree{}, err
	}
	sum := sha256.Sum256(payload)
	return ownedTree{identity: identity, fingerprint: hex.EncodeToString(sum[:])}, nil
}

func (b *Batch) persistJournal(phase string) error {
	if b.Journal == "" {
		return fmt.Errorf("batch journal path is empty")
	}
	visible, err := writeBatchJournalWithSync(b.Journal, b.journal(phase), b.SyncJournalDirectory)
	if visible {
		b.phase = phase
	}
	if err != nil {
		return err
	}
	return nil
}

func (b *Batch) clearJournal() error {
	if b.Journal == "" {
		return nil
	}
	if _, err := os.Lstat(b.Journal); err == nil {
		_, owner, inspectErr := readBatchJournal(filepath.Dir(b.Journal), b.Journal)
		if inspectErr != nil {
			return inspectErr
		}
		if err := removeOwnedJournal(filepath.Dir(b.Journal), b.Journal, b.journalID, owner); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if b.SyncJournalDirectory == nil {
		b.SyncJournalDirectory = syncDirectory
	}
	if err := b.SyncJournalDirectory(filepath.Dir(b.Journal)); err != nil {
		return err
	}
	b.phase = ""
	return nil
}

func (b *Batch) prepareMutationJournal() error {
	for index := range b.Copies {
		copy := &b.Copies[index]
		if _, err := os.Lstat(copy.Destination); err == nil {
			owner, err := inspectOwnedTree(copy.Destination)
			if err != nil {
				return err
			}
			if !copy.existed || owner != copy.destOwner {
				return fmt.Errorf("destination changed after prepare")
			}
		} else if os.IsNotExist(err) && copy.existed {
			return fmt.Errorf("destination disappeared after prepare")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return b.persistJournal("mutating")
}

func (s Service) RecoverBatches(ctx context.Context, ledger []config.Skill) ([]RecoveryIssue, error) {
	root, err := filepath.Abs(s.LibraryRoot)
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	var journals []string
	var journalTombstones []string
	var batchPaths []string
	err = filepath.WalkDir(resolvedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 && strings.HasPrefix(name, ".aikit-batch-") {
			batchPaths = append(batchPaths, path)
			return nil
		}
		if info.Mode().IsRegular() && strings.HasPrefix(name, ".aikit-batch-") && strings.HasSuffix(name, ".journal") {
			journals = append(journals, path)
		}
		if info.Mode().IsRegular() && strings.HasPrefix(name, ".aikit-batch-journal-") && strings.HasSuffix(name, ".tombstone") {
			journalTombstones = append(journalTombstones, path)
		}
		if entry.IsDir() && (strings.HasPrefix(name, ".aikit-batch-stage-") || strings.HasPrefix(name, ".aikit-batch-backup-") || strings.HasPrefix(name, ".aikit-batch-quarantine-")) {
			batchPaths = append(batchPaths, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	referenced := make(map[string]struct{})
	ledgerByID := make(map[string]config.Skill, len(ledger))
	for _, skill := range ledger {
		ledgerByID[skill.ID] = skill
	}
	var issues []RecoveryIssue
	for _, path := range journals {
		journal, journalOwner, err := readBatchJournal(resolvedRoot, path)
		if err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
			continue
		}
		if err := validateRecoveryJournal(resolvedRoot, journal); err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
			continue
		}
		for _, copy := range journal.Copies {
			for _, candidate := range []string{copy.Staging, copy.Backup, copy.Quarantine} {
				if candidate != "" {
					canonical, canonicalErr := canonicalRecoveryPath(candidate)
					if canonicalErr == nil {
						referenced[canonical] = struct{}{}
					}
				}
			}
		}
		journalIssues := recoverJournal(ctx, path, journal, journalOwner, ledgerByID)
		issues = append(issues, journalIssues...)
	}
	for _, path := range journalTombstones {
		journal, owner, err := readBatchJournal(resolvedRoot, path)
		if err != nil || validateRecoveryJournal(resolvedRoot, journal) != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: "invalid journal tombstone"})
			continue
		}
		if err := removeOwnedJournal(resolvedRoot, path, journal.ID, owner); err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
		}
	}
	for _, path := range batchPaths {
		canonical, _ := canonicalRecoveryPath(path)
		if _, ok := referenced[canonical]; !ok {
			issues = append(issues, RecoveryIssue{Path: path, Action: "preserved", Detail: "unknown batch path has no authenticated journal"})
		}
	}
	return issues, nil
}

func validateRecoveryJournal(root string, journal batchJournal) error {
	if journal.Operation == "remove" {
		return validateRemoveJournal(root, journal)
	}
	if journal.Operation != "" && journal.Operation != "copy" {
		return fmt.Errorf("invalid batch operation %q", journal.Operation)
	}
	if journal.Phase != "prepared" && journal.Phase != "mutating" && journal.Phase != "committed" {
		return fmt.Errorf("invalid batch phase %q", journal.Phase)
	}
	seen := make(map[string]struct{}, len(journal.Copies)*4)
	for index, copy := range journal.Copies {
		var canonicalDestination string
		for _, path := range []string{copy.Destination, copy.Staging, copy.Backup, copy.Quarantine} {
			if path == "" {
				continue
			}
			canonical, err := canonicalRecoveryPath(path)
			if err != nil || !isWithin(root, canonical) {
				return fmt.Errorf("journal path escapes library root")
			}
			if _, duplicate := seen[canonical]; duplicate {
				return fmt.Errorf("journal path is reused")
			}
			seen[canonical] = struct{}{}
			if path == copy.Destination {
				canonicalDestination = canonical
			}
			if _, statErr := os.Lstat(path); statErr == nil {
				resolved, resolveErr := filepath.EvalSymlinks(path)
				if resolveErr != nil || !isWithin(root, resolved) {
					return fmt.Errorf("existing journal path escapes library root")
				}
			}
		}
		relativeDestination, err := filepath.Rel(root, canonicalDestination)
		if err != nil || validateID(filepath.ToSlash(relativeDestination)) != nil || strings.Contains(filepath.ToSlash(relativeDestination), "/.aikit-batch-") || strings.HasPrefix(filepath.ToSlash(relativeDestination), ".aikit-batch-") {
			return fmt.Errorf("invalid destination record")
		}
		parent := filepath.Dir(copy.Destination)
		if copy.Staging != batchArtifactPath(parent, "stage", journal.ID, index) || copy.New.Identity == "" || copy.New.Fingerprint == "" {
			return fmt.Errorf("invalid staging record")
		}
		if copy.Backup != batchArtifactPath(parent, "backup", journal.ID, index) {
			return fmt.Errorf("invalid backup record")
		}
		if copy.Quarantine != batchArtifactPath(parent, "quarantine", journal.ID, index) {
			return fmt.Errorf("invalid quarantine record")
		}
		if journal.Phase != "prepared" && copy.Existed && (copy.Old.Identity == "" || copy.Old.Fingerprint == "") {
			return fmt.Errorf("invalid old owner record")
		}
		if copy.Operation != "" {
			if copy.Operation != "add" && copy.Operation != "update" || copy.NewSkill == nil || copy.NewSkill.ID != filepath.ToSlash(relativeDestination) {
				return fmt.Errorf("invalid ledger-driven copy metadata")
			}
			if copy.Operation == "add" && copy.OldSkill != nil || copy.Operation == "update" && (copy.OldSkill == nil || copy.OldSkill.ID != copy.NewSkill.ID) {
				return fmt.Errorf("invalid old skill metadata")
			}
		}
	}
	return nil
}

func validateRemoveJournal(root string, journal batchJournal) error {
	if journal.Phase != "prepared" && journal.Phase != "mutating" && journal.Phase != "committed" {
		return fmt.Errorf("invalid remove phase %q", journal.Phase)
	}
	if len(journal.Copies) == 0 {
		return fmt.Errorf("remove journal must contain copies")
	}
	for index, copy := range journal.Copies {
		destination, err := canonicalRecoveryPath(copy.Destination)
		if err != nil || !isWithin(root, destination) {
			return fmt.Errorf("remove destination escapes library root")
		}
		relative, err := filepath.Rel(root, destination)
		if err != nil || validateID(filepath.ToSlash(relative)) != nil || strings.HasPrefix(filepath.ToSlash(relative), ".aikit-batch-") {
			return fmt.Errorf("invalid remove destination")
		}
		wantQuarantine := batchArtifactPath(filepath.Dir(copy.Destination), "quarantine", journal.ID, index)
		if copy.Quarantine != wantQuarantine || copy.Staging != "" || copy.Backup != "" {
			return fmt.Errorf("invalid remove artifact record")
		}
		if copy.Existed && (copy.Old.Identity == "" || copy.Old.Fingerprint == "") || !copy.Existed && (copy.Old.Identity != "" || copy.Old.Fingerprint != "") {
			return fmt.Errorf("invalid remove owner record")
		}
		if copy.Operation != "remove" || copy.OldSkill == nil || copy.OldSkill.ID != filepath.ToSlash(relative) || copy.NewSkill != nil {
			return fmt.Errorf("invalid remove skill metadata")
		}
		quarantine, err := canonicalRecoveryPath(copy.Quarantine)
		if err != nil || !isWithin(root, quarantine) {
			return fmt.Errorf("remove quarantine escapes library root")
		}
	}
	return nil
}

func canonicalRecoveryPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return resolved, nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func recoverJournal(ctx context.Context, path string, journal batchJournal, journalOwner ownedTree, ledger map[string]config.Skill) []RecoveryIssue {
	if journal.Operation == "remove" {
		return recoverRemoveJournal(ctx, path, journal, journalOwner, ledger)
	}
	ledgerDriven := len(journal.Copies) > 0
	for _, copy := range journal.Copies {
		ledgerDriven = ledgerDriven && (copy.Operation == "add" || copy.Operation == "update") && copy.NewSkill != nil
	}
	if ledgerDriven {
		return recoverLedgerJournal(ctx, path, journal, journalOwner, ledger)
	}
	var issues []RecoveryIssue
	if journal.Phase == "committed" {
		for _, copy := range journal.Copies {
			if err := verifyOwnedTree(copy.Destination, ownerFromJournal(copy.New)); err != nil {
				return []RecoveryIssue{{Journal: path, Path: copy.Destination, Action: "preserved", Detail: "committed destination changed or missing; every backup was preserved"}}
			}
		}
	}
	for index := len(journal.Copies) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
		}
		copy := journal.Copies[index]
		newOwner, oldOwner := ownerFromJournal(copy.New), ownerFromJournal(copy.Old)
		if err := recoverBoundQuarantine(copy, journal.Phase, newOwner, oldOwner); err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Path: copy.Quarantine, Action: "preserved", Detail: err.Error()})
			continue
		}
		if journal.Phase == "mutating" {
			if err := recoverMutatingCopy(copy, newOwner, oldOwner); err != nil {
				issues = append(issues, RecoveryIssue{Journal: path, Path: copy.Destination, Action: "preserved", Detail: err.Error()})
			}
		}
		if err := removeIfOwned(copy.Staging, copy.Quarantine, newOwner); err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Path: copy.Staging, Action: "preserved", Detail: err.Error()})
		}
		if journal.Phase == "committed" {
			if err := removeIfOwned(copy.Backup, copy.Quarantine, oldOwner); err != nil {
				issues = append(issues, RecoveryIssue{Journal: path, Path: copy.Backup, Action: "preserved", Detail: err.Error()})
			}
		}
	}
	if len(issues) == 0 {
		if err := removeOwnedJournal(filepath.Dir(path), path, journal.ID, journalOwner); err != nil && !os.IsNotExist(err) {
			issues = append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
		}
	}
	return issues
}

func recoverLedgerJournal(ctx context.Context, path string, journal batchJournal, journalOwner ownedTree, ledger map[string]config.Skill) []RecoveryIssue {
	var issues []RecoveryIssue
	for index := len(journal.Copies) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
		}
		copy := journal.Copies[index]
		current, exists := ledger[copy.NewSkill.ID]
		desiredNew := exists && skillRecoveryMatch(current, *copy.NewSkill)
		desiredOld := false
		if copy.OldSkill == nil {
			desiredOld = !exists
		} else {
			desiredOld = exists && skillRecoveryMatch(current, *copy.OldSkill)
		}
		var err error
		switch {
		case desiredNew:
			err = rollForwardCopy(copy)
		case desiredOld:
			err = rollBackCopy(copy)
		default:
			err = fmt.Errorf("ledger matches neither old nor new skill metadata")
		}
		if err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Path: copy.Destination, Action: "preserved", Detail: err.Error()})
		}
	}
	if len(issues) == 0 {
		if err := removeOwnedJournal(filepath.Dir(path), path, journal.ID, journalOwner); err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Action: "preserved", Detail: err.Error()})
		}
	}
	return issues
}

func rollForwardCopy(copy journalCopy) error {
	newOwner, oldOwner := ownerFromJournal(copy.New), ownerFromJournal(copy.Old)
	if _, err := os.Lstat(copy.Destination); err == nil {
		if verifyOwnedTree(copy.Destination, newOwner) == nil {
			// Already installed.
		} else if copy.Existed && verifyOwnedTree(copy.Destination, oldOwner) == nil {
			if _, backupErr := os.Lstat(copy.Backup); backupErr == nil {
				return fmt.Errorf("old destination and backup both exist")
			} else if !os.IsNotExist(backupErr) {
				return backupErr
			}
			if err := moveNoReplace(copy.Destination, copy.Backup); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("destination matches neither old nor new owner")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(copy.Destination); os.IsNotExist(err) {
		source := copy.Staging
		if verifyOwnedTree(source, newOwner) != nil && verifyOwnedTree(copy.Quarantine, newOwner) == nil {
			source = copy.Quarantine
		}
		if err := verifyOwnedTree(source, newOwner); err != nil {
			return fmt.Errorf("new staging is unavailable: %w", err)
		}
		if err := moveNoReplace(source, copy.Destination); err != nil {
			return err
		}
	}
	if err := verifyOwnedTree(copy.Destination, newOwner); err != nil {
		return err
	}
	if err := removeIfOwned(copy.Staging, copy.Quarantine, newOwner); err != nil {
		return err
	}
	if copy.Existed {
		if err := removeIfOwned(copy.Backup, copy.Quarantine, oldOwner); err != nil {
			return err
		}
	}
	return cleanupBoundQuarantine(copy.Quarantine, newOwner, oldOwner)
}

func rollBackCopy(copy journalCopy) error {
	newOwner, oldOwner := ownerFromJournal(copy.New), ownerFromJournal(copy.Old)
	if _, err := os.Lstat(copy.Destination); err == nil {
		if verifyOwnedTree(copy.Destination, newOwner) == nil {
			newTombstone := copy.Quarantine
			if verifyOwnedTree(copy.Quarantine, oldOwner) == nil {
				newTombstone = copy.Staging
			}
			if err := removeOwnedArtifact(copy.Destination, newTombstone, newOwner, moveNoReplace); err != nil {
				return err
			}
		} else if !copy.Existed || verifyOwnedTree(copy.Destination, oldOwner) != nil {
			return fmt.Errorf("destination matches neither recoverable owner")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if copy.Existed {
		if _, err := os.Lstat(copy.Destination); os.IsNotExist(err) {
			source := copy.Backup
			if verifyOwnedTree(source, oldOwner) != nil && verifyOwnedTree(copy.Quarantine, oldOwner) == nil {
				source = copy.Quarantine
			}
			if err := verifyOwnedTree(source, oldOwner); err != nil {
				return fmt.Errorf("old tree is unavailable: %w", err)
			}
			if err := moveNoReplace(source, copy.Destination); err != nil {
				return err
			}
		}
		if err := verifyOwnedTree(copy.Destination, oldOwner); err != nil {
			return err
		}
	}
	if err := removeIfOwned(copy.Staging, copy.Quarantine, newOwner); err != nil {
		return err
	}
	return cleanupBoundQuarantine(copy.Quarantine, newOwner, oldOwner)
}

func cleanupBoundQuarantine(path string, owners ...ownedTree) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, owner := range owners {
		if owner.identity != "" && verifyOwnedTree(path, owner) == nil {
			return removeOwnedTree(path, owner)
		}
	}
	return fmt.Errorf("quarantine matches no journal owner")
}

func recoverRemoveJournal(ctx context.Context, path string, journal batchJournal, journalOwner ownedTree, ledger map[string]config.Skill) []RecoveryIssue {
	if err := ctx.Err(); err != nil {
		return []RecoveryIssue{{Journal: path, Action: "preserved", Detail: err.Error()}}
	}
	rollback := true
	for _, copy := range journal.Copies {
		current, exists := ledger[copy.OldSkill.ID]
		if exists && skillRecoveryMatch(current, *copy.OldSkill) {
			continue
		}
		if exists {
			return []RecoveryIssue{{Journal: path, Path: copy.Destination, Action: "preserved", Detail: "ledger skill does not match recorded remove metadata"}}
		}
		rollback = false
	}
	var issues []RecoveryIssue
	for _, copy := range journal.Copies {
		oldOwner := ownerFromJournal(copy.Old)
		var err error
		if rollback {
			err = rollbackRemoveCopy(copy, oldOwner)
		} else {
			err = rollForwardRemoveCopy(copy, oldOwner)
		}
		if err != nil {
			issues = append(issues, RecoveryIssue{Journal: path, Path: copy.Destination, Action: "preserved", Detail: err.Error()})
		}
	}
	if len(issues) > 0 {
		return issues
	}
	if err := removeOwnedJournal(filepath.Dir(path), path, journal.ID, journalOwner); err != nil {
		return []RecoveryIssue{{Journal: path, Action: "preserved", Detail: err.Error()}}
	}
	return nil
}

func rollForwardRemoveCopy(copy journalCopy, oldOwner ownedTree) error {
	if !copy.Existed {
		return nil
	}
	if _, err := os.Lstat(copy.Quarantine); err == nil {
		return removeOwnedTree(copy.Quarantine, oldOwner)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(copy.Destination); err == nil {
		return removeOwnedArtifact(copy.Destination, copy.Quarantine, oldOwner, moveNoReplace)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func rollbackRemoveCopy(copy journalCopy, oldOwner ownedTree) error {
	if !copy.Existed {
		return nil
	}
	if _, err := os.Lstat(copy.Destination); err == nil {
		return verifyOwnedTree(copy.Destination, oldOwner)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := verifyOwnedTree(copy.Quarantine, oldOwner); err != nil {
		return fmt.Errorf("old destination and authenticated quarantine are unavailable")
	}
	return moveNoReplace(copy.Quarantine, copy.Destination)
}

func skillRecoveryMatch(current, recorded config.Skill) bool {
	if current.ID != recorded.ID || current.Hash != recorded.Hash || current.Resolved != recorded.Resolved || current.Source != recorded.Source || current.SourcePath != recorded.SourcePath {
		return false
	}
	if current.Ref == nil || recorded.Ref == nil {
		return current.Ref == nil && recorded.Ref == nil
	}
	return current.Ref.Kind == recorded.Ref.Kind && current.Ref.Value == recorded.Ref.Value
}

func rollbackRemoveJournal(path string, journal batchJournal, journalOwner ownedTree, copy journalCopy, oldOwner ownedTree) []RecoveryIssue {
	if _, err := os.Lstat(copy.Destination); err == nil {
		if err := verifyOwnedTree(copy.Destination, oldOwner); err != nil {
			return []RecoveryIssue{{Journal: path, Path: copy.Destination, Action: "preserved", Detail: err.Error()}}
		}
	} else if os.IsNotExist(err) {
		if err := verifyOwnedTree(copy.Quarantine, oldOwner); err != nil {
			return []RecoveryIssue{{Journal: path, Path: copy.Quarantine, Action: "preserved", Detail: "old destination and authenticated quarantine are unavailable"}}
		}
		if err := moveNoReplace(copy.Quarantine, copy.Destination); err != nil {
			return []RecoveryIssue{{Journal: path, Path: copy.Destination, Action: "preserved", Detail: err.Error()}}
		}
	} else {
		return []RecoveryIssue{{Journal: path, Path: copy.Destination, Action: "preserved", Detail: err.Error()}}
	}
	if err := removeOwnedJournal(filepath.Dir(path), path, journal.ID, journalOwner); err != nil {
		return []RecoveryIssue{{Journal: path, Action: "preserved", Detail: err.Error()}}
	}
	return nil
}

func recoverBoundQuarantine(copy journalCopy, phase string, newOwner, oldOwner ownedTree) error {
	if _, err := os.Lstat(copy.Quarantine); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	actual, err := inspectOwnedTree(copy.Quarantine)
	if err != nil {
		return err
	}
	allowed := []ownedTree{newOwner}
	if phase == "committed" && copy.Existed {
		allowed = append(allowed, oldOwner)
	}
	for _, owner := range allowed {
		if owner.identity != "" && actual == owner {
			if err := removeOwnedTree(copy.Quarantine, owner); err != nil {
				return fmt.Errorf("retry quarantine cleanup: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("quarantine does not match the %s journal owner state", phase)
}

func verifyOwnedJournal(root, path string, expected ownedTree) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("journal changed type")
	}
	identity, err := pathIdentity(path)
	if err != nil || identity != expected.identity {
		return fmt.Errorf("journal identity changed")
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	payload, err := secureReadRegular(root, relative, info)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != expected.fingerprint {
		return fmt.Errorf("journal content changed")
	}
	return nil
}

func removeOwnedJournal(root, path, id string, expected ownedTree) error {
	if err := verifyOwnedJournal(root, path, expected); err != nil {
		return err
	}
	tombstone := filepath.Join(filepath.Dir(path), ".aikit-batch-journal-"+id+".tombstone")
	if path == tombstone {
		return os.Remove(path)
	}
	if err := moveNoReplace(path, tombstone); err != nil {
		return err
	}
	if err := verifyOwnedJournal(root, tombstone, expected); err != nil {
		return fmt.Errorf("moved journal changed; retained at %q: %w", tombstone, err)
	}
	return os.Remove(tombstone)
}

func recoverMutatingCopy(copy journalCopy, newOwner, oldOwner ownedTree) error {
	if _, err := os.Lstat(copy.Destination); err == nil {
		if verifyOwnedTree(copy.Destination, newOwner) == nil {
			quarantine := copy.Quarantine
			if quarantine == "" {
				return fmt.Errorf("missing authenticated quarantine path")
			}
			if err := removeOwnedArtifact(copy.Destination, quarantine, newOwner, moveNoReplace); err != nil {
				return err
			}
		} else if copy.Existed && verifyOwnedTree(copy.Destination, oldOwner) == nil {
			return nil
		} else {
			return fmt.Errorf("destination does not match journal ownership")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if !copy.Existed {
		return nil
	}
	if _, err := os.Lstat(copy.Backup); err == nil {
		if err := verifyOwnedTree(copy.Backup, oldOwner); err != nil {
			return fmt.Errorf("backup changed: %w", err)
		}
		return moveNoReplace(copy.Backup, copy.Destination)
	} else if os.IsNotExist(err) {
		if _, destinationErr := os.Lstat(copy.Destination); destinationErr == nil && verifyOwnedTree(copy.Destination, oldOwner) == nil {
			return nil
		}
		return fmt.Errorf("old destination and authenticated backup are both missing")
	} else {
		return err
	}
}

func removeIfOwned(path, tombstone string, owner ownedTree) error {
	if path == "" || owner.identity == "" {
		return nil
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return removeOwnedArtifact(path, tombstone, owner, moveNoReplace)
}
