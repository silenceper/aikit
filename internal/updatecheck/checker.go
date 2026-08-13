package updatecheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/pkg/config"
)

var fullObjectID = regexp.MustCompile(`^[0-9a-fA-F]{40}(?:[0-9a-fA-F]{24})?$`)

const maxGitErrorBytes = 1024

var (
	urlUserInfo = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s'"<>]*@`)
	urlQuery    = regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.-]*://[^\s'"<>?#]+)[?#][^\s'"<>]*`)
	authHeader  = regexp.MustCompile(`(?i)(\bauthorization\s*[:=]\s*)[^\r\n]*`)
)

type Checker struct {
	cachePath string
	runner    GitRunner
	ttl       time.Duration
	now       func() time.Time
	gate      chan struct{}
}

func New(cachePath string, runner GitRunner, options ...Option) *Checker {
	checker := &Checker{
		cachePath: cachePath,
		runner:    runner,
		ttl:       DefaultTTL,
		now:       time.Now,
		gate:      make(chan struct{}, 1),
	}
	for _, option := range options {
		option(checker)
	}
	return checker
}

func (checker *Checker) Check(ctx context.Context, skills []config.Skill, options CheckOptions) (CheckReport, error) {
	results := make([]Result, 0, len(skills))
	if err := checker.acquire(ctx); err != nil {
		return CheckReport{Results: results}, err
	}
	defer checker.release()
	if err := ctx.Err(); err != nil {
		return CheckReport{Results: results}, err
	}
	cache, rebuildCache, cacheErr := loadCache(checker.cachePath)
	warnings := make([]string, 0, 2)
	if cacheErr != nil {
		warnings = append(warnings, cacheErr.Error())
	}
	now := checker.now().UTC()
	dirty := rebuildCache
	refreshed := make(map[string]cacheEntry)
	updates := make(map[string]cacheEntry)

	for _, skill := range skills {
		if err := ctx.Err(); err != nil {
			return CheckReport{Results: results, Warnings: warnings}, err
		}
		result := Result{SkillID: skill.ID, Current: skill.Resolved}
		if skill.Source == "" {
			result.State = StateLocal
			results = append(results, result)
			continue
		}
		if skill.Ref == nil {
			result.State = StateCheckFailed
			result.Error = "remote skill has no ref"
			results = append(results, result)
			continue
		}
		if skill.Ref.Kind == "tag" || skill.Ref.Kind == "commit" {
			result.State = StatePinned
			results = append(results, result)
			continue
		}
		if skill.Ref.Kind != "branch" {
			result.State = StateCheckFailed
			result.Error = fmt.Sprintf("unsupported ref kind %q", skill.Ref.Kind)
			results = append(results, result)
			continue
		}
		if options.Offline {
			result.State = StateOffline
			results = append(results, result)
			continue
		}

		canonical, err := CanonicalSource(skill.Source)
		if err != nil {
			result.State = StateCheckFailed
			result.Error = sanitizeGitError(err)
			results = append(results, result)
			continue
		}
		key := cacheKey(canonical, skill.Ref)
		entry, wasRefreshed := refreshed[key]
		if !wasRefreshed {
			entry, wasRefreshed = cache.Entries[key]
			age := now.Sub(entry.CheckedAt)
			fresh := wasRefreshed &&
				entry.Source == canonical &&
				entry.RefKind == skill.Ref.Kind &&
				entry.RefValue == skill.Ref.Value &&
				fullObjectID.MatchString(entry.Remote) &&
				age >= 0 && age < checker.ttl
			if options.ForceRefresh || !wasRefreshed || !fresh {
				if checker.runner == nil {
					result.State = StateCheckFailed
					result.Error = "git runner is nil"
					results = append(results, result)
					continue
				}
				remote, err := checker.runner.RemoteBranchHead(ctx, skill.Source, skill.Ref.Value)
				if err != nil {
					result.State = StateCheckFailed
					result.Error = sanitizeGitError(err)
					results = append(results, result)
					continue
				}
				remote = strings.TrimSpace(remote)
				if !fullObjectID.MatchString(remote) {
					result.State = StateCheckFailed
					result.Error = "remote did not return a full hexadecimal object id"
					results = append(results, result)
					continue
				}
				entry = cacheEntry{
					Source: canonical, RefKind: skill.Ref.Kind, RefValue: skill.Ref.Value,
					Remote: strings.ToLower(remote), CheckedAt: now,
				}
				cache.Entries[key] = entry
				refreshed[key] = entry
				updates[key] = entry
				dirty = true
			} else {
				result.FromCache = true
			}
		}

		result.Remote = entry.Remote
		if strings.EqualFold(result.Current, result.Remote) {
			result.State = StateCurrent
		} else {
			result.State = StateUpdateAvailable
		}
		results = append(results, result)
	}
	if err := ctx.Err(); err != nil {
		return CheckReport{Results: results, Warnings: warnings}, err
	}

	if dirty {
		if err := mergeAndWriteCache(ctx, checker.cachePath, updates); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return CheckReport{Results: results, Warnings: warnings}, err
			}
			warnings = append(warnings, err.Error())
		}
	}
	return CheckReport{Results: results, Warnings: warnings}, nil
}

func CanonicalSource(source string) (string, error) {
	canonical, err := library.NormalizeSource(source)
	if err != nil {
		return "", errors.New(sanitizeGitError(err))
	}
	return canonical, nil
}

func (checker *Checker) acquire(ctx context.Context) error {
	select {
	case checker.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (checker *Checker) release() {
	<-checker.gate
}

type CommandGitRunner struct {
	GitBinary string
	CacheRoot string
	Executor  CommandExecutor
}

type CommandExecutor interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

func (runner CommandGitRunner) RemoteBranchHead(ctx context.Context, source, branch string) (head string, resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.New(sanitizeGitError(resultErr))
		}
	}()
	if strings.TrimSpace(runner.CacheRoot) == "" {
		return "", fmt.Errorf("repository cache root is empty")
	}
	canonical, err := CanonicalSource(source)
	if err != nil {
		return "", err
	}
	if !validBranchName(branch) {
		return "", fmt.Errorf("invalid branch %q", branch)
	}
	repository, err := library.RepoCachePath(runner.CacheRoot, canonical)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(repository)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("repository cache missing for %q; re-add the skill", canonical)
	}
	if err != nil {
		return "", fmt.Errorf("inspect repository cache for %q: %w", canonical, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("repository cache for %q is not a safe directory; re-add the skill", canonical)
	}
	executor := runner.Executor
	if executor == nil {
		executor = commandExecutor{binary: runner.GitBinary}
	}
	originOutput, err := executor.Run(ctx, repository, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("read repository origin: %s", sanitizeGitError(err))
	}
	origin := strings.TrimSpace(originOutput)
	if origin == "" || strings.ContainsRune(origin, '\x00') {
		return "", fmt.Errorf("repository origin is empty or invalid; re-add the skill")
	}

	cacheRoot := filepath.Dir(filepath.Dir(repository))
	temporaryRoot, err := os.MkdirTemp(cacheRoot, ".update-check-repo-*")
	if err != nil {
		return "", fmt.Errorf("create temporary update-check repository: %w", err)
	}
	defer os.RemoveAll(temporaryRoot)
	temporaryRepository := filepath.Join(temporaryRoot, "repository.git")
	if _, err := executor.Run(ctx, cacheRoot, "clone", "--mirror", "--no-local", "--", repository, temporaryRepository); err != nil {
		return "", fmt.Errorf("clone temporary update-check repository: %s", sanitizeGitError(err))
	}
	if _, err := executor.Run(ctx, temporaryRepository, "remote", "set-url", "origin", origin); err != nil {
		return "", fmt.Errorf("configure temporary repository origin: %s", sanitizeGitError(err))
	}

	ref := "refs/remotes/aikit/" + branch
	refspec := "+refs/heads/" + branch + ":" + ref
	if _, err := executor.Run(ctx, temporaryRepository, "fetch", "--prune", "--no-tags", "origin", refspec); err != nil {
		return "", fmt.Errorf("fetch branch %q: %s", branch, sanitizeGitError(err))
	}
	output, err := executor.Run(ctx, temporaryRepository, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve fetched branch %q: %s", branch, sanitizeGitError(err))
	}
	return strings.TrimSpace(output), nil
}

func validBranchName(branch string) bool {
	if branch == "" || branch == "@" || strings.TrimSpace(branch) != branch || strings.HasPrefix(branch, "-") ||
		strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.HasSuffix(branch, ".") ||
		strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.Contains(branch, "//") ||
		strings.ContainsAny(branch, "~^:?*[\\\x00") {
		return false
	}
	for _, segment := range strings.Split(branch, "/") {
		if segment == "" || strings.HasPrefix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return false
		}
	}
	for _, character := range branch {
		if character <= 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func sanitizeGitError(err error) string {
	if err == nil {
		return "unknown git error"
	}
	message := err.Error()
	message = urlUserInfo.ReplaceAllString(message, `${1}[REDACTED]@`)
	message = urlQuery.ReplaceAllString(message, `${1}?[REDACTED]`)
	message = authHeader.ReplaceAllString(message, `${1}[REDACTED]`)
	message = strings.TrimSpace(message)
	if len(message) <= maxGitErrorBytes {
		return message
	}
	const suffix = "... [truncated]"
	message = message[:maxGitErrorBytes-len(suffix)]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message + suffix
}

type commandExecutor struct {
	binary string
}

func (executor commandExecutor) Run(ctx context.Context, dir string, args ...string) (string, error) {
	binary := executor.binary
	if binary == "" {
		binary = "git"
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		failure := fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		return "", fmt.Errorf("%s", sanitizeGitError(failure))
	}
	return string(output), nil
}
