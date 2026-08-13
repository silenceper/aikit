package link

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/pkg/config"
)

func Inspect(path, libraryRoot string) (State, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return State{Kind: StateAbsent}, nil
	}
	if err != nil {
		return State{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if info.IsDir() {
			return State{Kind: StateDirectory}, nil
		}
		return State{Kind: StateFile}, nil
	}

	raw, err := os.Readlink(path)
	if err != nil {
		return State{}, err
	}
	lexical := raw
	if !filepath.IsAbs(lexical) {
		lexical = filepath.Join(filepath.Dir(path), lexical)
	}
	lexical = filepath.Clean(lexical)
	broken := false
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return State{}, err
		}
		broken = true
	} else if resolved, err := filepath.EvalSymlinks(path); err == nil {
		resolvedRoot := libraryRoot
		if evaluatedRoot, rootErr := filepath.EvalSymlinks(libraryRoot); rootErr == nil {
			resolvedRoot = evaluatedRoot
		}
		if id, ok := containedID(resolvedRoot, resolved); ok {
			return State{Kind: StateManagedLink, SkillID: id, LinkTarget: raw}, nil
		}
		return State{Kind: StateExternalLink, LinkTarget: raw}, nil
	}
	if id, ok := containedID(libraryRoot, lexical); ok {
		return State{Kind: StateManagedLink, SkillID: id, LinkTarget: raw, Broken: broken}, nil
	}
	return State{Kind: StateExternalLink, LinkTarget: raw, Broken: broken}, nil
}

func containedID(root, path string) (string, bool) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func libraryPath(root, id string) (string, bool) {
	path := filepath.Join(root, filepath.FromSlash(id))
	got, ok := containedID(root, path)
	if !ok || got != id {
		return path, false
	}
	resolvedRoot, err := resolveExistingPrefix(root)
	if err != nil {
		return path, false
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path, true
	} else if err != nil {
		return path, false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, false
	}
	resolvedID, inside := containedID(resolvedRoot, resolved)
	return path, inside && resolvedID == id
}

func LibraryPath(root, id string) (string, bool) { return libraryPath(root, id) }
func ValidateTarget(target Target) error         { return validateOverlayDir(target) }

func targetPath(root, name string) (string, bool) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", false
	}
	path := filepath.Join(root, name)
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return path, err == nil && rel == name && !filepath.IsAbs(rel)
}

func BuildPlan(libraryRoot string, targets []Target, pending []config.PendingOperation, selector Selector) Plan {
	plan := Plan{}
	pendingTargets := map[string]struct{}{}
	for _, op := range pending {
		if !selector.Matches(op.Scope) {
			continue
		}
		pendingTargets[filepath.Clean(op.Target)] = struct{}{}
		plan.Actions = append(plan.Actions, Action{Kind: ActionRecover, Scope: op.Scope, Path: op.Target, SkillID: op.SkillID, Operation: op.ID, Library: libraryRoot})
	}
	for _, target := range targets {
		if !selector.Matches(target.Scope) {
			continue
		}
		if target.Blocked != "" {
			plan.Issues = append(plan.Issues, Issue{Kind: IssueScopeConflict, Scope: target.Scope, Path: target.Dir, Message: target.Blocked})
			continue
		}
		err := validateOverlayDir(target)
		if err != nil {
			message := "target directory is outside its scope"
			message = err.Error()
			plan.Issues = append(plan.Issues, Issue{Kind: IssueUnsafePath, Scope: target.Scope, Path: target.Dir, Message: message, Err: err})
			continue
		}
		if target.Scope.Project != "" {
			info, err := os.Stat(target.Scope.ProjectPath)
			if err != nil || !info.IsDir() {
				plan.Warnings = append(plan.Warnings, Issue{Kind: IssueProjectMissing, Scope: target.Scope, Path: target.Scope.ProjectPath, Message: "project path is missing or not a directory", Err: err})
				continue
			}
		}
		planTarget(&plan, libraryRoot, target, pendingTargets)
	}
	sort.SliceStable(plan.Actions, func(i, j int) bool {
		if plan.Actions[i].Path == plan.Actions[j].Path {
			return plan.Actions[i].Kind < plan.Actions[j].Kind
		}
		return plan.Actions[i].Path < plan.Actions[j].Path
	})
	sort.SliceStable(plan.Issues, func(i, j int) bool { return plan.Issues[i].Path < plan.Issues[j].Path })
	return plan
}

func expectedTargetDir(target Target) (string, error) {
	a, ok := agent.ByName(target.Scope.Agent)
	if !ok {
		return "", fmt.Errorf("unknown agent %q", target.Scope.Agent)
	}
	if target.Scope.Project != "" {
		if !filepath.IsAbs(target.Scope.ProjectPath) || filepath.Clean(target.Scope.ProjectPath) != target.Scope.ProjectPath {
			return "", fmt.Errorf("project scope has unsafe project path")
		}
		return a.ProjectSkillDir(target.Scope.ProjectPath), nil
	}
	if target.Home == "" || !filepath.IsAbs(target.Home) || filepath.Clean(target.Home) != target.Home {
		return "", fmt.Errorf("global target requires a clean absolute home")
	}
	return a.GlobalSkillDir(target.Home), nil
}

func sameResolvedPath(expected, actual string) bool {
	return filepath.Clean(expected) == filepath.Clean(actual)
}

func validateOverlayDir(target Target) error {
	expected, err := expectedTargetDir(target)
	if err != nil {
		return err
	}
	if !sameResolvedPath(expected, target.Dir) {
		return fmt.Errorf("target directory is outside its scope")
	}
	base := target.Home
	if target.Scope.Project != "" {
		base = target.Scope.ProjectPath
	}
	resolvedBase, err := resolveExistingPrefix(base)
	if err != nil {
		return fmt.Errorf("resolve scope base: %w", err)
	}
	resolvedDir, err := resolveExistingPrefix(target.Dir)
	if err != nil {
		return err
	}
	if _, ok := containedID(resolvedBase, resolvedDir); !ok {
		return fmt.Errorf("overlay directory escapes resolved scope base")
	}
	return nil
}

func resolveExistingPrefix(path string) (string, error) {
	missing := []string{}
	current := filepath.Clean(path)
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, e := filepath.EvalSymlinks(current)
			if e != nil {
				return "", e
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func planTarget(plan *Plan, libraryRoot string, target Target, pending map[string]struct{}) {
	names := make([]string, 0, len(target.Desired))
	for name := range target.Desired {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path, targetSafe := targetPath(target.Dir, name)
		if !targetSafe {
			plan.Issues = append(plan.Issues, Issue{Kind: IssueConflict, Scope: target.Scope, Path: target.Dir, Message: "unsafe target name"})
			continue
		}
		if _, ok := pending[filepath.Clean(path)]; ok {
			continue
		}
		id := target.Desired[name]
		lib, safe := libraryPath(libraryRoot, id)
		if !safe {
			plan.Issues = append(plan.Issues, Issue{Kind: IssueLibraryMissing, Scope: target.Scope, Path: path, SkillID: id, Message: "library path is unsafe or escapes library root"})
			continue
		}
		info, err := os.Stat(lib)
		if err != nil || !info.IsDir() {
			plan.Issues = append(plan.Issues, Issue{Kind: IssueLibraryMissing, Scope: target.Scope, Path: path, SkillID: id, Message: "library directory is missing"})
			continue
		}
		state, err := Inspect(path, libraryRoot)
		if err != nil {
			plan.Issues = append(plan.Issues, Issue{Kind: IssueIO, Scope: target.Scope, Path: path, SkillID: id, Err: err, Message: err.Error()})
			continue
		}
		switch state.Kind {
		case StateAbsent:
			plan.Actions = append(plan.Actions, Action{Kind: ActionCreate, Scope: target.Scope, Path: path, SkillID: id, Library: libraryRoot, TargetDir: target.Dir, Home: target.Home})
		case StateManagedLink:
			if state.SkillID != id || state.Broken {
				plan.Actions = append(plan.Actions, Action{Kind: ActionReplace, Scope: target.Scope, Path: path, SkillID: id, PreviousID: state.SkillID, Library: libraryRoot, TargetDir: target.Dir, Home: target.Home})
			}
		default:
			plan.Issues = append(plan.Issues, Issue{Kind: IssueConflict, Scope: target.Scope, Path: path, SkillID: id, Message: fmt.Sprintf("target is %s", state.Kind)})
		}
	}

	entries, err := os.ReadDir(target.Dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		plan.Issues = append(plan.Issues, Issue{Kind: IssueIO, Scope: target.Scope, Path: target.Dir, Err: err, Message: err.Error()})
		return
	}
	for _, entry := range entries {
		if _, wanted := target.Desired[entry.Name()]; wanted {
			continue
		}
		path := filepath.Join(target.Dir, entry.Name())
		if _, ok := pending[filepath.Clean(path)]; ok {
			continue
		}
		state, err := Inspect(path, libraryRoot)
		if err != nil {
			plan.Issues = append(plan.Issues, Issue{Kind: IssueIO, Scope: target.Scope, Path: path, Err: err, Message: err.Error()})
			continue
		}
		if state.Kind == StateManagedLink {
			plan.Actions = append(plan.Actions, Action{Kind: ActionRemove, Scope: target.Scope, Path: path, SkillID: state.SkillID, Library: libraryRoot, TargetDir: target.Dir, Home: target.Home})
		}
	}
}
