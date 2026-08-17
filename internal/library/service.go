package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/silenceper/aikit/pkg/config"
)

var fullObjectID = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

type Service struct {
	LibraryRoot string
	CacheRoot   string
	Runner      Runner
	CopySkill   func(source, destination string) error
	Rename      func(old, new string) error
	SyncDir     func(path string) error
}

type LocalCandidate struct {
	Origin string
	Candidate
}

type LocalAllocation struct {
	Origin string
	Candidate
	Skill config.Skill
}

// AllocateLocal performs the stable ID allocation shared by scan and local
// add. Callers scanning several roots must pass the whole batch with the
// specification's g/... and p/... Origin ordering keys.
func AllocateLocal(candidates []LocalCandidate, existing []config.Skill) ([]LocalAllocation, error) {
	candidates = append([]LocalCandidate(nil), candidates...)
	sort.SliceStable(candidates, func(i, j int) bool { return localOriginLess(candidates[i].Origin, candidates[j].Origin) })
	existingByNameHash := make(map[string]config.Skill, len(existing))
	occupied := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		existingByNameHash[skill.Name+"\x00"+skill.Hash] = skill
		occupied[skill.ID] = struct{}{}
	}
	result := make([]LocalAllocation, 0, len(candidates))
	for _, candidate := range candidates {
		skill, reused := existingByNameHash[candidate.Name+"\x00"+candidate.Hash]
		if !reused {
			id := "local/" + candidate.Name
			if _, exists := occupied[id]; exists {
				if len(candidate.Hash) < 12 {
					return nil, fmt.Errorf("candidate hash is too short")
				}
				id += "-" + candidate.Hash[:12]
			}
			if _, exists := occupied[id]; exists {
				return nil, fmt.Errorf("local skill id collision %q", id)
			}
			skill = config.Skill{ID: id, Name: candidate.Name, Description: candidate.Description, Hash: candidate.Hash}
			existingByNameHash[candidate.Name+"\x00"+candidate.Hash] = skill
			occupied[id] = struct{}{}
		}
		result = append(result, LocalAllocation{Origin: candidate.Origin, Candidate: candidate.Candidate, Skill: skill})
	}
	return result, nil
}

var localAgentOrder = map[string]int{
	"cursor": 0, "claude-code": 1, "codex": 2, "copilot": 3, "windsurf": 4,
}

func localOriginLess(left, right string) bool {
	l, r := strings.Split(left, "/"), strings.Split(right, "/")
	if len(l) >= 2 && len(r) >= 2 && l[0] == r[0] {
		switch l[0] {
		case "g":
			li, lok := localAgentOrder[l[1]]
			ri, rok := localAgentOrder[r[1]]
			if lok && rok && li != ri {
				return li < ri
			}
		case "p":
			if len(l) >= 3 && len(r) >= 3 {
				if l[1] != r[1] {
					return l[1] < r[1]
				}
				li, lok := localAgentOrder[l[2]]
				ri, rok := localAgentOrder[r[2]]
				if lok && rok && li != ri {
					return li < ri
				}
			}
		}
	}
	return left < right
}

type GitAddOptions struct {
	SourcePath string
	Ref        *config.Ref
	Skill      string
	Skills     []string
	Existing   []config.Skill
	Force      bool
}

// GitPreview is the immutable result of a network-enabled, temporary Git
// discovery. Candidate roots are intentionally omitted because the checkout is
// removed before PreviewGit returns.
type GitPreview struct {
	Candidates []Candidate
	Ref        *config.Ref
	Resolved   string
}

type UpdateSpec struct {
	Old config.Skill
	Ref *config.Ref
}

func (s Service) AddLocal(source string, existing []config.Skill) ([]config.Skill, error) {
	mutation, err := s.PrepareLocal(context.Background(), source, nil, existing)
	if err != nil {
		return nil, err
	}
	defer mutation.Abort()
	if err := mutation.Commit(context.Background()); err != nil {
		return nil, err
	}
	return mutation.Skills, nil
}

func (s Service) PrepareLocal(ctx context.Context, source string, selections []string, existing []config.Skill) (*Mutation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidates, err := Discover(source)
	if err != nil {
		return nil, err
	}
	candidates, err = selectCandidates(candidates, selections)
	if err != nil {
		return nil, err
	}
	inputs := make([]LocalCandidate, len(candidates))
	for i, candidate := range candidates {
		inputs[i] = LocalCandidate{Origin: candidate.RelativePath, Candidate: candidate}
	}
	allocations, err := AllocateLocal(inputs, existing)
	if err != nil {
		return nil, err
	}
	result := make([]config.Skill, 0, len(allocations))
	var specs []CopySpec
	seen := make(map[string]struct{})
	existingIDs := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		existingIDs[skill.ID] = struct{}{}
	}
	for _, allocation := range allocations {
		if _, duplicate := seen[allocation.Skill.ID]; duplicate {
			continue
		}
		seen[allocation.Skill.ID] = struct{}{}
		ledgerDestination := false
		if _, ok := existingIDs[allocation.Skill.ID]; ok {
			ledgerDestination = true
		}
		lexical, err := lexicalLibraryPath(s.LibraryRoot, allocation.Skill.ID)
		if err != nil {
			return nil, err
		}
		if info, statErr := os.Lstat(lexical); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("library destination %q exists without a matching ledger directory", lexical)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		destination, err := SafeLibraryPath(s.LibraryRoot, allocation.Skill.ID)
		if err != nil {
			return nil, err
		}
		if info, statErr := os.Lstat(destination); os.IsNotExist(statErr) {
			specs = append(specs, CopySpec{Source: allocation.Root, Destination: destination})
		} else if statErr != nil {
			return nil, statErr
		} else if !ledgerDestination {
			return nil, fmt.Errorf("library destination %q exists without a matching ledger entry", destination)
		} else if !info.IsDir() {
			return nil, fmt.Errorf("library destination %q is not a directory", destination)
		}
		result = append(result, allocation.Skill)
	}
	if len(specs) > 0 {
		batch, err := PrepareBatch(specs, s.copySkill, s.rename)
		if err != nil {
			return nil, err
		}
		batch.SyncDirectory = s.syncDirectory
		if err := s.configureBatchSkills(batch, result, existing); err != nil {
			_ = batch.Abort()
			return nil, err
		}
		return &Mutation{Skills: result, batch: batch}, nil
	}
	return &Mutation{Skills: result}, nil
}

func (s Service) AddGit(ctx context.Context, rawSource string, options GitAddOptions) ([]config.Skill, error) {
	mutation, err := s.PrepareGit(ctx, rawSource, options)
	if err != nil {
		return nil, err
	}
	defer mutation.Abort()
	if err := mutation.Commit(ctx); err != nil {
		return nil, err
	}
	return mutation.Skills, nil
}

// PreviewGit clones into an operating-system temporary directory, discovers
// candidates, and removes the checkout before returning. It never touches the
// persistent cache or central library.
func (s Service) PreviewGit(ctx context.Context, rawSource, sourcePath string, requested *config.Ref) (GitPreview, error) {
	if s.Runner == nil {
		return GitPreview{}, fmt.Errorf("git runner is required")
	}
	resolvedSource, err := ResolveAddSource(rawSource)
	if err != nil {
		return GitPreview{}, err
	}
	if resolvedSource.Kind != AddSourceRemote {
		return GitPreview{}, fmt.Errorf("git preview requires a remote source")
	}
	if parsed, parseErr := url.Parse(strings.TrimSpace(resolvedSource.Source)); parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User != nil {
		return GitPreview{}, fmt.Errorf("HTTP git source must not contain embedded credentials")
	}
	if _, err := NormalizeSource(resolvedSource.Source); err != nil {
		return GitPreview{}, errors.New(sanitizeGitDiagnostic(err.Error()))
	}
	temporary, err := os.MkdirTemp("", "aikit-git-preview-")
	if err != nil {
		return GitPreview{}, err
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil {
		return GitPreview{}, err
	}
	checkout := filepath.Join(temporary, "checkout")
	if _, err := s.Runner.Run(ctx, temporary, "clone", "--no-checkout", "--", resolvedSource.Source, checkout); err != nil {
		return GitPreview{}, err
	}
	ref := requested
	if ref == nil {
		output, err := s.Runner.Run(ctx, checkout, "symbolic-ref", "--short", "HEAD")
		if err != nil {
			return GitPreview{}, fmt.Errorf("resolve default branch: %w", err)
		}
		branch := strings.TrimPrefix(strings.TrimSpace(output), "origin/")
		ref = &config.Ref{Kind: "branch", Value: branch}
	}
	if err := validateRef(ref); err != nil {
		return GitPreview{}, err
	}
	target := ref.Value
	switch ref.Kind {
	case "branch":
		target = "refs/remotes/origin/" + ref.Value
	case "tag":
		target = "refs/tags/" + ref.Value
	}
	if _, err := s.Runner.Run(ctx, checkout, "checkout", "--detach", target); err != nil {
		return GitPreview{}, err
	}
	output, err := s.Runner.Run(ctx, checkout, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return GitPreview{}, err
	}
	resolved := strings.ToLower(strings.TrimSpace(output))
	if !fullObjectID.MatchString(resolved) {
		return GitPreview{}, fmt.Errorf("git returned a non-full object id")
	}
	candidates, err := discoverSelection(checkout, sourcePath, "", true)
	if err != nil {
		return GitPreview{}, err
	}
	if len(candidates) == 0 {
		return GitPreview{}, fmt.Errorf("repository contains no SKILL.md")
	}
	for i := range candidates {
		candidates[i].Root = ""
	}
	refCopy := *ref
	return GitPreview{Candidates: candidates, Ref: &refCopy, Resolved: resolved}, nil
}

func (s Service) PrepareGit(ctx context.Context, rawSource string, options GitAddOptions) (*Mutation, error) {
	resolvedSource, err := ResolveAddSource(rawSource)
	if err != nil {
		return nil, errors.New(sanitizeGitDiagnostic(err.Error()))
	}
	if resolvedSource.Kind != AddSourceRemote {
		return nil, fmt.Errorf("git add requires a remote source")
	}
	rawSource = resolvedSource.Source
	if len(options.Skills) == 0 && options.Skill == "" {
		options.Skills = append([]string(nil), resolvedSource.SuggestedSelections...)
	}
	if parsed, parseErr := url.Parse(strings.TrimSpace(rawSource)); parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User != nil {
		return nil, fmt.Errorf("HTTP git source must not contain embedded credentials")
	}
	canonical, err := NormalizeSource(rawSource)
	if err != nil {
		return nil, errors.New(sanitizeGitDiagnostic(err.Error()))
	}
	checkout, ref, resolved, cleanup, err := s.checkout(ctx, canonical, rawSource, options.Ref, true)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	candidates, err := discoverSelection(checkout, options.SourcePath, options.Skill, true)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("repository contains no SKILL.md")
	}
	candidates, err = selectCandidates(candidates, options.Skills)
	if err != nil {
		return nil, err
	}
	occupied := make(map[string]struct{}, len(options.Existing))
	for _, skill := range options.Existing {
		occupied[skill.ID] = struct{}{}
	}
	type preparedSkill struct {
		candidate   Candidate
		id          string
		destination string
	}
	prepared := make([]preparedSkill, 0, len(candidates))
	plannedIDs := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		id := canonical + "/" + candidate.Name
		_, ledgerExists := occupied[id]
		if ledgerExists && !options.Force {
			return nil, fmt.Errorf("skill id %q already exists", id)
		}
		if _, duplicate := plannedIDs[id]; duplicate {
			return nil, fmt.Errorf("multiple discovered skills produce id %q", id)
		}
		lexical, err := lexicalLibraryPath(s.LibraryRoot, id)
		if err != nil {
			return nil, err
		}
		if _, statErr := os.Lstat(lexical); statErr == nil && !ledgerExists {
			return nil, fmt.Errorf("library destination %q exists without a matching ledger entry", lexical)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		destination, err := SafeLibraryPath(s.LibraryRoot, id)
		if err != nil {
			return nil, err
		}
		if _, statErr := os.Lstat(destination); statErr == nil && !ledgerExists {
			return nil, fmt.Errorf("library destination %q exists without a matching ledger entry", destination)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		prepared = append(prepared, preparedSkill{candidate: candidate, id: id, destination: destination})
		plannedIDs[id] = struct{}{}
	}
	specs := make([]CopySpec, 0, len(prepared))
	for _, item := range prepared {
		specs = append(specs, CopySpec{Source: item.candidate.Root, Destination: item.destination})
	}
	batch, err := PrepareBatch(specs, s.copySkill, s.rename)
	if err != nil {
		return nil, err
	}
	batch.SyncDirectory = s.syncDirectory
	result := make([]config.Skill, 0, len(prepared))
	for _, item := range prepared {
		refCopy := *ref
		result = append(result, config.Skill{
			ID: item.id, Name: item.candidate.Name, Description: item.candidate.Description,
			Source: canonical, SourcePath: item.candidate.RelativePath, Ref: &refCopy,
			Resolved: resolved, Hash: item.candidate.Hash,
		})
	}
	if err := s.configureBatchSkills(batch, result, options.Existing); err != nil {
		_ = batch.Abort()
		return nil, err
	}
	return &Mutation{Skills: result, batch: batch}, nil
}

// UpdateRef prepares and validates a fresh checkout before replacing the old
// library tree. It returns the original value on every failure.
func (s Service) UpdateRef(ctx context.Context, old config.Skill, requested *config.Ref) (config.Skill, error) {
	mutation, err := s.PrepareUpdate(ctx, old, requested)
	if err != nil {
		return old, err
	}
	defer mutation.Abort()
	if err := mutation.Commit(ctx); err != nil {
		return old, err
	}
	return *mutation.Updated, nil
}

func (s Service) PrepareUpdate(ctx context.Context, old config.Skill, requested *config.Ref) (*Mutation, error) {
	mutation, err := s.PrepareUpdates(ctx, []UpdateSpec{{Old: old, Ref: requested}})
	if err != nil {
		return nil, err
	}
	mutation.Updated = &mutation.Skills[0]
	return mutation, nil
}

func (s Service) PrepareUpdates(ctx context.Context, requests []UpdateSpec) (*Mutation, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("update batch is empty")
	}
	specs := make([]CopySpec, 0, len(requests))
	updated := make([]config.Skill, 0, len(requests))
	var cleanups []func()
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	for _, request := range requests {
		item, copySpec, cleanup, err := s.prepareUpdate(ctx, request.Old, request.Ref)
		if err != nil {
			return nil, err
		}
		cleanups = append(cleanups, cleanup)
		updated = append(updated, item)
		specs = append(specs, copySpec)
	}
	batch, err := PrepareBatch(specs, s.copySkill, s.rename)
	if err != nil {
		return nil, err
	}
	batch.SyncDirectory = s.syncDirectory
	old := make([]config.Skill, 0, len(requests))
	for _, request := range requests {
		old = append(old, request.Old)
	}
	if err := s.configureBatchSkills(batch, updated, old); err != nil {
		_ = batch.Abort()
		return nil, err
	}
	return &Mutation{Skills: updated, batch: batch}, nil
}

func (s Service) configureBatchSkills(batch *Batch, next, previous []config.Skill) error {
	newByPath := make(map[string]config.Skill, len(next))
	oldByID := make(map[string]config.Skill, len(previous))
	for _, skill := range next {
		path, err := SafeLibraryPath(s.LibraryRoot, skill.ID)
		if err != nil {
			return err
		}
		newByPath[path] = skill
	}
	for _, skill := range previous {
		oldByID[skill.ID] = skill
	}
	for index := range batch.Copies {
		copy := &batch.Copies[index]
		newSkill, ok := newByPath[copy.Destination]
		if !ok {
			return fmt.Errorf("batch destination has no new skill metadata")
		}
		newCopy := newSkill
		copy.newSkill = &newCopy
		if oldSkill, ok := oldByID[newSkill.ID]; ok {
			oldCopy := oldSkill
			copy.oldSkill = &oldCopy
			copy.operation = "update"
		} else {
			copy.operation = "add"
		}
	}
	return batch.persistJournal("prepared")
}

func (s Service) prepareUpdate(ctx context.Context, old config.Skill, requested *config.Ref) (config.Skill, CopySpec, func(), error) {
	if old.Source == "" || old.Ref == nil {
		return old, CopySpec{}, func() {}, fmt.Errorf("skill %q has no remote source", old.ID)
	}
	if err := ValidateSourcePath(old.SourcePath); err != nil {
		return old, CopySpec{}, func() {}, err
	}
	ref := old.Ref
	if requested != nil {
		ref = requested
	}
	checkout, actualRef, resolved, cleanup, err := s.checkout(ctx, old.Source, "", ref, false)
	if err != nil {
		return old, CopySpec{}, func() {}, err
	}
	candidates, err := discoverSelection(checkout, old.SourcePath, "", true)
	if err != nil {
		cleanup()
		return old, CopySpec{}, func() {}, err
	}
	if len(candidates) != 1 {
		cleanup()
		return old, CopySpec{}, func() {}, fmt.Errorf("source_path %q is not a single skill", old.SourcePath)
	}
	candidate := candidates[0]
	if candidate.Name != old.Name {
		cleanup()
		return old, CopySpec{}, func() {}, fmt.Errorf("skill name changed from %q to %q; add it again", old.Name, candidate.Name)
	}
	destination, err := SafeLibraryPath(s.LibraryRoot, old.ID)
	if err != nil {
		cleanup()
		return old, CopySpec{}, func() {}, err
	}
	updated := old
	refCopy := *actualRef
	updated.Ref = &refCopy
	updated.Resolved = resolved
	updated.Description = candidate.Description
	updated.Hash = candidate.Hash
	return updated, CopySpec{Source: candidate.Root, Destination: destination}, cleanup, nil
}

func (s Service) copySkill(source, destination string) error {
	if s.CopySkill != nil {
		return s.CopySkill(source, destination)
	}
	return AtomicCopy(source, destination)
}

func (s Service) rename(old, new string) error {
	if s.Rename != nil {
		return s.Rename(old, new)
	}
	return moveNoReplace(old, new)
}

func (s Service) syncDirectory(path string) error {
	if s.SyncDir != nil {
		return s.SyncDir(path)
	}
	return syncDirectory(path)
}

func (s Service) checkout(ctx context.Context, canonical, rawSource string, requested *config.Ref, allowCreate bool) (string, *config.Ref, string, func(), error) {
	if s.Runner == nil {
		return "", nil, "", func() {}, fmt.Errorf("git runner is required")
	}
	if err := os.MkdirAll(s.CacheRoot, 0o755); err != nil {
		return "", nil, "", func() {}, err
	}
	repository, err := s.ensureRepository(ctx, canonical, rawSource, allowCreate)
	if err != nil {
		return "", nil, "", func() {}, err
	}
	if _, err := s.Runner.Run(ctx, repository, "fetch", "--prune", "origin"); err != nil {
		return "", nil, "", func() {}, err
	}
	checkout, err := os.MkdirTemp(s.CacheRoot, "checkout-")
	if err != nil {
		return "", nil, "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(checkout) }
	if err := os.Remove(checkout); err != nil {
		cleanup()
		return "", nil, "", func() {}, err
	}
	if _, err := s.Runner.Run(ctx, s.CacheRoot, "clone", "--no-checkout", "--", repository, checkout); err != nil {
		cleanup()
		return "", nil, "", func() {}, err
	}

	ref := requested
	if ref == nil {
		output, err := s.Runner.Run(ctx, repository, "symbolic-ref", "--short", "HEAD")
		if err != nil {
			cleanup()
			return "", nil, "", func() {}, fmt.Errorf("resolve default branch: %w", err)
		}
		branch := strings.TrimSpace(output)
		branch = strings.TrimPrefix(branch, "origin/")
		ref = &config.Ref{Kind: "branch", Value: branch}
	}
	if err := validateRef(ref); err != nil {
		cleanup()
		return "", nil, "", func() {}, err
	}
	target := ref.Value
	switch ref.Kind {
	case "branch":
		target = "refs/remotes/origin/" + ref.Value
	case "tag":
		target = "refs/tags/" + ref.Value
	}
	if _, err := s.Runner.Run(ctx, checkout, "checkout", "--detach", target); err != nil {
		cleanup()
		return "", nil, "", func() {}, err
	}
	output, err := s.Runner.Run(ctx, checkout, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		cleanup()
		return "", nil, "", func() {}, err
	}
	resolved := strings.TrimSpace(output)
	if !fullObjectID.MatchString(resolved) {
		cleanup()
		return "", nil, "", func() {}, fmt.Errorf("git returned a non-full object id")
	}
	refCopy := *ref
	return checkout, &refCopy, strings.ToLower(resolved), cleanup, nil
}

func (s Service) ensureRepository(ctx context.Context, canonical, rawSource string, allowCreate bool) (string, error) {
	destination, err := RepoCachePath(s.CacheRoot, canonical)
	if err != nil {
		return "", err
	}
	repositories := filepath.Dir(destination)
	if err := ensurePrivateDirectory(repositories); err != nil {
		return "", err
	}
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		if err := ensurePrivateDirectory(destination); err != nil {
			return "", err
		}
		return destination, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if rawSource == "" {
		if !allowCreate {
			var reconstructErr error
			rawSource, reconstructErr = reconstructSource(canonical)
			if reconstructErr != nil {
				return "", reconstructErr
			}
		}
	}
	if rawSource == "" {
		return "", fmt.Errorf("git cache missing for %q; re-add the skill", canonical)
	}
	temporary, err := os.MkdirTemp(repositories, ".repo-")
	if err != nil {
		return "", err
	}
	_ = os.Remove(temporary)
	defer os.RemoveAll(temporary)
	if _, err := s.Runner.Run(ctx, repositories, "clone", "--mirror", "--", rawSource, temporary); err != nil {
		return "", err
	}
	if err := s.rename(temporary, destination); err != nil {
		if _, statErr := os.Stat(destination); statErr == nil {
			if privateErr := ensurePrivateDirectory(destination); privateErr != nil {
				return "", privateErr
			}
			return destination, nil
		}
		return "", err
	}
	if err := ensurePrivateDirectory(destination); err != nil {
		return "", err
	}
	if err := syncDirectory(repositories); err != nil {
		return "", err
	}
	return destination, nil
}

func lexicalLibraryPath(root, id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(absRoot, filepath.FromSlash(id))
	if !isWithin(absRoot, target) {
		return "", fmt.Errorf("library id %q escapes root", id)
	}
	return target, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path %q is not a safe directory", path)
	}
	return hardenPrivateDirectory(path)
}

func discoverSelection(checkout, sourcePath, skillName string, gitMode bool) ([]Candidate, error) {
	root := checkout
	if sourcePath != "" {
		if err := ValidateSourcePath(sourcePath); err != nil {
			return nil, err
		}
		root = filepath.Join(checkout, filepath.FromSlash(sourcePath))
		resolvedCheckout, err := filepath.EvalSymlinks(checkout)
		if err != nil {
			return nil, err
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, fmt.Errorf("resolve source_path %q: %w", sourcePath, err)
		}
		if !isWithin(resolvedCheckout, resolvedRoot) {
			return nil, fmt.Errorf("source_path %q escapes checkout", sourcePath)
		}
		root = resolvedRoot
	}
	var candidates []Candidate
	var err error
	if gitMode && sourcePath == "" {
		candidates, err = DiscoverGit(root)
	} else {
		candidates, err = Discover(root)
	}
	if err != nil {
		return nil, err
	}
	if sourcePath != "" {
		if len(candidates) != 1 || candidates[0].RelativePath != "." {
			return nil, fmt.Errorf("source_path %q is not a skill root", sourcePath)
		}
		candidates[0].RelativePath = filepath.ToSlash(sourcePath)
	}
	if skillName != "" {
		selected := candidates[:0]
		for _, candidate := range candidates {
			if candidate.Name == skillName || candidate.RelativePath == skillName {
				selected = append(selected, candidate)
			}
		}
		candidates = selected
		if len(candidates) == 0 {
			return nil, fmt.Errorf("skill %q not found", skillName)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].RelativePath < candidates[j].RelativePath })
	return candidates, nil
}

func selectCandidates(candidates []Candidate, selections []string) ([]Candidate, error) {
	if len(selections) == 0 {
		return candidates, nil
	}
	wanted := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		if selection == "" {
			return nil, fmt.Errorf("skill selection must not be empty")
		}
		wanted[selection] = struct{}{}
	}
	selected := make([]Candidate, 0, len(wanted))
	matched := make(map[string]struct{}, len(wanted))
	for _, candidate := range candidates {
		_, byName := wanted[candidate.Name]
		_, byPath := wanted[candidate.RelativePath]
		if !byName && !byPath {
			continue
		}
		selected = append(selected, candidate)
		if byName {
			matched[candidate.Name] = struct{}{}
		}
		if byPath {
			matched[candidate.RelativePath] = struct{}{}
		}
	}
	for selection := range wanted {
		if _, ok := matched[selection]; !ok {
			return nil, fmt.Errorf("skill %q not found", selection)
		}
	}
	return selected, nil
}

func validateRef(ref *config.Ref) error {
	if ref == nil || ref.Value == "" || strings.ContainsRune(ref.Value, '\x00') || strings.HasPrefix(ref.Value, "-") {
		return fmt.Errorf("invalid git ref")
	}
	switch ref.Kind {
	case "branch", "tag":
		return nil
	case "commit":
		if !fullObjectID.MatchString(ref.Value) {
			return fmt.Errorf("commit ref must be a full object id")
		}
		return nil
	default:
		return fmt.Errorf("invalid git ref kind %q", ref.Kind)
	}
}

func reconstructSource(canonical string) (string, error) {
	parts := strings.Split(canonical, "/")
	if len(parts) == 2 {
		return "https://github.com/" + canonical + ".git", nil
	}
	return "", fmt.Errorf("git cache missing for %q and transport is ambiguous; re-add the skill", canonical)
}
