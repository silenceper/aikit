package config

import (
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
	"gopkg.in/yaml.v3"
)

// CatalogConfig is the root structure of ~/.aikit/catalog.yaml.
type CatalogConfig struct {
	Skills   []asset.CatalogEntry `yaml:"skills,omitempty"`
	Rules    []asset.CatalogEntry `yaml:"rules,omitempty"`
	Mcps     []asset.CatalogEntry `yaml:"mcps,omitempty"`
	Commands []asset.CatalogEntry `yaml:"commands,omitempty"`
}

const (
	aikitDirName  = ".aikit"
	catalogName   = "catalog.yaml"
	cacheDirName  = "cache"
	localSourceID = "_local"
)

// AikitHome returns the aikit home directory (e.g. ~/.aikit).
func AikitHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, aikitDirName), nil
}

// CatalogPath returns the path to catalog.yaml.
func CatalogPath() (string, error) {
	home, err := AikitHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, catalogName), nil
}

// CacheDir returns the path to ~/.aikit/cache/.
func CacheDir() (string, error) {
	home, err := AikitHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, cacheDirName), nil
}

// LocalSourceID is the source value for locally registered assets.
func LocalSourceID() string { return localSourceID }

// LoadCatalog loads ~/.aikit/catalog.yaml. Returns nil config if file does not exist.
func LoadCatalog() (*CatalogConfig, error) {
	path, err := CatalogPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CatalogConfig{}, nil
		}
		return nil, err
	}
	var cfg CatalogConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveCatalog writes ~/.aikit/catalog.yaml. Creates ~/.aikit if needed.
func SaveCatalog(cfg *CatalogConfig) error {
	home, err := AikitHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	path := filepath.Join(home, catalogName)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
