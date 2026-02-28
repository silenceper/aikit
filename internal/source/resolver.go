package source

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// NormalizeSource converts a source string to a stable cache subdir under ~/.aikit/cache/.
// Examples:
//   - "silenceper/ai-assets" -> "silenceper/ai-assets"
//   - "https://github.com/silenceper/ai-assets.git" -> "silenceper/ai-assets"
//   - "git@github.com:silenceper/ai-assets.git" -> "silenceper/ai-assets"
//   - "https://gitlab.com/team/repo" -> "gitlab.com/team/repo"
func NormalizeSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" || source == "_local" {
		return source
	}
	// GitHub shorthand: owner/repo
	if ok, _ := regexp.MatchString(`^[a-zA-Z0-9][-a-zA-Z0-9]*/[a-zA-Z0-9_.-]+$`, source); ok && !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		return source
	}
	// SSH: git@host:owner/repo.git
	if strings.HasPrefix(source, "git@") {
		rest := strings.TrimPrefix(source, "git@")
		host, repo, ok := cut(rest, ":")
		if ok {
			repo = strings.TrimSuffix(repo, ".git")
			// git@github.com:user/repo -> user/repo (same as shorthand)
			if host == "github.com" {
				return repo
			}
			return host + "/" + repo
		}
		return sanitizeDir(rest)
	}
	// HTTPS or other URL
	u, err := url.Parse(source)
	if err != nil {
		return sanitizeDir(source)
	}
	host := u.Hostname()
	p := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' })
	if len(parts) >= 2 {
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]
		// GitHub URLs map to shorthand (owner/repo) for consistent cache paths
		if host == "github.com" {
			return owner + "/" + repo
		}
		return host + "/" + path.Join(owner, repo)
	}
	if len(parts) == 1 {
		return host + "/" + parts[0]
	}
	return sanitizeDir(host + u.Path)
}

func cut(s, sep string) (before, after string, ok bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func sanitizeDir(s string) string {
	s = strings.TrimPrefix(strings.TrimSuffix(s, ".git"), "://")
	s = strings.ReplaceAll(s, ":", "/")
	s = strings.ReplaceAll(s, "@", "-")
	return strings.Trim(s, "/")
}
