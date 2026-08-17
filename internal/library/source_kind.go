package library

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode"
)

type AddSourceKind string

const (
	AddSourceLocal  AddSourceKind = "local"
	AddSourceRemote AddSourceKind = "remote"
)

// ResolvedAddSource is the transport-ready representation of an add source.
// SuggestedSelections is populated for an exact skills.sh skill page and is
// advisory: an explicit user selection always takes precedence.
type ResolvedAddSource struct {
	Kind                AddSourceKind
	Source              string
	SuggestedSelections []string
}

// ResolveAddSource recognizes local directories, supported Git spellings and
// public skills.sh routes. skills.sh is only an index: cloning always happens
// directly from the corresponding GitHub repository.
func ResolveAddSource(source string) (ResolvedAddSource, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" || strings.ContainsRune(trimmed, '\x00') {
		return ResolvedAddSource{}, fmt.Errorf("source is empty or contains NUL")
	}
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return ResolvedAddSource{}, fmt.Errorf("invalid source URL")
		}
		if (u.Scheme == "http" || u.Scheme == "https") && u.User != nil {
			return ResolvedAddSource{}, fmt.Errorf("HTTP git source must not contain embedded credentials")
		}
		host := strings.ToLower(u.Hostname())
		if host == "skills.sh" || host == "www.skills.sh" {
			return resolveSkillsSHSource(u)
		}
		if _, err := NormalizeSource(trimmed); err != nil {
			return ResolvedAddSource{}, err
		}
		return ResolvedAddSource{Kind: AddSourceRemote, Source: trimmed}, nil
	}
	if looksLikeSCPSource(trimmed) {
		if _, err := NormalizeSource(trimmed); err != nil {
			return ResolvedAddSource{}, err
		}
		return ResolvedAddSource{Kind: AddSourceRemote, Source: trimmed}, nil
	}
	if info, err := os.Stat(trimmed); err == nil {
		if !info.IsDir() {
			return ResolvedAddSource{}, fmt.Errorf("local source %q is not a directory", source)
		}
		return ResolvedAddSource{Kind: AddSourceLocal, Source: trimmed}, nil
	} else if !os.IsNotExist(err) {
		return ResolvedAddSource{}, err
	}
	canonical, err := NormalizeSource(trimmed)
	if err == nil {
		parts := strings.Split(canonical, "/")
		if len(parts) == 2 {
			return ResolvedAddSource{Kind: AddSourceRemote, Source: "https://github.com/" + canonical + ".git"}, nil
		}
		return ResolvedAddSource{Kind: AddSourceRemote, Source: trimmed}, nil
	}
	return ResolvedAddSource{Kind: AddSourceLocal, Source: trimmed}, nil
}

func resolveSkillsSHSource(u *url.URL) (ResolvedAddSource, error) {
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return ResolvedAddSource{}, fmt.Errorf("skills.sh source must be a credential-free HTTPS page without query or fragment")
	}
	rawSegments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(rawSegments) != 2 && len(rawSegments) != 3 {
		return ResolvedAddSource{}, fmt.Errorf("skills.sh source must be /owner/repository or /owner/repository/skill")
	}
	segments := make([]string, len(rawSegments))
	for i, raw := range rawSegments {
		decoded, err := url.PathUnescape(raw)
		if err != nil || !safeSourceSegment(decoded) {
			return ResolvedAddSource{}, fmt.Errorf("unsafe skills.sh path segment %q", raw)
		}
		segments[i] = decoded
	}
	repository := strings.TrimSuffix(segments[1], ".git")
	if !safeSourceSegment(repository) {
		return ResolvedAddSource{}, fmt.Errorf("unsafe skills.sh repository segment")
	}
	resolved := ResolvedAddSource{
		Kind:   AddSourceRemote,
		Source: "https://github.com/" + url.PathEscape(segments[0]) + "/" + url.PathEscape(repository) + ".git",
	}
	if len(segments) == 3 {
		resolved.SuggestedSelections = []string{segments[2]}
	}
	return resolved, nil
}

func safeSourceSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, "/\\\x00") {
		return false
	}
	for _, current := range segment {
		if unicode.IsControl(current) {
			return false
		}
	}
	return true
}

// ClassifyAddSource recognizes unambiguous Git transports before consulting
// the filesystem. Ambiguous owner/repo shorthand retains the existing add
// behavior: an existing path is local, otherwise it is GitHub shorthand.
func ClassifyAddSource(source string) (AddSourceKind, error) {
	resolved, err := ResolveAddSource(source)
	return resolved.Kind, err
}
