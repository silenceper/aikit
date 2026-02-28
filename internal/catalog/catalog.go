package catalog

import (
	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/pkg/config"
)

// AddSkill adds a skill entry to the global catalog and saves.
func AddSkill(entry asset.CatalogEntry) error {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	for i := range cfg.Skills {
		if cfg.Skills[i].Name == entry.Name {
			cfg.Skills[i] = entry
			return config.SaveCatalog(cfg)
		}
	}
	cfg.Skills = append(cfg.Skills, entry)
	return config.SaveCatalog(cfg)
}

// AddMCP adds an MCP entry to the global catalog and saves.
func AddMCP(entry asset.CatalogEntry) error {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	for i := range cfg.Mcps {
		if cfg.Mcps[i].Name == entry.Name {
			cfg.Mcps[i] = entry
			return config.SaveCatalog(cfg)
		}
	}
	cfg.Mcps = append(cfg.Mcps, entry)
	return config.SaveCatalog(cfg)
}

// RemoveSkill removes a skill by name and saves.
func RemoveSkill(name string) error {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	var newSkills []asset.CatalogEntry
	for _, e := range cfg.Skills {
		if e.Name != name {
			newSkills = append(newSkills, e)
		}
	}
	cfg.Skills = newSkills
	return config.SaveCatalog(cfg)
}

// RemoveMCP removes an MCP by name and saves.
func RemoveMCP(name string) error {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	var newMcps []asset.CatalogEntry
	for _, e := range cfg.Mcps {
		if e.Name != name {
			newMcps = append(newMcps, e)
		}
	}
	cfg.Mcps = newMcps
	return config.SaveCatalog(cfg)
}

// FindSkill returns the catalog entry for a skill by name, or nil.
func FindSkill(name string) (*asset.CatalogEntry, error) {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return nil, err
	}
	for i := range cfg.Skills {
		if cfg.Skills[i].Name == name {
			return &cfg.Skills[i], nil
		}
	}
	return nil, nil
}

// FindMCP returns the catalog entry for an MCP by name, or nil.
func FindMCP(name string) (*asset.CatalogEntry, error) {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return nil, err
	}
	for i := range cfg.Mcps {
		if cfg.Mcps[i].Name == name {
			return &cfg.Mcps[i], nil
		}
	}
	return nil, nil
}
