package library

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type CopySpec struct {
	Source      string
	Destination string
}

type PreparedCopy struct {
	Destination string
	Staging     string
	Backup      string
	existed     bool
	backedUp    bool
	installed   bool
	stageOwner  ownedTree
	backupOwner ownedTree
	destOwner   ownedTree
}

type Batch struct {
	Copies        []PreparedCopy
	rename        func(string, string) error
	SyncDirectory func(string) error
}

// PrepareBatch fully copies and validates all sources without changing any
// destination. Abort removes only staging trees whose recorded identity and
// fingerprint still match; concurrently replaced paths are preserved.
func PrepareBatch(specs []CopySpec, copier func(string, string) error, rename func(string, string) error) (*Batch, error) {
	if copier == nil {
		copier = AtomicCopy
	}
	if rename == nil {
		rename = moveNoReplace
	}
	batch := &Batch{rename: rename, SyncDirectory: syncDirectory}
	destinations := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
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
		staging, err := os.MkdirTemp(parent, ".aikit-batch-stage-")
		if err != nil {
			batch.Abort()
			return nil, err
		}
		if err := os.Remove(staging); err != nil {
			batch.Abort()
			return nil, err
		}
		batch.Copies = append(batch.Copies, PreparedCopy{Destination: spec.Destination, Staging: staging})
		if err := copier(spec.Source, staging); err != nil {
			if owner, inspectErr := inspectOwnedTree(staging); inspectErr == nil {
				batch.Copies[len(batch.Copies)-1].stageOwner = owner
			}
			batch.Abort()
			return nil, err
		}
		owner, err := inspectOwnedTree(staging)
		if err != nil {
			batch.Abort()
			return nil, err
		}
		batch.Copies[len(batch.Copies)-1].stageOwner = owner
	}
	return batch, nil
}

func (b *Batch) Abort() {
	for i := range b.Copies {
		copy := &b.Copies[i]
		if copy.stageOwner.identity != "" {
			_ = removeOwnedTree(copy.Staging, copy.stageOwner)
		}
	}
}

// Commit switches a prepared batch as one recoverable unit. Errors are rolled
// back before returning. Cross-platform directory replacement has an
// unavoidable crash window between renames; callers must run under config.lock
// and the application layer must record/recover any surviving .aikit-batch-*
// paths before starting a later mutation.
func (b *Batch) Commit() error {
	for i := range b.Copies {
		copy := &b.Copies[i]
		if _, err := os.Lstat(copy.Destination); err == nil {
			copy.existed = true
			owner, ownerErr := inspectOwnedTree(copy.Destination)
			if ownerErr != nil {
				return b.withRollback("record destination identity", ownerErr, i-1)
			}
			copy.destOwner = owner
			backup, tempErr := os.MkdirTemp(filepath.Dir(copy.Destination), ".aikit-batch-backup-"+filepath.Base(copy.Destination)+"-")
			if tempErr != nil {
				return b.withRollback("create backup", tempErr, i-1)
			}
			if err := os.Remove(backup); err != nil {
				return b.withRollback("prepare backup", err, i-1)
			}
			copy.Backup = backup
			if err := b.rename(copy.Destination, copy.Backup); err != nil {
				return b.withRollback("backup destination", err, i-1)
			}
			copy.backedUp = true
			owner, ownerErr = inspectOwnedTree(copy.Backup)
			if ownerErr != nil {
				return b.withRollback("record backup identity", ownerErr, i-1)
			}
			if owner != copy.destOwner {
				if restoreErr := b.rename(copy.Backup, copy.Destination); restoreErr == nil {
					copy.backedUp = false
				}
				return b.withRollback("backup destination", fmt.Errorf("destination changed before backup; moved object was preserved"), i-1)
			}
			copy.backupOwner = owner
		} else if !os.IsNotExist(err) {
			return b.withRollback("inspect destination", err, i-1)
		}
	}

	installed := -1
	for i := range b.Copies {
		copy := &b.Copies[i]
		if err := b.rename(copy.Staging, copy.Destination); err != nil {
			if rollbackErr := b.rollback(installed); rollbackErr != nil {
				return fmt.Errorf("install batch: %v; rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("install batch: %w", err)
		}
		copy.installed = true
		installed = i
		if err := verifyOwnedTree(copy.Destination, copy.stageOwner); err != nil {
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
			if rollbackErr := b.rollback(installed); rollbackErr != nil {
				return fmt.Errorf("sync batch: %v; rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("sync batch: %w", err)
		}
	}
	for i := range b.Copies {
		copy := &b.Copies[i]
		if copy.Backup != "" {
			if err := removeOwnedTree(copy.Backup, copy.backupOwner); err != nil {
				// The new batch is already installed and synced. A cleanup error
				// must not trigger a rollback after earlier backups may have been
				// deleted; retain the identifiable backup for startup recovery.
				continue
			}
			copy.backedUp = false
		}
	}
	return nil
}

func (b *Batch) withRollback(stage string, cause error, installed int) error {
	if rollbackErr := b.rollback(installed); rollbackErr != nil {
		return fmt.Errorf("%s: %v; rollback failed: %w", stage, cause, rollbackErr)
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
		if err := quarantineAndRemove(copy.Destination, copy.stageOwner, b.rename); err != nil {
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
			if err := removeOwnedTree(copy.Staging, copy.stageOwner); err != nil && !os.IsNotExist(err) && first == nil {
				first = err
			}
		}
	}
	return first
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
	if err := rename(path, quarantine); err != nil {
		return err
	}
	if err := removeOwnedTree(quarantine, expected); err != nil {
		return fmt.Errorf("quarantined owned tree retained at %q: %w", quarantine, err)
	}
	return nil
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
