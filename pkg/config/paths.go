package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Home          string
	Config        string
	Lock          string
	LibrarySkills string
	Cache         string
	UpdateCache   string
}

func DefaultPaths() (Paths, error) {
	home := os.Getenv("AIKIT_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		home = filepath.Join(userHome, ".aikit")
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return Paths{}, err
	}
	return PathsForHome(abs), nil
}

func PathsForHome(home string) Paths {
	home = filepath.Clean(home)
	cache := filepath.Join(home, "cache")
	return Paths{
		Home:          home,
		Config:        filepath.Join(home, "config.yaml"),
		Lock:          filepath.Join(home, "config.lock"),
		LibrarySkills: filepath.Join(home, "library", "skills"),
		Cache:         cache,
		UpdateCache:   filepath.Join(cache, ".update-check"),
	}
}
