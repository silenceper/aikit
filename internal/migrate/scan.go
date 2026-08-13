package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

type Dependencies struct {
	Store      config.Store
	Paths      config.Paths
	UserHome   string
	WorkingDir string
	Library    library.Service
	Recover    func(string, []config.PendingOperation, link.Selector, bool) link.Result
	Inspect    func(string, string) (link.State, error)
}

type Service struct{ deps Dependencies }

func New(deps Dependencies) *Service {
	if deps.Paths.Home == "" {
		deps.Paths = deps.Store.Paths
	}
	if deps.Store.Paths.Home == "" {
		deps.Store.Paths = deps.Paths
	}
	if deps.Library.LibraryRoot == "" {
		deps.Library = library.Service{LibraryRoot: deps.Paths.LibrarySkills, CacheRoot: deps.Paths.Cache, Runner: library.CommandRunner{}}
	}
	if deps.Recover == nil {
		deps.Recover = link.Recover
	}
	if deps.Inspect == nil {
		deps.Inspect = link.Inspect
	}
	if deps.WorkingDir == "" {
		deps.WorkingDir, _ = os.Getwd()
	}
	return &Service{deps: deps}
}

type scanRoot struct {
	origin      string
	agent       string
	project     string
	projectPath string
	path        string
}

type discovered struct {
	root      scanRoot
	target    string
	candidate library.Candidate
	managedID string
	allocated config.Skill
}

func (s *Service) Scan(ctx context.Context, request app.ScanRequest) (app.ScanResult, error) {
	if request.DryRun {
		return s.previewScan(ctx, request)
	}
	var result app.ScanResult
	err := s.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if err := s.recoverLibrary(ctx, tx.Config.Library.Skills); err != nil {
			return err
		}
		roots, err := s.scanRoots(tx.Config, request)
		if err != nil {
			return err
		}
		if request.Adopt {
			warnings, failed, recoverErr := s.recoverPendingForRoots(tx, roots)
			result.Warnings = append(result.Warnings, warnings...)
			if recoverErr != nil {
				return recoverErr
			}
			if failed {
				result.Exit = app.ExitPartial
				return nil
			}
		}
		found, warnings := s.discoverRoots(roots)
		result.Warnings = append(result.Warnings, warnings...)
		if len(warnings) > 0 {
			result.Exit = app.ExitPartial
		}
		found, err = allocateDiscovered(found, tx.Config.Library.Skills)
		if err != nil {
			return err
		}
		for _, item := range found {
			if !selectedScanItem(item, request.Skills) || !selectedScanTarget(item, request.Targets) {
				continue
			}
			if item.managedID != "" && !request.Adopt {
				continue
			}
			scanItem, itemErr := s.importDiscovered(ctx, tx, item, request.Adopt)
			if itemErr != nil {
				scanItem.Error = itemErr.Error()
				result.Exit = app.ExitPartial
			}
			result.Items = append(result.Items, scanItem)
		}
		return nil
	})
	return result, err
}

// previewScan intentionally avoids WithLock, recovery, checkpoints, and
// library mutations. It only reads the ledger and discovered source paths.
func (s *Service) previewScan(ctx context.Context, request app.ScanRequest) (app.ScanResult, error) {
	cfg, err := s.deps.Store.Load(ctx)
	if err != nil {
		return app.ScanResult{}, err
	}
	roots, err := s.scanRoots(cfg, request)
	if err != nil {
		return app.ScanResult{}, err
	}
	found, warnings := s.discoverRoots(roots)
	found, err = allocateDiscovered(found, cfg.Library.Skills)
	if err != nil {
		return app.ScanResult{}, err
	}
	result := app.ScanResult{Warnings: warnings}
	if len(warnings) > 0 {
		result.Exit = app.ExitPartial
	}
	for _, item := range found {
		if !selectedScanItem(item, request.Skills) || !selectedScanTarget(item, request.Targets) {
			continue
		}
		skill := item.allocated
		if item.managedID != "" {
			var ok bool
			skill, ok = findSkill(cfg, item.managedID)
			if !ok {
				result.Items = append(result.Items, app.ScanItem{Origin: item.root.origin, Target: item.target, Error: fmt.Sprintf("managed link references unknown skill %q", item.managedID)})
				result.Exit = app.ExitPartial
				continue
			}
		}
		result.Items = append(result.Items, app.ScanItem{Origin: item.root.origin, Target: item.target, Skill: skill})
	}
	return result, nil
}

func (s *Service) scanRoots(cfg *config.Config, request app.ScanRequest) ([]scanRoot, error) {
	var agents []agent.Agent
	if request.Agent != "" {
		item, ok := agent.ByName(request.Agent)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q", request.Agent)
		}
		agents = []agent.Agent{item}
	} else {
		agents = agent.All()
	}
	roots := make([]scanRoot, 0, len(agents)*2)
	for _, item := range agents {
		roots = append(roots, scanRoot{origin: "g/" + item.Name(), agent: item.Name(), path: item.GlobalSkillDir(s.deps.UserHome)})
	}
	project, ok, err := s.selectedProject(cfg, request.Project)
	if err != nil {
		return nil, err
	}
	if ok {
		declared := make(map[string]struct{}, len(project.Agents))
		for _, name := range project.Agents {
			declared[name] = struct{}{}
		}
		for _, item := range agents {
			if _, exists := declared[item.Name()]; !exists {
				continue
			}
			roots = append(roots, scanRoot{
				origin: "p/" + project.Name + "/" + item.Name(), agent: item.Name(), project: project.Name,
				projectPath: project.Path, path: item.ProjectSkillDir(project.Path),
			})
		}
	}
	return roots, nil
}

func (s *Service) selectedProject(cfg *config.Config, requested string) (config.Project, bool, error) {
	if requested != "" {
		for _, project := range cfg.Projects {
			if project.Name == requested {
				return project, true, nil
			}
		}
		return config.Project{}, false, fmt.Errorf("unknown project %q", requested)
	}
	cwd, err := canonicalPath(s.deps.WorkingDir)
	if err != nil {
		return config.Project{}, false, nil
	}
	for _, project := range cfg.Projects {
		path, pathErr := canonicalPath(project.Path)
		if pathErr == nil && path == cwd {
			return project, true, nil
		}
	}
	return config.Project{}, false, nil
}

func (s *Service) discoverRoots(roots []scanRoot) ([]discovered, []string) {
	var found []discovered
	var warnings []string
	for _, root := range roots {
		entries, err := os.ReadDir(root.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("scan %s: %v", root.path, err))
			continue
		}
		for _, entry := range entries {
			target := filepath.Join(root.path, entry.Name())
			state, inspectErr := s.deps.Inspect(target, s.deps.Paths.LibrarySkills)
			if inspectErr != nil {
				warnings = append(warnings, fmt.Sprintf("inspect %s: %v", target, inspectErr))
				continue
			}
			if state.Kind == link.StateManagedLink {
				found = append(found, discovered{root: root, target: target, managedID: state.SkillID})
				continue
			}
			if state.Kind != link.StateDirectory && state.Kind != link.StateExternalLink {
				continue
			}
			candidates, discoverErr := library.Discover(target)
			if discoverErr != nil || len(candidates) != 1 {
				if discoverErr == nil {
					discoverErr = fmt.Errorf("expected one skill, found %d", len(candidates))
				}
				warnings = append(warnings, fmt.Sprintf("discover %s: %v", target, discoverErr))
				continue
			}
			found = append(found, discovered{root: root, target: target, candidate: candidates[0]})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].root.origin != found[j].root.origin {
			return originLess(found[i].root.origin, found[j].root.origin)
		}
		return found[i].target < found[j].target
	})
	return found, warnings
}

func (s *Service) recoverPendingForRoots(tx *config.Tx, roots []scanRoot) ([]string, bool, error) {
	seen := map[string]struct{}{}
	var warnings []string
	var completed []string
	failed := false
	for _, root := range roots {
		key := root.project + "\x00" + root.agent
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result := s.deps.Recover(s.deps.Paths.LibrarySkills, tx.Config.PendingOperations, link.Selector{Project: root.project, Agent: root.agent}, false)
		completed = append(completed, result.Completed...)
		for _, issue := range result.Failures {
			failed = true
			warnings = append(warnings, fmt.Sprintf("recover %s: %s", issue.Path, issue.Message))
		}
		for _, issue := range result.Issues {
			failed = true
			warnings = append(warnings, fmt.Sprintf("recover %s: %s", issue.Path, issue.Message))
		}
	}
	if len(completed) > 0 {
		removeCompleted(tx.Config, completed)
		if err := tx.Checkpoint(); err != nil {
			return warnings, true, err
		}
	}
	return warnings, failed, nil
}

func allocateDiscovered(found []discovered, existing []config.Skill) ([]discovered, error) {
	inputs := make([]library.LocalCandidate, 0, len(found))
	indexes := make([]int, 0, len(found))
	for index := range found {
		if found[index].managedID != "" {
			continue
		}
		inputs = append(inputs, library.LocalCandidate{Origin: found[index].root.origin, Candidate: found[index].candidate})
		indexes = append(indexes, index)
	}
	allocations, err := library.AllocateLocal(inputs, existing)
	if err != nil {
		return nil, err
	}
	for i, allocation := range allocations {
		found[indexes[i]].allocated = allocation.Skill
	}
	return found, nil
}

func selectedScanItem(item discovered, selected []string) bool {
	if len(selected) == 0 {
		return true
	}
	name, id := item.candidate.Name, item.allocated.ID
	if item.managedID != "" {
		id = item.managedID
		name = filepath.Base(item.target)
	}
	for _, value := range selected {
		if value == name || value == id {
			return true
		}
	}
	return false
}

func selectedScanTarget(item discovered, selected []string) bool {
	if len(selected) == 0 {
		return true
	}
	target := filepath.Clean(item.target)
	for _, value := range selected {
		if filepath.Clean(value) == target {
			return true
		}
	}
	return false
}

func originLess(left, right string) bool {
	allocations, err := library.AllocateLocal([]library.LocalCandidate{
		{Origin: left, Candidate: library.Candidate{Name: "left", Hash: "000000000000"}},
		{Origin: right, Candidate: library.Candidate{Name: "right", Hash: "111111111111"}},
	}, nil)
	return err == nil && len(allocations) == 2 && allocations[0].Origin == left
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func (s *Service) recoverLibrary(ctx context.Context, ledger []config.Skill) error {
	if err := os.MkdirAll(s.deps.Paths.LibrarySkills, 0o755); err != nil {
		return err
	}
	issues, err := s.deps.Library.RecoverBatches(ctx, ledger)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return fmt.Errorf("library recovery requires attention: %s", issues[0].Detail)
	}
	return nil
}

func validateEffective(cfg *config.Config) error {
	issues := scope.ValidateEffective(cfg)
	if len(issues) > 0 {
		return fmt.Errorf("effective scope conflict: %s", issues[0].Message)
	}
	return nil
}
