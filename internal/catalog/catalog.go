package catalog

import (
	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/pkg/config"
)

// --- Add ---

func AddSkill(entry asset.CatalogEntry) error {
	return upsert(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Skills }, entry)
}

func AddRule(entry asset.CatalogEntry) error {
	return upsert(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Rules }, entry)
}

func AddMCP(entry asset.CatalogEntry) error {
	return upsert(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Mcps }, entry)
}

func AddCommand(entry asset.CatalogEntry) error {
	return upsert(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Commands }, entry)
}

// --- Remove (returns true if entry was found and removed) ---

func RemoveSkill(name string) (bool, error) {
	return remove(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Skills }, name)
}

func RemoveRule(name string) (bool, error) {
	return remove(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Rules }, name)
}

func RemoveMCP(name string) (bool, error) {
	return remove(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Mcps }, name)
}

func RemoveCommand(name string) (bool, error) {
	return remove(func(c *config.CatalogConfig) *[]asset.CatalogEntry { return &c.Commands }, name)
}

// --- Find ---

func FindSkill(name string) (*asset.CatalogEntry, error) {
	return find(func(c *config.CatalogConfig) []asset.CatalogEntry { return c.Skills }, name)
}

func FindRule(name string) (*asset.CatalogEntry, error) {
	return find(func(c *config.CatalogConfig) []asset.CatalogEntry { return c.Rules }, name)
}

func FindMCP(name string) (*asset.CatalogEntry, error) {
	return find(func(c *config.CatalogConfig) []asset.CatalogEntry { return c.Mcps }, name)
}

func FindCommand(name string) (*asset.CatalogEntry, error) {
	return find(func(c *config.CatalogConfig) []asset.CatalogEntry { return c.Commands }, name)
}

// --- helpers ---

type sliceAccessor func(*config.CatalogConfig) *[]asset.CatalogEntry
type sliceReader func(*config.CatalogConfig) []asset.CatalogEntry

func upsert(accessor sliceAccessor, entry asset.CatalogEntry) error {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	s := accessor(cfg)
	for i := range *s {
		if (*s)[i].Name == entry.Name {
			(*s)[i] = entry
			return config.SaveCatalog(cfg)
		}
	}
	*s = append(*s, entry)
	return config.SaveCatalog(cfg)
}

func remove(accessor sliceAccessor, name string) (bool, error) {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return false, err
	}
	s := accessor(cfg)
	found := false
	var out []asset.CatalogEntry
	for _, e := range *s {
		if e.Name == name {
			found = true
		} else {
			out = append(out, e)
		}
	}
	*s = out
	return found, config.SaveCatalog(cfg)
}

func find(reader sliceReader, name string) (*asset.CatalogEntry, error) {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return nil, err
	}
	for _, e := range reader(cfg) {
		if e.Name == name {
			return &e, nil
		}
	}
	return nil, nil
}
