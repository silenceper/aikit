package link

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silenceper/aikit/pkg/config"
)

type deleteManifest struct {
	Version  int                   `json:"version"`
	Original config.Fingerprint    `json:"original"`
	Entries  []deleteManifestEntry `json:"entries"`
}
type deleteManifestEntry struct{ Path, Kind, Hash, LinkTarget string }

// DeleteJournalHash returns the digest of the canonical deletion manifest that
// must be persisted with an adopt operation before filesystem mutation.
func DeleteJournalHash(root string, original config.Fingerprint) (string, error) {
	m, err := buildDeleteManifest(root, original)
	if err != nil {
		return "", err
	}
	return deleteManifestHash(m)
}

func prepareDeleteJournal(backup, deleteRoot, manifestPath string, expected *config.Fingerprint, journalHash string, ops FileOps) error {
	match, err := matchesFingerprint(backup, expected)
	if err != nil || !match {
		if err == nil {
			err = fmt.Errorf("backup fingerprint changed")
		}
		return err
	}
	m, err := buildDeleteManifest(backup, *expected)
	if err != nil {
		return err
	}
	if err = verifyDeleteManifestHash(m, journalHash); err != nil {
		return err
	}
	if err = writeManifestAtomic(manifestPath, m); err != nil {
		return err
	}
	if err = ops.MoveNoReplace(backup, deleteRoot); err != nil {
		return err
	}
	return nil
}

func buildDeleteManifest(root string, original config.Fingerprint) (deleteManifest, error) {
	m := deleteManifest{Version: 1, Original: original}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		} else if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		}
		e := deleteManifestEntry{Path: filepath.ToSlash(rel), Kind: kind}
		if info.IsDir() {
			e.Hash = directoryManifestHash()
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			e.Hash = symlinkManifestHash(target)
			e.LinkTarget = target
		} else {
			fp, err := FingerprintPath(path)
			if err != nil {
				return err
			}
			e.Hash = fp.Hash
			e.LinkTarget = fp.LinkTarget
		}
		m.Entries = append(m.Entries, e)
		return nil
	})
	if err != nil {
		return deleteManifest{}, err
	}
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	return m, nil
}

func deleteManifestHash(m deleteManifest) (string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func verifyDeleteManifestHash(m deleteManifest, expected string) error {
	actual, err := deleteManifestHash(m)
	if err != nil {
		return err
	}
	if expected == "" || actual != strings.ToLower(expected) {
		return fmt.Errorf("delete manifest digest mismatch")
	}
	return nil
}

func writeManifestAtomic(path string, m deleteManifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".aikit-delete-manifest-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = moveNoReplace(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func loadDeleteManifest(path, expectedHash string) (deleteManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return deleteManifest{}, err
	}
	var m deleteManifest
	if err = json.Unmarshal(b, &m); err != nil {
		return deleteManifest{}, fmt.Errorf("decode delete manifest: %w", err)
	}
	if m.Version != 1 || len(m.Entries) == 0 {
		return deleteManifest{}, fmt.Errorf("invalid delete manifest")
	}
	seen := map[string]bool{}
	for _, e := range m.Entries {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(e.Path)))
		if e.Path == "" || strings.Contains(e.Path, "\\") || clean != e.Path || seen[e.Path] || filepath.IsAbs(filepath.FromSlash(e.Path)) || e.Path == ".." || strings.HasPrefix(e.Path, "../") {
			return deleteManifest{}, fmt.Errorf("unsafe delete manifest path")
		}
		seen[e.Path] = true
		if e.Hash == "" {
			return deleteManifest{}, fmt.Errorf("delete manifest entry lacks hash")
		}
		switch e.Kind {
		case "file", "symlink", "directory":
		default:
			return deleteManifest{}, fmt.Errorf("invalid delete manifest kind")
		}
	}
	if !seen["."] {
		return deleteManifest{}, fmt.Errorf("delete manifest lacks root")
	}
	if err := verifyDeleteManifestHash(m, expectedHash); err != nil {
		return deleteManifest{}, err
	}
	return m, nil
}

func continueDeleteJournal(backup, deleteRoot, manifestPath string, expected *config.Fingerprint, journalHash string, ops FileOps) error {
	m, err := loadDeleteManifest(manifestPath, journalHash)
	if err != nil {
		return err
	}
	if expected == nil || m.Original != *expected {
		return fmt.Errorf("delete manifest original fingerprint mismatch")
	}
	backupExists, err := lstatExists(backup)
	if err != nil {
		return err
	}
	rootExists, err := lstatExists(deleteRoot)
	if err != nil {
		return err
	}
	rootQExists, err := lstatExists(deleteEntryQuarantine(deleteRoot, "."))
	if err != nil {
		return err
	}
	if backupExists {
		if rootExists || rootQExists {
			return fmt.Errorf("backup and delete journal objects both exist")
		}
		match, err := matchesFingerprint(backup, expected)
		if err != nil || !match {
			if err == nil {
				err = fmt.Errorf("backup changed after manifest write")
			}
			return err
		}
		if err = ops.MoveNoReplace(backup, deleteRoot); err != nil {
			return err
		}
	}
	return resumeDeleteJournal(deleteRoot, manifestPath, journalHash, ops)
}

func resumeDeleteJournal(deleteRoot, manifestPath, journalHash string, ops FileOps) error {
	m, err := loadDeleteManifest(manifestPath, journalHash)
	if err != nil {
		return err
	}
	entries := append([]deleteManifestEntry(nil), m.Entries...)
	entryKinds := make(map[string]string, len(entries))
	for _, entry := range entries {
		entryKinds[entry.Path] = entry.Kind
	}
	sort.Slice(entries, func(i, j int) bool {
		di := pathDepth(entries[i].Path)
		dj := pathDepth(entries[j].Path)
		if di == dj {
			return entries[i].Kind != "directory" && entries[j].Kind == "directory"
		}
		return di > dj
	})
	for _, e := range entries {
		source := deleteRoot
		if e.Path != "." {
			source = filepath.Join(deleteRoot, filepath.FromSlash(e.Path))
		}
		q := deleteEntryQuarantine(deleteRoot, e.Path)
		sourceExists, statErr := lstatExists(source)
		if statErr != nil {
			return statErr
		}
		qExists, statErr := lstatExists(q)
		if statErr != nil {
			return statErr
		}
		if sourceExists && qExists {
			return fmt.Errorf("delete source and quarantine both exist for %s", e.Path)
		}
		if !sourceExists && !qExists {
			continue
		}
		if sourceExists {
			if err = validateDeleteSourceAncestors(deleteRoot, e.Path, entryKinds); err != nil {
				return err
			}
		}
		current := source
		if qExists {
			current = q
		}
		if err = validateManifestEntry(current, e); err != nil {
			return err
		}
		if sourceExists {
			if err = ops.MoveNoReplace(source, q); err != nil {
				return err
			}
			if err = validateManifestEntry(q, e); err != nil {
				return err
			}
		}
		if err = ops.Remove(q); err != nil {
			return fmt.Errorf("delete journal entry %s retained at %s: %w", e.Path, q, err)
		}
	}
	if entriesLeft, err := os.ReadDir(deleteRoot); err == nil && len(entriesLeft) > 0 {
		return fmt.Errorf("unknown entries remain in delete root")
	} else if err == nil {
		return fmt.Errorf("delete root still exists outside manifest")
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Remove(manifestPath)
}

func validateDeleteSourceAncestors(root, rel string, entryKinds map[string]string) error {
	if rel == "." {
		return nil
	}
	current := root
	if err := validateDeleteDirectoryAncestor(current, ".", entryKinds); err != nil {
		return err
	}
	parts := strings.Split(rel, "/")
	for i := 0; i < len(parts)-1; i++ {
		current = filepath.Join(current, filepath.FromSlash(parts[i]))
		manifestPath := strings.Join(parts[:i+1], "/")
		if err := validateDeleteDirectoryAncestor(current, manifestPath, entryKinds); err != nil {
			return err
		}
	}
	return nil
}

func validateDeleteDirectoryAncestor(path, manifestPath string, entryKinds map[string]string) error {
	if entryKinds[manifestPath] != "directory" {
		return fmt.Errorf("delete entry %s lacks a directory ancestor in manifest", manifestPath)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("delete entry %s directory ancestor changed", manifestPath)
	}
	return nil
}

func validateManifestEntry(path string, e deleteManifestEntry) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	kind := "file"
	if info.IsDir() {
		kind = "directory"
	} else if info.Mode()&os.ModeSymlink != 0 {
		kind = "symlink"
	}
	if kind != e.Kind {
		return fmt.Errorf("delete entry %s kind changed", e.Path)
	}
	if kind == "directory" && e.Hash != directoryManifestHash() {
		return fmt.Errorf("delete entry %s directory hash invalid", e.Path)
	}
	if kind == "symlink" {
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if target != e.LinkTarget || symlinkManifestHash(target) != e.Hash {
			return fmt.Errorf("delete entry %s symlink changed", e.Path)
		}
	} else if kind != "directory" {
		fp, err := FingerprintPath(path)
		if err != nil {
			return err
		}
		if fp.Hash != e.Hash || fp.LinkTarget != e.LinkTarget {
			return fmt.Errorf("delete entry %s fingerprint changed", e.Path)
		}
	}
	return nil
}
func symlinkManifestHash(target string) string {
	sum := sha256.Sum256([]byte("aikit-delete-manifest-symlink-v1\x00" + target))
	return hex.EncodeToString(sum[:])
}
func directoryManifestHash() string {
	sum := sha256.Sum256([]byte("aikit-delete-manifest-directory-v1"))
	return hex.EncodeToString(sum[:])
}
func deleteEntryQuarantine(root, rel string) string {
	sum := sha256.Sum256([]byte(filepath.Base(root) + "\x00" + rel))
	return filepath.Join(filepath.Dir(root), ".aikit-adopt-entry-"+hex.EncodeToString(sum[:12]))
}
func pathDepth(path string) int {
	if path == "." {
		return 0
	}
	return len(filepath.SplitList(filepath.ToSlash(path))) + len(path)
}
func existsLstat(path string) bool { _, err := os.Lstat(path); return err == nil }
func lstatExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
