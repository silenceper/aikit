package source

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CloneOrFetch clones the repo into cacheDir if it does not exist, otherwise fetches (git pull).
// cacheDir is the full path to the cache subdir (e.g. ~/.aikit/cache/silenceper/ai-assets).
// source can be shorthand, HTTPS, or SSH URL.
func CloneOrFetch(source, cacheDir string) error {
	repoURL := toGitURL(source)
	_, err := os.Stat(filepath.Join(cacheDir, ".git"))
	if err != nil {
		if os.IsNotExist(err) {
			return clone(repoURL, cacheDir)
		}
		return err
	}
	return fetch(cacheDir)
}

func toGitURL(source string) string {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "git@") {
		return source
	}
	// GitHub shorthand
	return "https://github.com/" + source + ".git"
}

func clone(repoURL, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	return nil
}

func fetch(dir string) error {
	cmd := exec.Command("git", "pull", "--ff-only")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}
	return nil
}
