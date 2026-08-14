package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	Store                    config.Store
	Paths                    config.Paths
	UserHome                 string
	WorkingDir               string
	Library                  library.Service
	Recover                  func(string, []config.PendingOperation, link.Selector, bool) link.Result
	Inspect                  func(string, string) (link.State, error)
	InventoryInspect         func(context.Context, string, string) (link.State, error)
	BeforeMutationValidation func()
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
	if deps.InventoryInspect == nil {
		inspect := deps.Inspect
		deps.InventoryInspect = func(ctx context.Context, path, libraryRoot string) (link.State, error) {
			if err := ctx.Err(); err != nil {
				return link.State{}, err
			}
			state, err := inspect(path, libraryRoot)
			if err != nil {
				return link.State{}, err
			}
			if err := ctx.Err(); err != nil {
				return link.State{}, err
			}
			return state, nil
		}
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
	root         scanRoot
	rootInfo     os.FileInfo
	target       string
	targetInfo   os.FileInfo
	linkState    link.State
	pendingIssue string
	candidate    library.Candidate
	managedID    string
	allocated    config.Skill
}

func (s *Service) Scan(ctx context.Context, request app.ScanRequest) (app.ScanResult, error) {
	if request.DryRun {
		return s.previewScan(ctx, request)
	}
	var result app.ScanResult
	err := s.deps.Store.WithLock(ctx, func(tx *config.Tx) error {
		if len(tx.Config.PendingOperations) > 0 {
			return fmt.Errorf("pending recovery must be resolved before inventory mutation")
		}
		roots, err := s.scanRoots(tx.Config, request)
		if err != nil {
			return err
		}
		found, warnings, issues := s.discoverRoots(roots)
		result.Warnings = append(result.Warnings, warnings...)
		result.Issues = append(result.Issues, issues...)
		if len(warnings) > 0 {
			result.Exit = app.ExitPartial
		}
		found = s.appendPendingDiscovered(tx.Config, roots, found)
		found, err = allocateDiscovered(found, tx.Config.Library.Skills)
		if err != nil {
			return err
		}
		selected, err := selectDiscovered(found, request)
		if err != nil {
			return err
		}
		if err := s.validateSelectorSnapshots(tx.Config, selected, request); err != nil {
			return err
		}
		if len(selected) > 0 && s.deps.BeforeMutationValidation != nil {
			s.deps.BeforeMutationValidation()
		}
		valid := make([]discovered, 0, len(selected))
		for _, item := range selected {
			if item.managedID != "" && !request.Adopt {
				continue
			}
			preview := s.inventoryItem(tx.Config, item, request.Adopt)
			if len(request.Selectors) > 0 && (preview.Action == app.ScanActionConflict || preview.State == app.ScanStateBrokenLink || preview.State == app.ScanStateDrifted || preview.State == app.ScanStateError) {
				preview.Error = fmt.Sprintf("inventory item %s is not safe to mutate in state %s", preview.Target, preview.State)
				preview.Issues = append(preview.Issues, app.ScanIssue{State: preview.State, Origin: preview.Origin, Path: preview.Target, Message: preview.Error})
				result.Items = append(result.Items, preview)
				result.Exit = app.ExitPartial
				continue
			}
			if err := s.revalidateDiscovered(item); err != nil {
				preview.Error = err.Error()
				preview.State = app.ScanStateError
				preview.DiagnosticState = app.ScanStateError
				preview.Issues = append(preview.Issues, app.ScanIssue{State: preview.State, Origin: preview.Origin, Path: preview.Target, Message: preview.Error})
				result.Items = append(result.Items, preview)
				result.Exit = app.ExitPartial
				continue
			}
			valid = append(valid, item)
		}
		preflightOutputs := make([]app.ScanItem, len(valid))
		if request.Adopt {
			var preflightErr error
			preflightOutputs, preflightErr = s.preflightAdoptBatch(tx.Config, valid)
			if preflightErr != nil {
				result.Items = append(result.Items, markScanBatchError(preflightOutputs, preflightErr)...)
				result.Exit = app.ExitPartial
				return nil
			}
		} else {
			for index, item := range valid {
				preflightOutputs[index] = s.inventoryItem(tx.Config, item, false)
			}
		}
		if len(valid) > 0 {
			if err := s.recoverLibrary(ctx, tx.Config.Library.Skills); err != nil {
				return err
			}
		}
		for index, item := range valid {
			preview := preflightOutputs[index]
			scanItem, itemErr := s.importDiscovered(ctx, tx, item, request.Adopt)
			scanItem = mergeScanItem(preview, scanItem)
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
	found, warnings, issues := s.discoverRoots(roots)
	found = s.appendPendingDiscovered(cfg, roots, found)
	found, err = allocateDiscovered(found, cfg.Library.Skills)
	if err != nil {
		return app.ScanResult{}, err
	}
	result := app.ScanResult{Warnings: warnings, Issues: issues}
	if len(warnings) > 0 {
		result.Exit = app.ExitPartial
	}
	selected, err := selectDiscovered(found, request)
	if err != nil {
		return app.ScanResult{}, err
	}
	if err := s.validateSelectorSnapshots(cfg, selected, request); err != nil {
		return app.ScanResult{}, err
	}
	if request.Adopt {
		outputs, validationErr := s.preflightAdoptBatch(cfg, selected)
		if validationErr != nil {
			if !scanBatchContainsUnsafeState(outputs) {
				outputs = markScanBatchError(outputs, validationErr)
			}
			result.Exit = app.ExitPartial
		}
		result.Items = append(result.Items, outputs...)
		return result, nil
	}
	for _, item := range selected {
		result.Items = append(result.Items, s.inventoryItem(cfg, item, false))
	}
	return result, nil
}

func scanBatchContainsUnsafeState(items []app.ScanItem) bool {
	for _, item := range items {
		switch item.State {
		case app.ScanStateBrokenLink, app.ScanStateDrifted, app.ScanStatePendingRecovery, app.ScanStateError, app.ScanStateNameConflict:
			return true
		}
	}
	return false
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
	roots := make([]scanRoot, 0, len(agents)*(len(cfg.Projects)+1))
	for _, item := range agents {
		roots = append(roots, scanRoot{origin: "g/" + item.Name(), agent: item.Name(), path: item.GlobalSkillDir(s.deps.UserHome)})
	}
	var projects []config.Project
	if request.AllProjects && request.Project == "" {
		projects = append(projects, cfg.Projects...)
		sort.SliceStable(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	} else {
		project, ok, err := s.selectedProject(cfg, request.Project)
		if err != nil {
			return nil, err
		}
		if ok {
			projects = []config.Project{project}
		}
	}
	for _, project := range projects {
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

func (s *Service) discoverRoots(roots []scanRoot) ([]discovered, []string, []app.ScanIssue) {
	var found []discovered
	var warnings []string
	var issues []app.ScanIssue
	for _, root := range roots {
		rootInfo, err := s.validateScanRoot(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
			continue
		}
		entries, err := os.ReadDir(root.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
			continue
		}
		for _, entry := range entries {
			target := filepath.Join(root.path, entry.Name())
			if err := revalidatePathIdentity(root.path, rootInfo, "inventory root"); err != nil {
				appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
				break
			}
			targetInfo, statErr := os.Lstat(target)
			if statErr != nil {
				appendScanIssue(&warnings, &issues, root, target, "inspect", statErr)
				continue
			}
			state, inspectErr := s.deps.Inspect(target, s.deps.Paths.LibrarySkills)
			if inspectErr != nil {
				appendScanIssue(&warnings, &issues, root, target, "inspect", inspectErr)
				continue
			}
			if err := revalidatePathIdentity(root.path, rootInfo, "inventory root"); err != nil {
				appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
				break
			}
			if err := revalidatePathIdentity(target, targetInfo, "inventory target"); err != nil {
				appendScanIssue(&warnings, &issues, root, target, "inspect", err)
				continue
			}
			if state.Kind == link.StateManagedLink {
				found = append(found, discovered{root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo, linkState: state, managedID: state.SkillID})
				continue
			}
			if state.Kind == link.StateExternalLink && state.Broken {
				found = append(found, discovered{root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo, linkState: state})
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
				appendScanIssue(&warnings, &issues, root, target, "discover", discoverErr)
				continue
			}
			if err := revalidatePathIdentity(root.path, rootInfo, "inventory root"); err != nil {
				appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
				break
			}
			if err := revalidatePathIdentity(target, targetInfo, "inventory target"); err != nil {
				appendScanIssue(&warnings, &issues, root, target, "discover", err)
				continue
			}
			found = append(found, discovered{root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo, linkState: state, candidate: candidates[0]})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].root.origin != found[j].root.origin {
			return originLess(found[i].root.origin, found[j].root.origin)
		}
		return found[i].target < found[j].target
	})
	return found, warnings, issues
}

func appendScanIssue(warnings *[]string, issues *[]app.ScanIssue, root scanRoot, path, operation string, err error) {
	message := fmt.Sprintf("%s %s: %v", operation, path, err)
	*warnings = append(*warnings, message)
	*issues = append(*issues, app.ScanIssue{State: app.ScanStateError, Origin: root.origin, Path: path, Message: message})
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
		if found[index].managedID != "" || found[index].candidate.Root == "" {
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

func (s *Service) validateScanRoot(root scanRoot) (os.FileInfo, error) {
	if !filepath.IsAbs(root.path) || filepath.Clean(root.path) != root.path {
		return nil, fmt.Errorf("inventory root must be a clean absolute path")
	}
	target := link.Target{Scope: itemScope(root), Home: s.deps.UserHome, Dir: root.path}
	if err := link.ValidateTarget(target); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root.path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("inventory root must not be a symlink")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("inventory root is not a directory")
	}
	return info, nil
}

func revalidatePathIdentity(path string, expected os.FileInfo, label string) error {
	if expected == nil {
		return fmt.Errorf("%s has no discovery identity", label)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s changed: %w", label, err)
	}
	if !os.SameFile(expected, current) || expected.Mode().Type() != current.Mode().Type() {
		return fmt.Errorf("%s changed during scan", label)
	}
	return nil
}

func (s *Service) revalidateDiscovered(item discovered) error {
	currentRootInfo, err := s.validateScanRoot(item.root)
	if err != nil {
		return err
	}
	if item.rootInfo == nil || !os.SameFile(item.rootInfo, currentRootInfo) || item.rootInfo.Mode().Type() != currentRootInfo.Mode().Type() {
		return fmt.Errorf("inventory root changed after discovery")
	}
	if err := revalidatePathIdentity(item.root.path, item.rootInfo, "inventory root"); err != nil {
		return err
	}
	if err := revalidatePathIdentity(item.target, item.targetInfo, "inventory target"); err != nil {
		return err
	}
	state, err := s.deps.Inspect(item.target, s.deps.Paths.LibrarySkills)
	if err != nil {
		return err
	}
	if state.Kind != item.linkState.Kind || state.SkillID != item.linkState.SkillID || state.Broken != item.linkState.Broken {
		return fmt.Errorf("inventory target changed after discovery")
	}
	if item.candidate.Root != "" {
		candidates, err := library.Discover(item.target)
		if err != nil {
			return err
		}
		if len(candidates) != 1 || candidates[0].Name != item.candidate.Name || candidates[0].Hash != item.candidate.Hash {
			return fmt.Errorf("inventory target content changed after discovery")
		}
	}
	return revalidatePathIdentity(item.target, item.targetInfo, "inventory target")
}

func (s *Service) appendPendingDiscovered(cfg *config.Config, roots []scanRoot, found []discovered) []discovered {
	seen := make(map[string]struct{}, len(found))
	for _, item := range found {
		seen[item.root.origin+"\x00"+item.target] = struct{}{}
	}
	for _, operation := range cfg.PendingOperations {
		for _, root := range roots {
			if operation.Scope.Agent != root.agent || operation.Scope.Project != root.project {
				continue
			}
			target := operation.Target
			pendingIssue := ""
			if filepath.IsAbs(target) {
				target = filepath.Clean(target)
			} else {
				pendingIssue = "pending recovery target is not an absolute canonical path"
			}
			identity := root.origin + "\x00" + target
			if _, exists := seen[identity]; exists {
				continue
			}
			var rootInfo, targetInfo os.FileInfo
			state := link.State{Kind: link.StateAbsent}
			if pendingIssue == "" && filepath.Dir(target) != root.path {
				pendingIssue = "pending recovery target is outside the selected inventory root"
			}
			if pendingIssue == "" {
				var rootErr error
				rootInfo, rootErr = os.Lstat(root.path)
				switch {
				case os.IsNotExist(rootErr):
					pendingIssue = "pending recovery root is missing"
				case rootErr != nil:
					pendingIssue = fmt.Sprintf("pending recovery root cannot be inspected: %v", rootErr)
				case rootInfo.Mode()&os.ModeSymlink != 0:
					pendingIssue = "pending recovery root is a symlink"
					rootInfo = nil
				case !rootInfo.IsDir():
					pendingIssue = "pending recovery root is not a directory"
					rootInfo = nil
				}
			}
			if pendingIssue == "" {
				var targetErr error
				targetInfo, targetErr = os.Lstat(target)
				switch {
				case os.IsNotExist(targetErr):
					pendingIssue = "pending recovery target is missing"
				case targetErr != nil:
					pendingIssue = fmt.Sprintf("pending recovery target cannot be inspected: %v", targetErr)
				case targetInfo.Mode()&os.ModeSymlink != 0:
					state.Kind = link.StateExternalLink
					pendingIssue = "pending recovery target is a symlink not accepted by discovery"
				case targetInfo.IsDir():
					state.Kind = link.StateDirectory
					pendingIssue = "pending recovery target directory was not accepted by discovery"
				case targetInfo.Mode().IsRegular():
					state.Kind = link.StateFile
					pendingIssue = "pending recovery target is an unexpected regular file"
				default:
					state.Kind = link.StateFile
					pendingIssue = "pending recovery target has an unsupported file type"
				}
			}
			found = append(found, discovered{
				root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo,
				linkState: state, pendingIssue: pendingIssue, managedID: operation.SkillID,
			})
			seen[identity] = struct{}{}
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].root.origin != found[j].root.origin {
			return originLess(found[i].root.origin, found[j].root.origin)
		}
		return found[i].target < found[j].target
	})
	return found
}

func selectDiscovered(found []discovered, request app.ScanRequest) ([]discovered, error) {
	legacy := make([]discovered, 0, len(found))
	for _, item := range found {
		if selectedScanItem(item, request.Skills) && selectedScanTarget(item, request.Targets) {
			legacy = append(legacy, item)
		}
	}
	if len(request.Selectors) == 0 {
		return legacy, nil
	}
	byIdentity := make(map[string]discovered, len(legacy))
	for _, item := range legacy {
		key := inventoryKey(item.root.origin, item.target)
		byIdentity[key+"\x00"+item.root.origin+"\x00"+item.target] = item
	}
	selected := make([]discovered, 0, len(request.Selectors))
	seen := make(map[string]struct{}, len(request.Selectors))
	for _, selector := range request.Selectors {
		if selector.Key == "" || selector.Origin == "" || selector.Target == "" {
			return nil, fmt.Errorf("inventory selector requires key, origin, and target")
		}
		canonical, err := canonicalInventoryTarget(selector.Target)
		if err != nil || canonical != selector.Target {
			return nil, fmt.Errorf("inventory selector target must be canonical")
		}
		if inventoryKey(selector.Origin, selector.Target) != selector.Key {
			return nil, fmt.Errorf("inventory selector key does not match origin and target")
		}
		identity := selector.Key + "\x00" + selector.Origin + "\x00" + selector.Target
		item, ok := byIdentity[identity]
		if !ok {
			return nil, fmt.Errorf("inventory selector no longer matches current origin and target")
		}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		selected = append(selected, item)
	}
	return selected, nil
}

func (s *Service) validateSelectorSnapshots(cfg *config.Config, selected []discovered, request app.ScanRequest) error {
	if len(request.Selectors) == 0 {
		return nil
	}
	selectors := make(map[string]app.ScanSelector, len(request.Selectors))
	for _, selector := range request.Selectors {
		selectors[selector.Key+"\x00"+selector.Origin+"\x00"+selector.Target] = selector
	}
	for _, item := range selected {
		key := inventoryKey(item.root.origin, item.target)
		selector := selectors[key+"\x00"+item.root.origin+"\x00"+item.target]
		current := s.inventoryItem(cfg, item, request.Adopt)
		if selector.ExpectedHash == "" || selector.ExpectedObjectID == "" || selector.ExpectedRootID == "" || selector.ExpectedState == "" || selector.ExpectedSkillID == "" {
			return fmt.Errorf("inventory selector preview identity is incomplete")
		}
		if current.MatchedLibraryID != "" && selector.ExpectedLibraryHash == "" {
			return fmt.Errorf("inventory selector preview library identity is incomplete")
		}
		if selector.ExpectedHash != current.ContentHash || selector.ExpectedLibraryHash != current.MatchedLibraryActualHash || selector.ExpectedObjectID != current.ObjectID || selector.ExpectedRootID != current.RootObjectID || selector.ExpectedState != current.State || selector.ExpectedSkillID != current.Skill.ID {
			return fmt.Errorf("inventory selector preview identity no longer matches current item")
		}
	}
	return nil
}

func canonicalInventoryTarget(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("inventory target must be absolute")
	}
	return filepath.Clean(path), nil
}

func inventoryKey(origin, target string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aikit-inventory-v1\x00"))
	for _, value := range []string{origin, target} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func mergeScanItem(preview, mutation app.ScanItem) app.ScanItem {
	preview.Adopted = mutation.Adopted
	if mutation.Skill.ID != "" {
		preview.Skill = mutation.Skill
	}
	if mutation.Error != "" {
		preview.Error = mutation.Error
	}
	return preview
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
