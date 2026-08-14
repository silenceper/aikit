package link

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/silenceper/aikit/pkg/config"
)

func NewAdoptOperation(id string, scope config.Scope, target, skillID string) (config.PendingOperation, error) {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return config.PendingOperation{}, fmt.Errorf("adopt target must be a clean absolute path")
	}
	fingerprint, err := FingerprintPath(target)
	if err != nil {
		return config.PendingOperation{}, err
	}
	journalHash, err := DeleteJournalHash(target, fingerprint)
	if err != nil {
		return config.PendingOperation{}, err
	}
	if id == "" {
		id, err = operationID("adopt")
		if err != nil {
			return config.PendingOperation{}, err
		}
	}
	parent := filepath.Dir(target)
	for attempt := 0; attempt < 100; attempt++ {
		nonce, nonceErr := operationID("")
		if nonceErr != nil {
			return config.PendingOperation{}, nonceErr
		}
		op := config.PendingOperation{ID: id, Kind: config.OperationAdopt, Scope: scope, Target: target, SkillID: skillID, Temp: filepath.Join(parent, ".aikit-adopt-temp-"+nonce), Backup: filepath.Join(parent, ".aikit-adopt-backup-"+nonce), Original: &fingerprint, JournalHash: journalHash}
		available := true
		for _, path := range []string{op.Temp, op.Backup} {
			if _, statErr := os.Lstat(path); statErr == nil {
				available = false
				break
			} else if !os.IsNotExist(statErr) {
				return config.PendingOperation{}, statErr
			}
		}
		if available {
			return op, nil
		}
	}
	return config.PendingOperation{}, fmt.Errorf("unable to allocate adopt recovery paths")
}

func NewCleanupOperation(id string, scope config.Scope, target, skillID, reason string) (config.PendingOperation, error) {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return config.PendingOperation{}, fmt.Errorf("cleanup target must be a clean absolute path")
	}
	var err error
	if id == "" {
		id, err = operationID("cleanup")
		if err != nil {
			return config.PendingOperation{}, err
		}
	}
	operation := config.PendingOperation{ID: id, Kind: config.OperationCleanup, Scope: scope, Target: target, SkillID: skillID, Reason: reason}
	operation.Tombstone = filepath.Join(filepath.Dir(target), ".aikit-cleanup-"+id)
	if _, statErr := os.Lstat(operation.Tombstone); statErr == nil {
		return config.PendingOperation{}, fmt.Errorf("cleanup tombstone already exists")
	} else if !os.IsNotExist(statErr) {
		return config.PendingOperation{}, statErr
	}
	if _, statErr := os.Lstat(target); os.IsNotExist(statErr) {
		operation.ExpectedAbsent = true
	} else if statErr != nil {
		return config.PendingOperation{}, statErr
	} else {
		fingerprint, fingerprintErr := FingerprintPath(target)
		if fingerprintErr != nil {
			return config.PendingOperation{}, fingerprintErr
		}
		operation.ExpectedSkillID = skillID
		operation.Expected = &fingerprint
	}
	return operation, nil
}

func NewReconcileOperation(id string, scope config.Scope, target, skillID, libraryRoot, reason string) (config.PendingOperation, error) {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return config.PendingOperation{}, fmt.Errorf("reconcile target must be a clean absolute path")
	}
	state, err := Inspect(target, libraryRoot)
	if err != nil {
		return config.PendingOperation{}, err
	}
	operation := config.PendingOperation{Kind: config.OperationReconcile, Scope: scope, Target: target, SkillID: skillID, Reason: reason}
	switch state.Kind {
	case StateAbsent:
		operation.ExpectedAbsent = true
	case StateManagedLink:
		fingerprint, fingerprintErr := FingerprintPath(target)
		if fingerprintErr != nil {
			return config.PendingOperation{}, fingerprintErr
		}
		operation.ExpectedSkillID = state.SkillID
		operation.Expected = &fingerprint
	default:
		return config.PendingOperation{}, fmt.Errorf("reconcile target is not absent or aikit-managed")
	}
	if id == "" {
		id, err = operationID("reconcile")
		if err != nil {
			return config.PendingOperation{}, err
		}
	}
	operation.ID = id
	operation.Tombstone = filepath.Join(filepath.Dir(target), ".aikit-reconcile-"+id)
	if _, statErr := os.Lstat(operation.Tombstone); statErr == nil {
		return config.PendingOperation{}, fmt.Errorf("reconcile tombstone already exists")
	} else if !os.IsNotExist(statErr) {
		return config.PendingOperation{}, statErr
	}
	return operation, nil
}

func ExpectedManagedFingerprint(libraryRoot, skillID string) (*config.Fingerprint, error) {
	path, safe := libraryPath(libraryRoot, skillID)
	if !safe {
		return nil, fmt.Errorf("unsafe skill id %q", skillID)
	}
	content, err := FingerprintPath(path)
	if err != nil {
		return nil, err
	}
	return &config.Fingerprint{Kind: "symlink", Hash: content.Hash, LinkTarget: path}, nil
}

func operationID(prefix string) (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	value := hex.EncodeToString(b[:])
	if prefix != "" {
		value = prefix + "-" + value
	}
	return value, nil
}

// FingerprintPath records enough information to refuse recovery when an
// adopted object has been changed by another process.
func FingerprintPath(path string) (config.Fingerprint, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return config.Fingerprint{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return config.Fingerprint{}, err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return config.Fingerprint{}, fmt.Errorf("resolve original symlink: %w", err)
		}
		content, err := FingerprintPath(resolved)
		if err != nil {
			return config.Fingerprint{}, fmt.Errorf("fingerprint original symlink target: %w", err)
		}
		return config.Fingerprint{Kind: "symlink", Hash: content.Hash, LinkTarget: target}, nil
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return config.Fingerprint{}, fmt.Errorf("unsupported original object type %s", info.Mode())
	}
	h := sha256.New()
	if info.IsDir() {
		type record struct {
			path string
			info os.FileInfo
		}
		var records []record
		err = filepath.Walk(path, func(current string, item os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if current == path {
				return nil
			}
			if item.IsDir() && item.Name() == ".git" {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(path, current)
			if err != nil {
				return err
			}
			records = append(records, record{filepath.ToSlash(rel), item})
			return nil
		})
		if err != nil {
			return config.Fingerprint{}, err
		}
		sort.Slice(records, func(i, j int) bool { return records[i].path < records[j].path })
		for _, r := range records {
			if err := hashRecord(h, filepath.Join(path, filepath.FromSlash(r.path)), r.path, r.info); err != nil {
				return config.Fingerprint{}, err
			}
		}
		return config.Fingerprint{Kind: "directory", Hash: hex.EncodeToString(h.Sum(nil))}, nil
	}
	if err := hashRecord(h, path, ".", info); err != nil {
		return config.Fingerprint{}, err
	}
	return config.Fingerprint{Kind: "file", Hash: hex.EncodeToString(h.Sum(nil))}, nil
}

func hashRecord(w io.Writer, full, rel string, info os.FileInfo) error {
	writeField(w, []byte(rel))
	typeName := "file"
	if info.IsDir() {
		typeName = "directory"
	} else if info.Mode()&os.ModeSymlink != 0 {
		typeName = "symlink"
	}
	writeField(w, []byte(typeName))
	exec := byte(0)
	if info.Mode()&0o111 != 0 {
		exec = 1
	}
	writeField(w, []byte{exec})
	if info.IsDir() {
		writeField(w, nil)
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		raw, err := os.Readlink(full)
		if err != nil {
			return err
		}
		writeField(w, []byte(raw))
		return nil
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	writeField(w, b)
	return nil
}

func writeField(w io.Writer, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = w.Write(n[:])
	_, _ = w.Write(b)
}

func matchesFingerprint(path string, expected *config.Fingerprint) (bool, error) {
	if expected == nil {
		return false, nil
	}
	actual, err := FingerprintPath(path)
	if err != nil {
		return false, err
	}
	return actual.Kind == expected.Kind && actual.Hash == expected.Hash && actual.LinkTarget == expected.LinkTarget, nil
}
