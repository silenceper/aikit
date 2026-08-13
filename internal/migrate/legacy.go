package migrate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
	"gopkg.in/yaml.v3"
)

type legacyAsset struct {
	Source string `yaml:"source"`
	Name   string `yaml:"name"`
}

type legacyCatalog struct {
	Skills []legacyAsset `yaml:"skills"`
}

type legacyProject struct {
	Project struct {
		Name    string   `yaml:"name"`
		Targets []string `yaml:"targets"`
	} `yaml:"project"`
	Assets struct {
		Skills []legacyAsset `yaml:"skills"`
	} `yaml:"assets"`
}

type legacyCandidate struct {
	asset     legacyAsset
	root      string
	candidate library.Candidate
	canonical string
}

func (s *Service) Migrate(ctx context.Context, request app.MigrateRequest) (app.MigrateResult, error) {
	if request.DryRun {
		cfg, err := s.deps.Store.Load(ctx)
		if err != nil {
			return app.MigrateResult{}, err
		}
		return s.previewMigration(cfg, request), nil
	}
	var result app.MigrateResult
	err := s.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := s.recoverLibrary(ctx, tx.Config.Library.Skills); err != nil {
			return err
		}
		catalog, err := readLegacyCatalog(filepath.Join(s.deps.Paths.Home, "catalog.yaml"))
		if err != nil {
			return err
		}
		for _, asset := range catalog.Skills {
			_, imported, itemErr := s.importLegacySkill(ctx, tx, asset, nil)
			summarizeMigration(&result, imported, itemErr)
		}
		paths, pathWarnings := s.legacyProjectPaths(request.ProjectPaths)
		result.Warnings = append(result.Warnings, pathWarnings...)
		result.Failed += len(pathWarnings)
		for _, path := range paths {
			project, err := readLegacyProject(filepath.Join(path, ".aikit.yaml"))
			if err != nil {
				result.Failed++
				result.Warnings = append(result.Warnings, fmt.Sprintf("read %s: %v", path, err))
				continue
			}
			beforeProject := cloneConfig(tx.Config)
			index, err := mergeLegacyProject(tx.Config, path, project)
			if err != nil {
				*tx.Config = *beforeProject
				result.Failed++
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			if err := validateMutation(tx.Config); err != nil {
				*tx.Config = *beforeProject
				result.Failed++
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			if err := tx.Checkpoint(); err != nil {
				return err
			}
			projectRoots := rootsForProject(tx.Config.Projects[index])
			if request.Adopt {
				warnings, failed, recoverErr := s.recoverPendingForRoots(tx, projectRoots)
				result.Warnings = append(result.Warnings, warnings...)
				if recoverErr != nil {
					return recoverErr
				}
				if failed {
					result.Failed++
					continue
				}
			}
			for _, asset := range project.Assets.Skills {
				bindingAdded := false
				skill, imported, itemErr := s.importLegacySkill(ctx, tx, asset, func(cfg *config.Config, id string) {
					binding := &cfg.Projects[index].Binding
					before := len(binding.Skills)
					binding.Skills = appendUnique(binding.Skills, id)
					bindingAdded = len(binding.Skills) != before
				})
				if itemErr == nil && !imported && bindingAdded {
					// Reusing a library entry while adding its project reference is a
					// successful merge, not a skipped migration entry.
					imported = true
				}
				summarizeMigration(&result, imported, itemErr)
				if itemErr == nil && request.Adopt {
					failures, warnings := s.adoptMigratedProjectSkill(tx, cfgProject(tx.Config, index), skill)
					result.Failed += failures
					result.Warnings = append(result.Warnings, warnings...)
				} else if itemErr == nil {
					pending, failures, warnings := s.pendingAdoptSummary(cfgProject(tx.Config, index), skill)
					result.PendingAdopt += pending
					result.Failed += failures
					result.Warnings = append(result.Warnings, warnings...)
				}
			}
		}
		return nil
	})
	if result.Failed > 0 {
		result.Exit = app.ExitPartial
	}
	return result, err
}

func (s *Service) previewMigration(cfg *config.Config, request app.MigrateRequest) app.MigrateResult {
	result := app.MigrateResult{}
	catalog, err := readLegacyCatalog(filepath.Join(s.deps.Paths.Home, "catalog.yaml"))
	if err != nil {
		result.Failed++
		result.Warnings = append(result.Warnings, err.Error())
	}
	for _, asset := range catalog.Skills {
		candidate, itemErr := s.resolveLegacyCandidate(asset)
		if itemErr != nil {
			result.Failed++
			result.Warnings = append(result.Warnings, itemErr.Error())
			continue
		}
		id, idErr := candidateID(candidate, cfg.Library.Skills)
		if idErr != nil {
			result.Failed++
			result.Warnings = append(result.Warnings, idErr.Error())
			continue
		}
		if existing, ok := findSkill(cfg, id); ok {
			if existing.Hash == candidate.candidate.Hash && existing.SourcePath == candidate.candidate.RelativePath {
				result.Skipped++
			} else {
				result.Failed++
				result.Warnings = append(result.Warnings, fmt.Sprintf("skill %q conflicts with existing ledger", id))
			}
			continue
		}
		result.Imported++
	}
	paths, warnings := s.legacyProjectPaths(request.ProjectPaths)
	result.Warnings = append(result.Warnings, warnings...)
	result.Failed += len(warnings)
	for _, path := range paths {
		project, readErr := readLegacyProject(filepath.Join(path, ".aikit.yaml"))
		if readErr != nil {
			result.Failed++
			result.Warnings = append(result.Warnings, readErr.Error())
			continue
		}
		for _, asset := range project.Assets.Skills {
			candidate, itemErr := s.resolveLegacyCandidate(asset)
			if itemErr != nil {
				result.Failed++
				result.Warnings = append(result.Warnings, itemErr.Error())
				continue
			}
			id, idErr := candidateID(candidate, cfg.Library.Skills)
			if idErr != nil {
				result.Failed++
				result.Warnings = append(result.Warnings, idErr.Error())
				continue
			}
			if _, ok := findSkill(cfg, id); ok {
				result.Skipped++
			} else {
				result.Imported++
			}
		}
	}
	if result.Failed > 0 {
		result.Exit = app.ExitPartial
	}
	return result
}

func (s *Service) importLegacySkill(ctx context.Context, tx *config.Tx, asset legacyAsset, bind func(*config.Config, string)) (config.Skill, bool, error) {
	candidate, err := s.resolveLegacyCandidate(asset)
	if err != nil {
		return config.Skill{}, false, err
	}
	id, err := candidateID(candidate, tx.Config.Library.Skills)
	if err != nil {
		return config.Skill{}, false, err
	}
	if existing, ok := findSkill(tx.Config, id); ok {
		conflict := existing.Hash != candidate.candidate.Hash
		if candidate.canonical != "" {
			conflict = conflict || existing.SourcePath != candidate.candidate.RelativePath
		}
		if conflict {
			return config.Skill{}, false, fmt.Errorf("skill %q conflicts with existing source_path or content", id)
		}
		before := cloneConfig(tx.Config)
		if bind != nil {
			bind(tx.Config, existing.ID)
		}
		if err := validateMutation(tx.Config); err != nil {
			*tx.Config = *before
			return config.Skill{}, false, err
		}
		if bind != nil {
			if err := tx.Checkpoint(); err != nil {
				*tx.Config = *before
				return config.Skill{}, false, err
			}
		}
		return existing, false, nil
	}

	if candidate.canonical != "" {
		if err := s.seedRepository(ctx, candidate.canonical, candidate.root); err != nil {
			return config.Skill{}, false, err
		}
	}
	var mutation *library.Mutation
	if candidate.canonical == "" {
		mutation, err = s.deps.Library.PrepareLocal(ctx, candidate.candidate.Root, nil, tx.Config.Library.Skills)
	} else {
		mutation, err = s.deps.Library.PrepareGit(ctx, asset.Source, library.GitAddOptions{
			SourcePath: candidate.candidate.RelativePath, Skills: []string{asset.Name}, Existing: tx.Config.Library.Skills,
		})
	}
	if err != nil {
		return config.Skill{}, false, err
	}
	checkpointed := false
	defer func() {
		if !checkpointed {
			_ = mutation.Abort()
		}
	}()
	if len(mutation.Skills) != 1 {
		return config.Skill{}, false, fmt.Errorf("legacy source %q prepared %d skills", asset.Source, len(mutation.Skills))
	}
	skill := mutation.Skills[0]
	if skill.ID != id || skill.Hash != candidate.candidate.Hash {
		return config.Skill{}, false, fmt.Errorf("legacy source %q changed while preparing", asset.Source)
	}
	before := cloneConfig(tx.Config)
	appendSkill(tx.Config, skill)
	if bind != nil {
		bind(tx.Config, skill.ID)
	}
	if err := validateMutation(tx.Config); err != nil {
		*tx.Config = *before
		return config.Skill{}, false, err
	}
	if err := tx.Checkpoint(); err != nil {
		*tx.Config = *before
		return config.Skill{}, false, err
	}
	checkpointed = true
	if err := mutation.Commit(ctx); err != nil {
		commitErr := err
		*tx.Config = *before
		if restoreErr := tx.Checkpoint(); restoreErr != nil {
			return config.Skill{}, false, fmt.Errorf("commit migrated skill: %v; restore ledger: %w", commitErr, restoreErr)
		}
		if recoverErr := s.recoverLibrary(ctx, tx.Config.Library.Skills); recoverErr != nil {
			return config.Skill{}, false, fmt.Errorf("commit migrated skill: %v; recover library: %w", commitErr, recoverErr)
		}
		return config.Skill{}, false, commitErr
	}
	return skill, true, nil
}

func (s *Service) resolveLegacyCandidate(asset legacyAsset) (legacyCandidate, error) {
	if asset.Name == "" || asset.Source == "" {
		return legacyCandidate{}, fmt.Errorf("legacy skill requires source and name")
	}
	root := ""
	canonical := ""
	if asset.Source == "_local" {
		root = filepath.Join(s.deps.Paths.Home, "skills", asset.Name)
	} else {
		var err error
		canonical, err = library.NormalizeSource(asset.Source)
		if err != nil {
			return legacyCandidate{}, err
		}
		root, err = legacyCachePath(s.deps.Paths.Cache, asset.Source, canonical)
		if err != nil {
			return legacyCandidate{}, err
		}
	}
	candidates, err := library.DiscoverGit(root)
	if err != nil {
		return legacyCandidate{}, fmt.Errorf("discover legacy source %q: %w", asset.Source, err)
	}
	var matches []library.Candidate
	for _, candidate := range candidates {
		if candidate.Name == asset.Name {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return legacyCandidate{}, fmt.Errorf("legacy source %q has %d matches for skill %q", asset.Source, len(matches), asset.Name)
	}
	return legacyCandidate{asset: asset, root: root, candidate: matches[0], canonical: canonical}, nil
}

func candidateID(candidate legacyCandidate, existing []config.Skill) (string, error) {
	if candidate.canonical == "" {
		allocations, err := library.AllocateLocal([]library.LocalCandidate{{Origin: "legacy/" + candidate.candidate.Name, Candidate: candidate.candidate}}, existing)
		if err != nil {
			return "", err
		}
		return allocations[0].Skill.ID, nil
	}
	return candidate.canonical + "/" + candidate.candidate.Name, nil
}

func legacyCachePath(cacheRoot, raw, canonical string) (string, error) {
	rel := canonical
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		parts := strings.FieldsFunc(strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git"), func(r rune) bool { return r == '/' })
		if len(parts) >= 2 {
			repository := strings.Join(parts[len(parts)-2:], "/")
			if parsed.Hostname() == "github.com" {
				rel = repository
			} else {
				rel = parsed.Hostname() + "/" + repository
			}
		}
	}
	target := filepath.Join(cacheRoot, filepath.FromSlash(rel))
	absRoot, err := filepath.Abs(cacheRoot)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	relCheck, err := filepath.Rel(absRoot, absTarget)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("legacy cache path escapes cache root")
	}
	return absTarget, nil
}

func (s *Service) seedRepository(ctx context.Context, canonical, legacyRoot string) error {
	repository, err := library.RepoCachePath(s.deps.Paths.Cache, canonical)
	if err != nil {
		return err
	}
	if info, err := os.Stat(repository); err == nil && info.IsDir() {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(repository), 0o700); err != nil {
		return err
	}
	_, err = s.deps.Library.Runner.Run(ctx, s.deps.Paths.Cache, "clone", "--mirror", "--", legacyRoot, repository)
	return err
}

func readLegacyCatalog(path string) (legacyCatalog, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return legacyCatalog{}, nil
	}
	if err != nil {
		return legacyCatalog{}, err
	}
	var catalog legacyCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return legacyCatalog{}, fmt.Errorf("decode legacy catalog: %w", err)
	}
	return catalog, nil
}

func readLegacyProject(path string) (legacyProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return legacyProject{}, err
	}
	var project legacyProject
	if err := yaml.Unmarshal(data, &project); err != nil {
		return legacyProject{}, fmt.Errorf("decode legacy project: %w", err)
	}
	return project, nil
}

func (s *Service) legacyProjectPaths(explicit []string) ([]string, []string) {
	values := append([]string(nil), explicit...)
	if _, err := os.Stat(filepath.Join(s.deps.WorkingDir, ".aikit.yaml")); err == nil {
		values = append(values, s.deps.WorkingDir)
	}
	seen := map[string]struct{}{}
	var result, warnings []string
	for _, value := range values {
		path, err := canonicalPath(value)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("project path %q: %v", value, err))
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result, warnings
}

func mergeLegacyProject(cfg *config.Config, path string, legacy legacyProject) (int, error) {
	canonical, err := canonicalPath(path)
	if err != nil {
		return -1, err
	}
	name := strings.TrimSpace(legacy.Project.Name)
	if name == "" {
		name = filepath.Base(canonical)
	}
	for index := range cfg.Projects {
		existingPath, pathErr := canonicalPath(cfg.Projects[index].Path)
		if pathErr == nil && existingPath == canonical {
			mergeProjectAgents(&cfg.Projects[index], legacy.Project.Targets)
			return index, nil
		}
		if cfg.Projects[index].Name == name {
			return -1, fmt.Errorf("legacy project %q conflicts with registered path %q", name, cfg.Projects[index].Path)
		}
	}
	project := config.Project{Name: name, Path: canonical, AgentBindings: map[string]config.Binding{}}
	mergeProjectAgents(&project, legacy.Project.Targets)
	cfg.Projects = append(cfg.Projects, project)
	return len(cfg.Projects) - 1, nil
}

func mergeProjectAgents(project *config.Project, legacy []string) {
	for _, name := range legacy {
		name = agent.NormalizeLegacyName(name)
		if _, ok := agent.ByName(name); ok {
			project.Agents = appendUnique(project.Agents, name)
		}
	}
	if len(project.Agents) == 0 {
		project.Agents = []string{"cursor"}
	}
	if project.AgentBindings == nil {
		project.AgentBindings = map[string]config.Binding{}
	}
}

func (s *Service) adoptMigratedProjectSkill(tx *config.Tx, project config.Project, skill config.Skill) (int, []string) {
	var failures int
	var warnings []string
	for _, name := range project.Agents {
		item, ok := agent.ByName(name)
		if !ok {
			continue
		}
		target := filepath.Join(item.ProjectSkillDir(project.Path), skill.Name)
		state, err := s.deps.Inspect(target, s.deps.Paths.LibrarySkills)
		if err != nil {
			failures++
			warnings = append(warnings, fmt.Sprintf("inspect %s: %v", target, err))
			continue
		}
		if state.Kind == link.StateAbsent || (state.Kind == link.StateManagedLink && state.SkillID == skill.ID) {
			continue
		}
		if state.Kind == link.StateManagedLink {
			failures++
			warnings = append(warnings, fmt.Sprintf("adopt %s: managed by %s", target, state.SkillID))
			continue
		}
		op, err := link.NewAdoptOperation("", config.Scope{Project: project.Name, ProjectPath: project.Path, Agent: name}, target, skill.ID)
		if err != nil {
			failures++
			warnings = append(warnings, err.Error())
			continue
		}
		tx.Config.PendingOperations = append(tx.Config.PendingOperations, op)
		if err := tx.Checkpoint(); err != nil {
			failures++
			warnings = append(warnings, err.Error())
			continue
		}
		recovered := s.deps.Recover(s.deps.Paths.LibrarySkills, []config.PendingOperation{op}, link.Selector{Project: project.Name, Agent: name}, false)
		removeCompleted(tx.Config, recovered.Completed)
		if err := tx.Checkpoint(); err != nil {
			failures++
			warnings = append(warnings, err.Error())
			continue
		}
		if len(recovered.Failures) > 0 {
			failures++
			warnings = append(warnings, recovered.Failures[0].Message)
		} else if len(recovered.Issues) > 0 {
			failures++
			warnings = append(warnings, recovered.Issues[0].Message)
		}
	}
	return failures, warnings
}

func rootsForProject(project config.Project) []scanRoot {
	result := make([]scanRoot, 0, len(project.Agents))
	for _, name := range project.Agents {
		item, ok := agent.ByName(name)
		if !ok {
			continue
		}
		result = append(result, scanRoot{
			origin: "p/" + project.Name + "/" + name, agent: name, project: project.Name,
			projectPath: project.Path, path: item.ProjectSkillDir(project.Path),
		})
	}
	return result
}

func (s *Service) pendingAdoptSummary(project config.Project, skill config.Skill) (int, int, []string) {
	var pending, failures int
	var warnings []string
	for _, name := range project.Agents {
		item, ok := agent.ByName(name)
		if !ok {
			continue
		}
		target := filepath.Join(item.ProjectSkillDir(project.Path), skill.Name)
		state, err := s.deps.Inspect(target, s.deps.Paths.LibrarySkills)
		if err != nil {
			failures++
			warnings = append(warnings, fmt.Sprintf("inspect %s: %v", target, err))
			continue
		}
		switch {
		case state.Kind == link.StateAbsent:
		case state.Kind == link.StateManagedLink && state.SkillID == skill.ID:
		case state.Kind == link.StateManagedLink:
			failures++
			warnings = append(warnings, fmt.Sprintf("adopt %s: managed by %s", target, state.SkillID))
		default:
			pending++
		}
	}
	return pending, failures, warnings
}

func cfgProject(cfg *config.Config, index int) config.Project { return cfg.Projects[index] }

func summarizeMigration(result *app.MigrateResult, imported bool, err error) {
	if err != nil {
		result.Failed++
		result.Warnings = append(result.Warnings, err.Error())
		return
	}
	if imported {
		result.Imported++
	} else {
		result.Skipped++
	}
}

var _ app.MigrationService = (*Service)(nil)
