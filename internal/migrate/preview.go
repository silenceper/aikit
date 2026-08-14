package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

type classificationInput struct {
	managed         bool
	matched         bool
	bound           bool
	sameContent     bool
	nameConflict    bool
	broken          bool
	drifted         bool
	updateAvailable bool
	pending         bool
	hasError        bool
}

type scanClassification struct {
	state      app.ScanState
	management app.ScanState
	diagnostic app.ScanState
	action     app.ScanAction
}

func classifyScanItem(input classificationInput) scanClassification {
	result := scanClassification{}
	switch {
	case input.managed:
		if input.bound {
			result = scanClassification{management: app.ScanStateManaged, action: app.ScanActionNone}
		} else {
			result = scanClassification{management: app.ScanStateManaged, action: app.ScanActionLinkExisting}
		}
	case input.sameContent:
		result = scanClassification{management: app.ScanStateSameContent, action: app.ScanActionLinkExisting}
	case input.nameConflict:
		result = scanClassification{management: app.ScanStateNameConflict, action: app.ScanActionConflict}
	default:
		result = scanClassification{management: app.ScanStateUnmanaged, action: app.ScanActionAdopt}
	}
	result.diagnostic = result.management
	switch {
	case input.pending:
		result.diagnostic = app.ScanStatePendingRecovery
		result.action = app.ScanActionNone
	case input.hasError:
		result.diagnostic = app.ScanStateError
		result.action = app.ScanActionNone
	case input.broken:
		result.diagnostic = app.ScanStateBrokenLink
		result.action = app.ScanActionNone
	case input.drifted:
		result.diagnostic = app.ScanStateDrifted
		result.action = app.ScanActionNone
	case input.updateAvailable:
		result.diagnostic = app.ScanStateUpdateAvailable
		result.action = app.ScanActionNone
	}
	result.state = result.diagnostic
	return result
}

func (s *Service) inventoryItem(cfg *config.Config, item discovered, adoptIntent bool) app.ScanItem {
	output := app.ScanItem{
		Key:     inventoryKey(item.root.origin, item.target),
		Origin:  item.root.origin,
		Target:  item.target,
		Scope:   itemScope(item.root),
		Agent:   item.root.agent,
		Project: item.root.project,
		Discovered: app.Candidate{
			Name: item.candidate.Name, Description: item.candidate.Description,
			RelativePath: item.candidate.RelativePath, Hash: item.candidate.Hash,
		},
		DiscoveredHash: item.candidate.Hash,
		ContentHash:    item.candidate.Hash,
		Skill:          item.allocated,
	}
	input := classificationInput{}
	if item.targetInfo != nil {
		identity, err := objectIdentity(item.target, item.targetInfo)
		if err != nil {
			input.hasError = true
			output.Error = err.Error()
		} else {
			output.ObjectID = identity
		}
	}
	if item.rootInfo != nil {
		identity, err := objectIdentity(item.root.path, item.rootInfo)
		if err != nil {
			input.hasError = true
			if output.Error == "" {
				output.Error = err.Error()
			} else {
				output.Error += "; " + err.Error()
			}
		} else {
			output.RootObjectID = identity
		}
	}
	if item.linkState.Kind == link.StateExternalLink && item.linkState.Broken {
		input.broken = true
	}
	var matched config.Skill
	if item.managedID != "" {
		input.managed = true
		input.broken = item.linkState.Broken
		matched, input.matched = findSkill(cfg, item.managedID)
		if input.matched {
			output.Skill = matched
			output.MatchedLibraryID = matched.ID
			output.MatchedLibraryHash = matched.Hash
			input.bound = bindingContains(cfg, item.root, matched.ID)
			if !input.broken {
				libraryPath, pathErr := library.SafeLibraryPath(s.deps.Paths.LibrarySkills, matched.ID)
				var info os.FileInfo
				var statErr error
				if pathErr == nil {
					info, statErr = os.Stat(libraryPath)
				}
				if pathErr != nil || statErr != nil || info == nil || !info.IsDir() {
					input.broken = true
				} else if hash, err := library.HashSkill(libraryPath); err != nil {
					input.hasError = true
					output.Error = err.Error()
				} else {
					output.ContentHash = hash
					output.MatchedLibraryActualHash = hash
					if matched.Hash != "" && hash != matched.Hash {
						input.drifted = true
					}
				}
			}
		} else {
			input.broken = true
			output.Error = fmt.Sprintf("managed link references unknown skill %q", item.managedID)
		}
	} else {
		for _, skill := range cfg.Library.Skills {
			if skill.Name != item.candidate.Name {
				continue
			}
			libraryPath, pathErr := library.SafeLibraryPath(s.deps.Paths.LibrarySkills, skill.ID)
			var actualHash string
			if pathErr == nil {
				info, statErr := os.Stat(libraryPath)
				if statErr != nil {
					pathErr = statErr
				} else if !info.IsDir() {
					pathErr = fmt.Errorf("library skill %q is not a directory", skill.ID)
				} else {
					actualHash, pathErr = library.HashSkill(libraryPath)
				}
			}
			matched = skill
			output.MatchedLibraryID = skill.ID
			output.MatchedLibraryHash = skill.Hash
			if pathErr != nil {
				input.hasError = true
				output.Error = pathErr.Error()
				break
			}
			output.MatchedLibraryActualHash = actualHash
			if actualHash != skill.Hash {
				input.drifted = true
				if skill.Hash == item.candidate.Hash {
					input.sameContent = true
				} else {
					input.nameConflict = true
				}
				break
			}
			if actualHash == item.candidate.Hash {
				input.sameContent = true
				break
			}
			input.nameConflict = true
		}
		if matched.ID != "" {
			output.Skill = matched
		}
	}
	for _, operation := range cfg.PendingOperations {
		if filepath.Clean(operation.Target) == item.target && operation.Scope.Agent == item.root.agent && operation.Scope.Project == item.root.project {
			input.pending = true
			break
		}
	}
	classification := classifyScanItem(input)
	output.State = classification.state
	output.ManagementState = classification.management
	output.DiagnosticState = classification.diagnostic
	output.Action = classification.action
	if output.Action == app.ScanActionAdopt && !adoptIntent {
		output.Action = app.ScanActionImport
	}
	if output.Error != "" || output.DiagnosticState != output.ManagementState {
		message := output.Error
		if item.pendingIssue != "" {
			message = item.pendingIssue
		}
		if message == "" {
			message = fmt.Sprintf("inventory target is %s", output.DiagnosticState)
		}
		output.Issues = append(output.Issues, app.ScanIssue{State: output.DiagnosticState, Origin: output.Origin, Path: output.Target, Message: message})
	}
	return output
}

func bindingContains(cfg *config.Config, root scanRoot, id string) bool {
	if root.project == "" {
		return contains(cfg.Agents[root.agent].Skills, id)
	}
	for _, project := range cfg.Projects {
		if project.Name == root.project {
			return contains(project.AgentBindings[root.agent].Skills, id)
		}
	}
	return false
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func (s *Service) preflightAdoptBatch(cfg *config.Config, items []discovered) ([]app.ScanItem, error) {
	outputs := make([]app.ScanItem, len(items))
	for index, item := range items {
		outputs[index] = s.inventoryItem(cfg, item, true)
	}
	clone := cloneConfig(cfg)
	for index, item := range items {
		output := outputs[index]
		switch output.Action {
		case app.ScanActionAdopt, app.ScanActionLinkExisting:
			if output.Skill.ID == "" || output.ContentHash == "" {
				return outputs, fmt.Errorf("inventory item %s has incomplete adoption identity", output.Target)
			}
			appendSkill(clone, output.Skill)
			addBinding(clone, item.root, output.Skill.ID)
			if err := validateMutation(clone); err != nil {
				return outputs, err
			}
		case app.ScanActionConflict:
			return outputs, fmt.Errorf("inventory item %s conflicts with the library", output.Target)
		case app.ScanActionNone:
			if output.State != app.ScanStateManaged {
				return outputs, fmt.Errorf("inventory item %s is not safe to adopt in state %s", output.Target, output.State)
			}
		default:
			return outputs, fmt.Errorf("inventory item %s has unsupported adoption action %s", output.Target, output.Action)
		}
	}
	return outputs, nil
}

func markScanBatchError(items []app.ScanItem, err error) []app.ScanItem {
	for index := range items {
		items[index].Error = err.Error()
		items[index].State = app.ScanStateError
		items[index].DiagnosticState = app.ScanStateError
		items[index].Action = app.ScanActionNone
		items[index].Issues = append(items[index].Issues, app.ScanIssue{
			State: app.ScanStateError, Origin: items[index].Origin,
			Path: items[index].Target, Message: err.Error(),
		})
	}
	return items
}
