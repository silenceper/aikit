package library

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type hashRecord struct {
	path       string
	kind       byte
	executable bool
	content    []byte
}

// HashSkill hashes deterministic typed records and rejects special files and
// symlinks whose fully resolved target leaves the skill root.
func HashSkill(root string) (string, error) {
	records, err := collectHashRecords(root)
	if err != nil {
		return "", err
	}
	first := hashRecords(records)
	verified, err := collectHashRecords(root)
	if err != nil {
		return "", err
	}
	second := hashRecords(verified)
	if first != second {
		return "", fmt.Errorf("skill source changed while hashing")
	}
	return first, nil
}

func hashRecords(records []hashRecord) string {
	h := sha256.New()
	for _, record := range records {
		writeLengthBytes(h, []byte(record.path))
		_, _ = h.Write([]byte{record.kind})
		if record.executable {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
		writeLengthBytes(h, record.content)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func collectHashRecords(root string) ([]hashRecord, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve skill root: %w", err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill root is not a directory")
	}

	var records []hashRecord
	err = filepath.WalkDir(resolvedRoot, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(resolvedRoot, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}

		pathName := filepath.ToSlash(rel)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		record := hashRecord{path: pathName, executable: info.Mode().Perm()&0o111 != 0}
		switch {
		case info.IsDir():
			record.kind = 'd'
		case info.Mode().IsRegular():
			record.kind = 'f'
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return resolveErr
			}
			if !isWithin(resolvedRoot, resolved) {
				return fmt.Errorf("file %q escapes skill root", pathName)
			}
			record.content, err = secureReadRegular(resolvedRoot, rel, info)
		case info.Mode()&os.ModeSymlink != 0:
			record.kind = 'l'
			record.executable = false
			var target string
			target, err = os.Readlink(current)
			if err == nil {
				var resolved string
				resolved, err = filepath.EvalSymlinks(current)
				if err == nil && !isWithin(resolvedRoot, resolved) {
					err = fmt.Errorf("symlink %q escapes skill root", pathName)
				}
			}
			record.content = []byte(target)
		default:
			err = fmt.Errorf("unsupported special file %q", pathName)
		}
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].path < records[j].path })
	return records, nil
}

func writeLengthBytes(w io.Writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = w.Write(length[:])
	_, _ = w.Write(value)
}
