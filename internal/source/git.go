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
// An optional branch can be provided to work on a specific branch.
func CloneOrFetch(source, cacheDir string, branch ...string) error {
	repoURL := ToGitURL(source)
	br := ""
	if len(branch) > 0 {
		br = branch[0]
	}

	_, err := os.Stat(filepath.Join(cacheDir, ".git"))
	if err != nil {
		if os.IsNotExist(err) {
			return clone(repoURL, cacheDir, br)
		}
		return err
	}
	return fetch(cacheDir, br)
}

// ToGitURL converts a source string (shorthand, HTTPS, SSH) to a full Git URL.
func ToGitURL(source string) string {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "git@") {
		return source
	}
	// GitHub shorthand
	return "https://github.com/" + source + ".git"
}

func clone(repoURL, dir, branch string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}

	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, "--depth", "1", repoURL, dir)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil && branch != "" {
		// Branch may not exist yet (e.g. empty repo); retry without -b
		args = []string{"clone", "--depth", "1", repoURL, dir}
		cmd = exec.Command("git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
	}
	if err != nil {
		// Last resort: clone without --depth (empty repos may reject shallow clone)
		_ = os.RemoveAll(dir)
		cmd = exec.Command("git", "clone", repoURL, dir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err2 := cmd.Run(); err2 != nil {
			return fmt.Errorf("git clone: %w", err2)
		}
	}
	return nil
}

func isEmptyRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = dir
	return cmd.Run() != nil
}

func fetch(dir, branch string) error {
	if isEmptyRepo(dir) {
		if branch != "" {
			return checkoutBranch(dir, branch)
		}
		return nil
	}

	if branch != "" {
		if err := checkoutBranch(dir, branch); err != nil {
			return err
		}
	}

	cmd := exec.Command("git", "pull", "--ff-only")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}
	return nil
}

// checkoutBranch switches to the given branch, creating it if it doesn't exist locally.
func checkoutBranch(dir, branch string) error {
	// Try switching to an existing branch first
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if cmd.Run() == nil {
		return nil
	}
	// Branch doesn't exist locally; create it
	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout -b %s: %w", branch, err)
	}
	return nil
}
