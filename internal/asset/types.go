package asset

// AssetRef is a reference to an asset (source + name), used in .aikit.yaml.
type AssetRef struct {
	Source    string         `yaml:"source"`
	Name     string         `yaml:"name"`
	Variables map[string]any `yaml:"variables,omitempty"`
}

// CatalogEntry is a catalog entry (name, source, description, group), used in ~/.aikit/catalog.yaml.
type CatalogEntry struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source"`
	Description string `yaml:"description"`
	Group       string `yaml:"group"`
}

// LocalRule is an inline rule defined in .aikit.yaml (local_rules).
type LocalRule struct {
	Name        string   `yaml:"name"`
	Content     string   `yaml:"content"`
	AlwaysApply bool     `yaml:"always_apply,omitempty"`
	Globs       []string `yaml:"globs,omitempty"`
}

// RuleData is a resolved rule ready to be installed into an IDE.
type RuleData struct {
	Name        string
	Content     string
	Globs       []string
	AlwaysApply bool
}

// MCPData is a resolved MCP config ready to be installed into an IDE.
type MCPData struct {
	Name              string
	Transport         string
	Command           string
	Args              []string
	Env               map[string]string
	ServerInstructions string
}

// CommandData is a resolved command ready to be installed into an IDE.
type CommandData struct {
	Name    string
	Content string
}
