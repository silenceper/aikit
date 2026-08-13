package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Store struct {
	Paths Paths
}

type Tx struct {
	Config *Config
	store  *Store
	ctx    context.Context
}

func (s Store) Load(ctx context.Context) (*Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.Paths.Config)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Library.Skills == nil {
		cfg.Library.Skills = []Skill{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

func (s Store) Save(ctx context.Context, cfg *Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return atomicWriteFile(s.Paths.Config, data, 0o600)
}

func (s Store) WithLock(ctx context.Context, fn func(*Tx) error) error {
	lock, err := acquireLock(ctx, s.Paths.Lock)
	if err != nil {
		return err
	}
	defer lock.Close()

	cfg, err := s.Load(ctx)
	if err != nil {
		return err
	}
	return fn(&Tx{Config: cfg, store: &s, ctx: ctx})
}

func (tx *Tx) Checkpoint() error {
	if tx == nil || tx.store == nil || tx.Config == nil {
		return fmt.Errorf("invalid config transaction")
	}
	return tx.store.Save(tx.ctx, tx.Config)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return err
	}
	return nil
}
