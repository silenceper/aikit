package link

import (
	"fmt"
	"os"
	"path/filepath"
)

func Execute(plan Plan, dryRun bool) Result {
	return ExecuteWithOps(plan, dryRun, defaultFileOps())
}

func ExecuteWithOps(plan Plan, dryRun bool, ops FileOps) Result {
	result := Result{Actions: append([]Action(nil), plan.Actions...), Issues: append([]Issue(nil), plan.Issues...), Warnings: append([]Issue(nil), plan.Warnings...)}
	if dryRun {
		return result
	}
	for _, action := range plan.Actions {
		if action.Kind == ActionRecover {
			continue
		}
		if err := execute(action, ops); err != nil {
			result.Failures = append(result.Failures, Issue{Kind: IssueIO, Scope: action.Scope, Path: action.Path, SkillID: action.SkillID, Err: err, Message: err.Error()})
			continue
		}
		result.Applied = append(result.Applied, action)
	}
	return result
}

func execute(action Action, ops FileOps) error {
	if action.Kind != ActionRecover {
		if err := validateOverlayDir(Target{Scope: action.Scope, Home: action.Home, Dir: action.TargetDir}); err != nil {
			return err
		}
		path, safe := targetPath(action.TargetDir, filepath.Base(action.Path))
		if !safe || path != filepath.Clean(action.Path) || filepath.Dir(action.Path) != filepath.Clean(action.TargetDir) {
			return fmt.Errorf("unsafe action path outside target directory")
		}
	}
	switch action.Kind {
	case ActionCreate, ActionReplace:
		lib, safe := libraryPath(action.Library, action.SkillID)
		if !safe {
			return fmt.Errorf("unsafe skill id %q", action.SkillID)
		}
		info, err := os.Stat(lib)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("library-missing: %s", action.SkillID)
		}
		if err := os.MkdirAll(filepath.Dir(action.Path), 0o755); err != nil {
			return err
		}
		if action.Kind == ActionCreate {
			if state, err := Inspect(action.Path, action.Library); err != nil || state.Kind != StateAbsent {
				if err != nil {
					return err
				}
				return fmt.Errorf("target appeared before create")
			}
			return ops.Symlink(lib, action.Path)
		} else {
			tombstone, err := quarantineManaged(action.Path, action.PreviousID, action.Library, ops, ".aikit-link-old-")
			if err != nil {
				return err
			}
			if err := ops.Symlink(lib, action.Path); err != nil {
				if restoreErr := ops.MoveNoReplace(tombstone, action.Path); restoreErr != nil {
					return fmt.Errorf("create replacement: %v; original retained at %s; restore failed: %w", err, tombstone, restoreErr)
				}
				return err
			}
			if err := ops.Remove(tombstone); err != nil {
				return fmt.Errorf("replacement installed but old link remains at %s: %w", tombstone, err)
			}
			return nil
		}
	case ActionRemove:
		state, err := Inspect(action.Path, action.Library)
		if err != nil {
			return err
		}
		if state.Kind == StateAbsent {
			return nil
		}
		tombstone, err := quarantineManaged(action.Path, action.SkillID, action.Library, ops, ".aikit-link-delete-")
		if err != nil {
			return err
		}
		if err := ops.Remove(tombstone); err != nil {
			return fmt.Errorf("link retained at tombstone %s: %w", tombstone, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported action %q", action.Kind)
	}
}

func quarantineManaged(path, expectedID, libraryRoot string, ops FileOps, prefix string) (string, error) {
	tombstone, err := unusedSibling(path, prefix)
	if err != nil {
		return "", err
	}
	if err := ops.MoveNoReplace(path, tombstone); err != nil {
		return "", err
	}
	state, inspectErr := Inspect(tombstone, libraryRoot)
	if inspectErr == nil && state.Kind == StateManagedLink && state.SkillID == expectedID {
		return tombstone, nil
	}
	if restoreErr := ops.MoveNoReplace(tombstone, path); restoreErr != nil {
		return "", fmt.Errorf("moved changed object to %s; restore failed: %w", tombstone, restoreErr)
	}
	if inspectErr != nil {
		return "", inspectErr
	}
	return "", fmt.Errorf("target changed before mutation; restored without deleting")
}

func unusedSibling(target, prefix string) (string, error) {
	for i := 0; i < 100; i++ {
		nonce, err := operationID("")
		if err != nil {
			return "", err
		}
		candidate := filepath.Join(filepath.Dir(target), prefix+nonce)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("unable to allocate reserved sibling path")
}
