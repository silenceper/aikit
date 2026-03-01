package config

import (
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
	"gopkg.in/yaml.v3"
)

// ProjectConfig is the root structure of .aikit.yaml.
type ProjectConfig struct {
	Project struct {
		Name    string   `yaml:"name"`
		Targets []string `yaml:"targets,omitempty"`
	} `yaml:"project"`
	Assets struct {
		Skills   []asset.AssetRef `yaml:"skills,omitempty"`
		Rules    []asset.AssetRef `yaml:"rules,omitempty"`
		Mcps     []asset.AssetRef `yaml:"mcps,omitempty"`
		Commands []asset.AssetRef `yaml:"commands,omitempty"`
	} `yaml:"assets"`
	LocalRules []asset.LocalRule `yaml:"local_rules,omitempty"`
}

const projectFileName = ".aikit.yaml"

// LoadProject loads .aikit.yaml from dir (default: current directory).
func LoadProject(dir string) (*ProjectConfig, error) {
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, projectFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveProject writes .aikit.yaml to dir.
func SaveProject(dir string, cfg *ProjectConfig) error {
	if dir == "" {
		dir = "."
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, projectFileName)
	return os.WriteFile(path, data, 0644)
}

// LoadProjectFile loads a ProjectConfig from an arbitrary file path.
func LoadProjectFile(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ProjectPath returns the path to .aikit.yaml in dir.
func ProjectPath(dir string) string {
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, projectFileName)
}
