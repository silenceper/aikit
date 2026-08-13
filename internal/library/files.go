package library

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/silenceper/aikit/pkg/config"
)

type CopySpec struct {
	Source      string
	Destination string
}

type PreparedCopy struct {
	Destination string
	Staging     string
	Backup      string
	Quarantine  string
	existed     bool
	backedUp    bool
	installed   bool
	stageOwner  ownedTree
	backupOwner ownedTree
	destOwner   ownedTree
	operation   string
	oldSkill    *config.Skill
	newSkill    *config.Skill
}

type Batch struct {
	Copies        []PreparedCopy
	Journal       string
	rename        func(string, string) error
	SyncDirectory func(string) error
	// SyncJournalDirectory is injectable so callers can distinguish a journal
	// replace that became visible from a failure before the replace.
	SyncJournalDirectory func(string) error
	journalID            string
	phase                string
	ledgerDriven         bool
}

// PrepareBatch fully copies and validates all sources without changing any
// destination. Abort removes only staging trees whose recorded identity and
// fingerprint still match; concurrently replaced paths are preserved.
func PrepareBatch(specs []CopySpec, copier func(string, string) error, rename func(string, string) error) (*Batch, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("batch has no copies")
	}
	if copier == nil {
		copier = AtomicCopy
	}
	if rename == nil {
		rename = moveNoReplace
	}
	journalID, err := newBatchID()
	if err != nil {
		return nil, err
	}
	batch := &Batch{rename: rename, SyncDirectory: syncDirectory, SyncJournalDirectory: syncDirectory, journalID: journalID}
	destinations := make(map[string]struct{}, len(specs))
	for index, spec := range specs {
		if _, duplicate := destinations[spec.Destination]; duplicate {
			batch.Abort()
			return nil, fmt.Errorf("duplicate batch destination %q", spec.Destination)
		}
		destinations[spec.Destination] = struct{}{}
		parent := filepath.Dir(spec.Destination)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			batch.Abort()
			return nil, err
		}
		staging := batchArtifactPath(parent, "stage", journalID, index)
		backup := batchArtifactPath(parent, "backup", journalID, index)
		quarantine := batchArtifactPath(parent, "quarantine", journalID, index)
		for _, artifact := range []string{staging, backup, quarantine} {
			if _, err := os.Lstat(artifact); err == nil {
				cleanupErr := batch.Abort()
				return nil, combineBatchError(fmt.Errorf("batch artifact %q already exists", artifact), cleanupErr)
			} else if !os.IsNotExist(err) {
				cleanupErr := batch.Abort()
				return nil, combineBatchError(err, cleanupErr)
			}
		}
		batch.Copies = append(batch.Copies, PreparedCopy{Destination: spec.Destination, Staging: staging, Backup: backup, Quarantine: quarantine})
		if err := copier(spec.Source, staging); err != nil {
			if owner, inspectErr := inspectOwnedTree(staging); inspectErr == nil {
				batch.Copies[len(batch.Copies)-1].stageOwner = owner
			}
			cleanupErr := batch.Abort()
			return nil, combineBatchError(err, cleanupErr)
		}
		owner, err := inspectOwnedTree(staging)
		if err != nil {
			cleanupErr := batch.Abort()
			return nil, combineBatchError(err, cleanupErr)
		}
		batch.Copies[len(batch.Copies)-1].stageOwner = owner
	}
	for index := range batch.Copies {
		copy := &batch.Copies[index]
		if _, err := os.Lstat(copy.Destination); err == nil {
			copy.existed = true
			owner, inspectErr := inspectOwnedTree(copy.Destination)
			if inspectErr != nil {
				cleanupErr := batch.Abort()
				return nil, combineBatchError(inspectErr, cleanupErr)
			}
			copy.destOwner = owner
		} else if !os.IsNotExist(err) {
			cleanupErr := batch.Abort()
			return nil, combineBatchError(err, cleanupErr)
		}
	}
	batch.Journal = filepath.Join(filepath.Dir(batch.Copies[0].Destination), ".aikit-batch-"+journalID+".journal")
	if err := batch.persistJournal("prepared"); err != nil {
		cleanupErr := batch.Abort()
		return nil, combineBatchError(err, cleanupErr)
	}
	return batch, nil
}

func (b *Batch) Abort() error {
	if b.phase != "" && b.phase != "prepared" {
		return fmt.Errorf("batch phase %q requires RecoverBatches", b.phase)
	}
	var first error
	for i := range b.Copies {
		copy := &b.Copies[i]
		if copy.stageOwner.identity != "" {
			if err := removeOwnedArtifact(copy.Staging, copy.Quarantine, copy.stageOwner, b.rename); err != nil && !os.IsNotExist(err) && first == nil {
				first = err
			}
		}
	}
	if first == nil {
		if err := b.clearJournal(); err != nil {
			first = err
		}
	}
	return first
}

// Commit switches a prepared batch as one recoverable unit. Errors are rolled
// back before returning. Cross-platform directory replacement has an
// unavoidable crash window between renames; callers must run under config.lock
// and the application layer must record/recover any surviving .aikit-batch-*
// paths before starting a later mutation.
func (b *Batch) Commit() error {
	if err := b.prepareMutationJournal(); err != nil {
		if b.ledgerDriven {
			return fmt.Errorf("prepare mutation journal; run RecoverBatches: %w", err)
		}
		if journalErrorVisible(err) {
			return fmt.Errorf("prepare mutation journal became visible; run RecoverBatches: %w", err)
		}
		return b.withRollback("prepare mutation journal", err, -1)
	}
	for i := range b.Copies {
		copy := &b.Copies[i]
		if copy.existed {
			if err := b.rename(copy.Destination, copy.Backup); err != nil {
				if b.ledgerDriven {
					return fmt.Errorf("backup destination; run RecoverBatches: %w", err)
				}
				return b.withRollback("backup destination", err, i-1)
			}
			copy.backedUp = true
			owner, ownerErr := inspectOwnedTree(copy.Backup)
			if ownerErr != nil {
				if b.ledgerDriven {
					return fmt.Errorf("record backup identity; run RecoverBatches: %w", ownerErr)
				}
				return b.withRollback("record backup identity", ownerErr, i-1)
			}
			if owner != copy.destOwner {
				if b.ledgerDriven {
					return fmt.Errorf("backup destination changed; run RecoverBatches")
				}
				if restoreErr := b.rename(copy.Backup, copy.Destination); restoreErr == nil {
					copy.backedUp = false
				}
				return b.withRollback("backup destination", fmt.Errorf("destination changed before backup; moved object was preserved"), i-1)
			}
			copy.backupOwner = owner
		}
	}

	installed := -1
	for i := range b.Copies {
		copy := &b.Copies[i]
		if err := b.rename(copy.Staging, copy.Destination); err != nil {
			if b.ledgerDriven {
				return fmt.Errorf("install batch; run RecoverBatches: %w", err)
			}
			if rollbackErr := b.rollback(installed); rollbackErr != nil {
				return fmt.Errorf("install batch: %v; rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("install batch: %w", err)
		}
		copy.installed = true
		installed = i
		if err := verifyOwnedTree(copy.Destination, copy.stageOwner); err != nil {
			if b.ledgerDriven {
				return fmt.Errorf("verify installed batch; run RecoverBatches: %w", err)
			}
			if restoreErr := b.rename(copy.Destination, copy.Staging); restoreErr == nil {
				copy.installed = false
			}
			if rollbackErr := b.rollback(installed); rollbackErr != nil {
				return fmt.Errorf("verify installed batch: %v; rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("verify installed batch: %w", err)
		}
	}
	parents := make(map[string]struct{})
	for _, copy := range b.Copies {
		parents[filepath.Dir(copy.Destination)] = struct{}{}
	}
	for parent := range parents {
		if err := b.SyncDirectory(parent); err != nil {
			if b.ledgerDriven {
				return fmt.Errorf("sync batch; run RecoverBatches: %w", err)
			}
			if rollbackErr := b.rollback(installed); rollbackErr != nil {
				return fmt.Errorf("sync batch: %v; rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("sync batch: %w", err)
		}
	}
	if err := b.persistJournal("committed"); err != nil {
		if b.ledgerDriven {
			return fmt.Errorf("commit batch journal; run RecoverBatches: %w", err)
		}
		if journalErrorVisible(err) {
			return fmt.Errorf("committed journal became visible; run RecoverBatches: %w", err)
		}
		return b.withRollback("commit batch journal", err, installed)
	}
	if b.ledgerDriven {
		// The durable ledger is the commit decision. Leave the committed journal
		// and authenticated backups for RecoverBatches to finalize in that
		// direction; this also makes a post-return crash recoverable.
		return nil
	}
	cleanupComplete := true
	for i := range b.Copies {
		copy := &b.Copies[i]
		if copy.existed {
			if err := removeOwnedArtifact(copy.Backup, copy.Quarantine, copy.backupOwner, b.rename); err != nil {
				// The new batch is already installed and synced. A cleanup error
				// must not trigger a rollback after earlier backups may have been
				// deleted; retain the identifiable backup for startup recovery.
				cleanupComplete = false
				continue
			}
			copy.backedUp = false
		}
	}
	if cleanupComplete {
		if err := b.clearJournal(); err != nil {
			return fmt.Errorf("clear batch journal: %w", err)
		}
	}
	return nil
}

func (b *Batch) withRollback(stage string, cause error, installed int) error {
	if rollbackErr := b.rollback(installed); rollbackErr != nil {
		return fmt.Errorf("%s: %v; rollback failed: %w", stage, cause, rollbackErr)
	}
	if err := b.clearJournal(); err != nil {
		return fmt.Errorf("%s: %v; rollback succeeded but journal cleanup failed: %w", stage, cause, err)
	}
	return fmt.Errorf("%s: %w", stage, cause)
}

func (b *Batch) rollback(installed int) error {
	var first error
	for i := installed; i >= 0; i-- {
		copy := &b.Copies[i]
		if !copy.installed {
			continue
		}
		if err := quarantineAndRemoveAt(copy.Destination, copy.Quarantine, copy.stageOwner, b.rename); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		copy.installed = false
	}
	for i := len(b.Copies) - 1; i >= 0; i-- {
		copy := &b.Copies[i]
		if copy.existed && copy.backedUp && copy.Backup != "" {
			if err := verifyOwnedTree(copy.Backup, copy.backupOwner); err != nil {
				if first == nil {
					first = fmt.Errorf("backup %q changed and was retained: %w", copy.Backup, err)
				}
				continue
			}
			if _, err := os.Lstat(copy.Destination); err == nil {
				if first == nil {
					first = fmt.Errorf("destination %q appeared during rollback; backup retained at %q", copy.Destination, copy.Backup)
				}
			} else if !os.IsNotExist(err) {
				if first == nil {
					first = err
				}
			} else if err := b.rename(copy.Backup, copy.Destination); err != nil {
				if first == nil {
					first = err
				}
			} else {
				copy.backedUp = false
			}
		}
		if copy.stageOwner.identity != "" {
			if err := removeOwnedArtifact(copy.Staging, copy.Quarantine, copy.stageOwner, b.rename); err != nil && !os.IsNotExist(err) && first == nil {
				first = err
			}
		}
	}
	return first
}

func combineBatchError(cause, cleanup error) error {
	if cleanup == nil {
		return cause
	}
	return fmt.Errorf("%v; cleanup requires RecoverBatches: %w", cause, cleanup)
}

func batchArtifactPath(parent, kind, id string, index int) string {
	return filepath.Join(parent, fmt.Sprintf(".aikit-batch-%s-%s-%d", kind, id, index))
}

// AtomicCopy validates and installs a skill into a previously absent path.
// Replacements must use PrepareBatch so failures can roll back the old tree.
func AtomicCopy(source, destination string) error {
	candidates, err := Discover(source)
	if err != nil {
		return err
	}
	if len(candidates) != 1 || candidates[0].RelativePath != "." {
		return fmt.Errorf("source must be a single skill root")
	}
	expectedHash := candidates[0].Hash

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".aikit-copy-")
	if err != nil {
		return err
	}
	var stagingOwner ownedTree
	defer func() {
		if stagingOwner.identity != "" {
			_ = removeOwnedTree(staging, stagingOwner)
		}
	}()
	if err := copyTree(source, staging); err != nil {
		return err
	}
	stagedCandidates, err := Discover(staging)
	if err != nil {
		return fmt.Errorf("validate staged skill: %w", err)
	}
	if len(stagedCandidates) != 1 || stagedCandidates[0].Hash != expectedHash {
		return fmt.Errorf("source changed while copying")
	}
	currentHash, err := HashSkill(source)
	if err != nil || currentHash != expectedHash {
		return fmt.Errorf("source changed while copying")
	}
	if err := syncTreeDirectories(staging); err != nil {
		return err
	}
	stagingOwner, err = inspectOwnedTree(staging)
	if err != nil {
		return err
	}

	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("destination %q already exists", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := moveNoReplace(staging, destination); err != nil {
		return err
	}
	if err := syncDirectory(parent); err != nil {
		_ = quarantineAndRemove(destination, stagingOwner, moveNoReplace)
		_ = syncDirectory(parent)
		return err
	}
	return nil
}

type ownedTree struct {
	identity    string
	fingerprint string
}

func inspectOwnedTree(path string) (ownedTree, error) {
	identity, err := pathIdentity(path)
	if err != nil {
		return ownedTree{}, err
	}
	fingerprint, err := fingerprintTree(path)
	if err != nil {
		return ownedTree{}, err
	}
	return ownedTree{identity: identity, fingerprint: fingerprint}, nil
}

func verifyOwnedTree(path string, expected ownedTree) error {
	actual, err := inspectOwnedTree(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("owned tree %q changed; preserving it", path)
	}
	return nil
}

func fingerprintTree(root string) (string, error) {
	var records []string
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		identity, err := pathIdentity(current)
		if err != nil {
			return err
		}
		record := filepath.ToSlash(rel) + "\x00" + identity + "\x00" + info.Mode().String()
		switch {
		case info.Mode().IsRegular():
			content, err := secureReadRegular(root, rel, info)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(content)
			record += "\x00" + hex.EncodeToString(sum[:])
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(current)
			if err != nil {
				return err
			}
			record += "\x00" + target
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(records)
	hash := sha256.New()
	for _, record := range records {
		_, _ = io.WriteString(hash, record)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func removeOwnedTree(path string, expected ownedTree) error {
	if err := verifyOwnedTree(path, expected); err != nil {
		return err
	}
	type removal struct {
		path     string
		identity string
	}
	var paths []removal
	if err := filepath.WalkDir(path, func(current string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		identity, err := pathIdentity(current)
		if err != nil {
			return err
		}
		paths = append(paths, removal{path: current, identity: identity})
		return nil
	}); err != nil {
		return err
	}
	for i := len(paths) - 1; i >= 0; i-- {
		identity, err := pathIdentity(paths[i].path)
		if err != nil || identity != paths[i].identity {
			return fmt.Errorf("owned tree entry %q changed; preserving it", paths[i].path)
		}
		if err := os.Remove(paths[i].path); err != nil {
			return err
		}
	}
	return nil
}

func quarantineAndRemove(path string, expected ownedTree, rename func(string, string) error) error {
	if err := verifyOwnedTree(path, expected); err != nil {
		return err
	}
	quarantine, err := os.MkdirTemp(filepath.Dir(path), ".aikit-batch-quarantine-")
	if err != nil {
		return err
	}
	if err := os.Remove(quarantine); err != nil {
		return err
	}
	return quarantineAndRemoveAt(path, quarantine, expected, rename)
}

func quarantineAndRemoveAt(path, quarantine string, expected ownedTree, rename func(string, string) error) error {
	if err := verifyOwnedTree(path, expected); err != nil {
		return err
	}
	if quarantine == "" {
		return fmt.Errorf("quarantine path is empty")
	}
	if err := rename(path, quarantine); err != nil {
		return err
	}
	if err := removeOwnedTree(quarantine, expected); err != nil {
		return fmt.Errorf("quarantined owned tree retained at %q: %w", quarantine, err)
	}
	return nil
}

// removeOwnedArtifact first moves the exact recorded object to a batch-bound
// tombstone with no replacement. A concurrently installed object at the old
// pathname is therefore never deleted. The post-move identity check also
// ensures the moved object is the one recorded by the journal.
func removeOwnedArtifact(path, tombstone string, expected ownedTree, rename func(string, string) error) error {
	if path == "" || expected.identity == "" {
		return nil
	}
	if err := verifyOwnedTree(path, expected); err != nil {
		return err
	}
	if tombstone == "" || tombstone == path {
		return fmt.Errorf("invalid artifact tombstone path")
	}
	if err := rename(path, tombstone); err != nil {
		return err
	}
	if err := verifyOwnedTree(tombstone, expected); err != nil {
		return fmt.Errorf("moved artifact identity changed; retained at %q: %w", tombstone, err)
	}
	return removeOwnedTree(tombstone, expected)
}

func copyTree(source, destination string) error {
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	return filepath.WalkDir(resolvedSource, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(resolvedSource, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		target := filepath.Join(destination, rel)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			return os.Mkdir(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return err
			}
			if !isWithin(resolvedSource, resolved) {
				return fmt.Errorf("file %q escapes skill root", rel)
			}
			content, err := secureReadRegular(resolvedSource, rel, info)
			if err != nil {
				return err
			}
			return writeRegularFile(content, target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return err
			}
			if !isWithin(resolvedSource, resolved) {
				return fmt.Errorf("symlink %q escapes skill root", rel)
			}
			linkTarget, err := os.Readlink(current)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		default:
			return fmt.Errorf("unsupported special file %q", rel)
		}
	})
}

func writeRegularFile(content []byte, destination string, mode os.FileMode) error {
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := out.Write(content); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func syncTreeDirectories(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, current)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := syncDirectory(directories[i]); err != nil {
			return err
		}
	}
	return nil
}
