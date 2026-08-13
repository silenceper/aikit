package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/silenceper/aikit/pkg/config"
)

const cacheVersion = 1

type cacheEntry struct {
	Source    string    `json:"source"`
	RefKind   string    `json:"ref_kind"`
	RefValue  string    `json:"ref_value"`
	Remote    string    `json:"remote"`
	CheckedAt time.Time `json:"checked_at"`
}

type cacheFile struct {
	Version int                   `json:"version"`
	Entries map[string]cacheEntry `json:"entries"`
}

func newCacheFile() cacheFile {
	return cacheFile{Version: cacheVersion, Entries: make(map[string]cacheEntry)}
}

func loadCache(path string) (cacheFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newCacheFile(), false, nil
		}
		return newCacheFile(), false, fmt.Errorf("read update cache: %w", err)
	}
	var cache cacheFile
	if err := json.Unmarshal(data, &cache); err != nil || cache.Version != cacheVersion || cache.Entries == nil {
		return newCacheFile(), true, nil
	}
	return cache, false, nil
}

func cacheKey(source string, ref *config.Ref) string {
	// JSON is used as a deterministic, collision-free encoding of the required
	// (canonical source, ref kind, ref value) tuple.
	encoded, _ := json.Marshal([3]string{source, ref.Kind, ref.Value})
	return string(encoded)
}

func mergeAndWriteCache(ctx context.Context, path string, updates map[string]cacheEntry) error {
	unlock, err := acquireCacheLock(ctx, path+".lock")
	if err != nil {
		return fmt.Errorf("lock update cache: %w", err)
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	latest, corrupt, err := loadCache(path)
	if err != nil {
		return err
	}
	for key, entry := range updates {
		current, exists := latest.Entries[key]
		if !exists || preferIncomingCacheEntry(current, entry) {
			latest.Entries[key] = entry
		}
	}
	if !corrupt && len(updates) == 0 {
		return nil
	}
	return writeCacheAtomic(path, latest)
}

func preferIncomingCacheEntry(current, incoming cacheEntry) bool {
	if incoming.CheckedAt.After(current.CheckedAt) {
		return true
	}
	if current.CheckedAt.After(incoming.CheckedAt) {
		return false
	}
	return incoming.Remote > current.Remote
}

func writeCacheAtomic(path string, cache cacheFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create update cache directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create update cache temporary file: %w", err)
	}
	tempName := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempName)
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("set update cache permissions: %w", err)
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cache); err != nil {
		return fmt.Errorf("encode update cache: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync update cache: %w", err)
	}
	if err := temp.Close(); err != nil {
		closed = true
		return fmt.Errorf("close update cache: %w", err)
	}
	closed = true
	if err := replaceFile(tempName, path); err != nil {
		return fmt.Errorf("replace update cache: %w", err)
	}

	// Syncing the containing directory makes the rename durable on filesystems
	// that require an explicit directory sync. Failure here does not make the
	// already completed atomic replacement invalid.
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}
