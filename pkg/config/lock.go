package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type fileLock struct {
	file *os.File
}

func acquireLock(ctx context.Context, path string) (*fileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open config lock: %w", err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		locked, lockErr := tryFileLock(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock config: %w", lockErr)
		}
		if locked {
			return &fileLock{file: file}, nil
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (l *fileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
