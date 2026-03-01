package discovery

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AssetYAML represents the standard asset.yaml format for rules, MCPs, and commands.
type AssetYAML struct {
	Kind     string   `yaml:"kind"`
	Metadata Metadata `yaml:"metadata"`
	Spec     Spec     `yaml:"spec"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Version     string `yaml:"version,omitempty"`
}

type Spec struct {
	ContentFile string            `yaml:"content_file,omitempty"`
	Globs       []string          `yaml:"globs,omitempty"`
	AlwaysApply bool              `yaml:"always_apply,omitempty"`
	Variables   map[string]VarDef `yaml:"variables,omitempty"`
	// MCP-specific
	Transport         string            `yaml:"transport,omitempty"`
	Command           string            `yaml:"command,omitempty"`
	Args              []string          `yaml:"args,omitempty"`
	Env               map[string]string `yaml:"env,omitempty"`
	PlatformOverrides map[string]Spec   `yaml:"platform_overrides,omitempty"`
}

type VarDef struct {
	Default     string `yaml:"default,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// AssetInfo holds a discovered asset's metadata and location.
type AssetInfo struct {
	Kind string
	Name string
	Desc string
	Dir  string
	Spec Spec
}

// DiscoverAll finds all asset.yaml files under root and returns discovered assets.
func DiscoverAll(root string) ([]AssetInfo, error) {
	var out []AssetInfo
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		if fi.Name() != "asset.yaml" {
			return nil
		}
		info, err := parseAssetYAML(path)
		if err != nil {
			return nil
		}
		out = append(out, *info)
		return nil
	})
	return out, err
}

// DiscoverByKind returns assets of a specific kind (rule, mcp, command).
func DiscoverByKind(root, kind string) ([]AssetInfo, error) {
	all, err := DiscoverAll(root)
	if err != nil {
		return nil, err
	}
	var out []AssetInfo
	for _, a := range all {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	return out, nil
}

func parseAssetYAML(path string) (*AssetInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ay AssetYAML
	if err := yaml.Unmarshal(data, &ay); err != nil {
		return nil, err
	}
	if ay.Kind == "" || ay.Metadata.Name == "" {
		return nil, nil
	}
	dir := filepath.Dir(path)
	return &AssetInfo{
		Kind: ay.Kind,
		Name: ay.Metadata.Name,
		Desc: ay.Metadata.Description,
		Dir:  dir,
		Spec: ay.Spec,
	}, nil
}

// LoadContent reads the content_file referenced by an asset's spec.
// Returns empty string if no content_file is specified.
func LoadContent(info AssetInfo) (string, error) {
	if info.Spec.ContentFile == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(info.Dir, info.Spec.ContentFile))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
