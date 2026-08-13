package library

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// RepoCachePath returns the deterministic, contained persistent repository
// cache used by add, update, and update checking.
func RepoCachePath(cacheRoot, canonical string) (string, error) {
	if canonical == "" || strings.ContainsRune(canonical, '\x00') {
		return "", fmt.Errorf("canonical source is empty or contains NUL")
	}
	absRoot, err := filepath.Abs(cacheRoot)
	if err != nil {
		return "", err
	}
	resolvedRoot := absRoot
	if _, statErr := os.Stat(absRoot); statErr == nil {
		resolvedRoot, err = filepath.EvalSymlinks(absRoot)
		if err != nil {
			return "", fmt.Errorf("resolve cache root: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}
	repositories := filepath.Join(resolvedRoot, "repos")
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
	target := filepath.Join(repositories, key)
	if !isWithin(resolvedRoot, target) {
		return "", fmt.Errorf("repository cache path escapes cache root")
	}
	for _, current := range []string{repositories, target} {
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", fmt.Errorf("resolve repository cache path: %w", evalErr)
			}
			if !isWithin(resolvedRoot, resolved) {
				return "", fmt.Errorf("repository cache path escapes cache root")
			}
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
	}
	return target, nil
}

// NormalizeSource converts supported Git source spellings to a stable,
// filesystem-safe identity. GitHub deliberately omits the host so its common
// shorthand and URL forms have the same identity.
func NormalizeSource(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("git source is empty or contains NUL")
	}

	var host, rawPath string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid git source %q", raw)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf("git source must not contain query or fragment")
		}
		host = canonicalHost(u)
		rawPath = strings.TrimPrefix(u.EscapedPath(), "/")
	case looksLikeSCPSource(raw):
		colon := strings.IndexByte(raw, ':')
		left := raw[:colon]
		rawPath = raw[colon+1:]
		if at := strings.LastIndexByte(left, '@'); at >= 0 {
			left = left[at+1:]
		}
		host = strings.ToLower(left)
	default:
		parts := strings.Split(strings.Trim(raw, "/"), "/")
		if len(parts) == 2 {
			host = "github.com"
			rawPath = strings.Join(parts, "/")
		} else if len(parts) >= 3 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
			host = strings.ToLower(parts[0])
			rawPath = strings.Join(parts[1:], "/")
		} else {
			return "", fmt.Errorf("invalid git source %q", raw)
		}
	}
	if host == "" {
		return "", fmt.Errorf("git source %q has no host", raw)
	}

	segments, err := normalizeSegments(rawPath)
	if err != nil {
		return "", err
	}
	if len(segments) < 2 {
		return "", fmt.Errorf("git source must include owner/group and repository")
	}
	if host == "github.com" {
		return strings.Join(segments, "/"), nil
	}
	return host + "/" + strings.Join(segments, "/"), nil
}

func canonicalHost(u *url.URL) string {
	hostname := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") || (u.Scheme == "ssh" && port == "22") {
		port = ""
	}
	if port != "" {
		return net.JoinHostPort(hostname, port)
	}
	return hostname
}

func looksLikeSCPSource(raw string) bool {
	if strings.Contains(raw, "://") {
		return false
	}
	colon := strings.IndexByte(raw, ':')
	slash := strings.IndexByte(raw, '/')
	return colon > 0 && (slash < 0 || colon < slash) && strings.Contains(raw[:colon], "@")
}

func normalizeSegments(rawPath string) ([]string, error) {
	rawSegments := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(rawSegments) == 0 {
		return nil, fmt.Errorf("git source path is empty")
	}
	segments := make([]string, 0, len(rawSegments))
	for i, segment := range rawSegments {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return nil, fmt.Errorf("invalid percent encoding in git source: %w", err)
		}
		if i == len(rawSegments)-1 {
			decoded = strings.TrimSuffix(decoded, ".git")
		}
		if decoded == "" || decoded == "." || decoded == ".." || strings.ContainsRune(decoded, '\x00') {
			return nil, fmt.Errorf("unsafe git source path segment %q", decoded)
		}
		segments = append(segments, url.PathEscape(decoded))
	}
	return segments, nil
}

// ValidateSourcePath validates a slash-separated path relative to a checkout.
func ValidateSourcePath(sourcePath string) error {
	if sourcePath == "" || strings.ContainsAny(sourcePath, "\\\x00") || path.IsAbs(sourcePath) {
		return fmt.Errorf("unsafe source_path %q", sourcePath)
	}
	if path.Clean(sourcePath) != sourcePath {
		return fmt.Errorf("source_path must be clean: %q", sourcePath)
	}
	for _, segment := range strings.Split(sourcePath, "/") {
		if segment == "" || segment == ".." {
			return fmt.Errorf("unsafe source_path %q", sourcePath)
		}
	}
	return nil
}

func validateID(id string) error {
	if id == "" || strings.ContainsAny(id, "\\\x00") || strings.HasPrefix(id, "/") || path.Clean(id) != id {
		return fmt.Errorf("unsafe library id %q", id)
	}
	for _, segment := range strings.Split(id, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("unsafe library id %q", id)
		}
	}
	return nil
}

// SafeLibraryPath returns a contained path, resolving the library root first
// so a symlinked AIKIT_HOME cannot accidentally change the containment check.
func SafeLibraryPath(root, id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolvedRoot := absRoot
	if _, statErr := os.Stat(absRoot); statErr == nil {
		resolvedRoot, err = filepath.EvalSymlinks(absRoot)
		if err != nil {
			return "", fmt.Errorf("resolve library root: %w", err)
		}
	}
	target := filepath.Join(resolvedRoot, filepath.FromSlash(id))
	if !isWithin(resolvedRoot, target) {
		return "", fmt.Errorf("library id %q escapes root", id)
	}

	// Existing intermediate symlinks must also remain within the root.
	current := resolvedRoot
	for _, segment := range strings.Split(id, "/") {
		current = filepath.Join(current, segment)
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", fmt.Errorf("resolve library path %q: %w", current, evalErr)
			}
			if !isWithin(resolvedRoot, resolved) {
				return "", fmt.Errorf("library path %q escapes root", current)
			}
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
	}
	return target, nil
}

func isWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
