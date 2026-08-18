package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type librarySelectionActionID string

const (
	selectionEnable  librarySelectionActionID = "enable"
	selectionDisable librarySelectionActionID = "disable"
	selectionUpdate  librarySelectionActionID = "update"
	selectionRemove  librarySelectionActionID = "remove"
	selectionClear   librarySelectionActionID = "clear"
)

type librarySelectionAction struct {
	ID       librarySelectionActionID
	Label    string
	Mnemonic rune
	Enabled  bool
	Reason   string
}

func (m Model) librarySelectionActions() []librarySelectionAction {
	ids := m.selectedLibraryIDs()
	if len(ids) == 0 {
		return nil
	}
	updateReason := ""
	if _, err := m.libraryUpdateBatchRequest(ids); err != nil {
		updateReason = err.Error()
	}
	return []librarySelectionAction{
		{ID: selectionEnable, Label: "Enable", Mnemonic: 'e', Enabled: true},
		{ID: selectionDisable, Label: "Disable", Mnemonic: 'd', Enabled: true},
		{ID: selectionUpdate, Label: "Update", Mnemonic: 'u', Enabled: updateReason == "", Reason: updateReason},
		{ID: selectionRemove, Label: "Remove", Mnemonic: 'r', Enabled: true},
		{ID: selectionClear, Label: "Clear", Mnemonic: 'c', Enabled: true},
	}
}

func (m Model) librarySelectionActionByMnemonic(key string) (librarySelectionAction, bool) {
	key = strings.ToLower(key)
	for _, action := range m.librarySelectionActions() {
		if key == string(action.Mnemonic) {
			return action, true
		}
	}
	return librarySelectionAction{}, false
}

func (m Model) libraryUpdateBatchRequest(ids []string) (app.BatchRequest, error) {
	request := app.BatchRequest{
		Operation: app.BatchUpdate,
		SkillIDs:  append([]string(nil), ids...),
		Expected:  make(map[string]app.ExpectedUpdate, len(ids)),
	}
	for _, skillID := range ids {
		skill, ok := snapshotSkill(m.Snapshot.Config, skillID)
		if !ok {
			return app.BatchRequest{}, fmt.Errorf("skill %q is no longer in the Library", skillID)
		}
		if skill.Source == "" || filepath.IsAbs(skill.Source) || strings.HasPrefix(skill.Source, ".") {
			return app.BatchRequest{}, fmt.Errorf("skill %q has no supported Git source", skillID)
		}
		if _, err := library.NormalizeSource(skill.Source); err != nil {
			return app.BatchRequest{}, fmt.Errorf("skill %q has no supported Git source", skillID)
		}
		if skill.Ref == nil || skill.Ref.Kind != "branch" || skill.Ref.Value == "" {
			return app.BatchRequest{}, fmt.Errorf("skill %q has no non-empty branch ref", skillID)
		}
		checked, ok := snapshotUpdate(m.Snapshot, skillID)
		if !ok || checked.State != updatecheck.StateUpdateAvailable {
			return app.BatchRequest{}, fmt.Errorf("skill %q does not have an update-available result", skillID)
		}
		if checked.Current == "" {
			return app.BatchRequest{}, fmt.Errorf("skill %q update result has no current revision", skillID)
		}
		if checked.Remote == "" {
			return app.BatchRequest{}, fmt.Errorf("skill %q update result has no remote revision", skillID)
		}
		if checked.Current != skill.Resolved {
			return app.BatchRequest{}, fmt.Errorf("skill %q changed since its update check", skillID)
		}
		request.Expected[skillID] = app.ExpectedUpdate{
			Ref:      &config.Ref{Kind: skill.Ref.Kind, Value: skill.Ref.Value},
			Resolved: checked.Current,
			Remote:   checked.Remote,
		}
	}
	return request, nil
}
