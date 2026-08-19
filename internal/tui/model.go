package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
)

type View string

const (
	ViewOverview      View = "overview"
	ViewLibrary       View = "library"
	ViewWorkspaces    View = "workspaces"
	ViewPresets       View = "presets"
	ViewMigration     View = "migration"
	ViewStatus        View = "status"
	ViewConfiguration View = "configuration"

	// Legacy launch aliases are accepted at the command boundary and open the
	// corresponding subsection of Workspaces.
	ViewAgents   View = "agents"
	ViewProjects View = "projects"
)

var topViews = []View{ViewOverview, ViewLibrary, ViewPresets}

type Action string

const (
	ActionNone              Action = ""
	ActionEnable            Action = "enable"
	ActionUpdate            Action = "update"
	ActionScan              Action = "adopt"
	ActionBinding           Action = "binding"
	ActionAdd               Action = "add source"
	ActionDiscoverAdd       Action = "discover remote source"
	ActionPreset            Action = "preset"
	ActionForcePresetDelete Action = "force delete preset"
	ActionBatch             Action = "batch"
	ActionRef               Action = "change ref"
	ActionSync              Action = "sync"
	ActionRemoveSkill       Action = "remove skill"
	ActionForceRemove       Action = "force remove skill"
	ActionForceBatchRemove  Action = "force remove selected"
	ActionProjectEdit       Action = "project edit"
	ActionRemoveProject     Action = "remove project"
	ActionRecovery          Action = "resume recovery"
)

type Mode string

const (
	ModeTable           Mode = "table"
	ModeFilter          Mode = "filter"
	ModeUpdates         Mode = "updates"
	ModeScan            Mode = "scan"
	ModeConfirm         Mode = "confirm"
	ModeInput           Mode = "input"
	ModeAddSelect       Mode = "add-select"
	ModeProjectAgents   Mode = "project-agents"
	ModeWorkspaceSkills Mode = "workspace-skills"
	ModeScopePicker     Mode = "scope-picker"
	ModePresetPicker    Mode = "preset-picker"
	ModeMore            Mode = "more"
	ModeErrorDetail     Mode = "error-detail"
	ModeCommand         Mode = "command"
)

type Focus string

const (
	FocusNavigation        Focus = "navigation"
	FocusList              Focus = "list"
	FocusCollectionActions Focus = "collection-actions"
	FocusDetail            Focus = "detail"
	FocusDetailActions     Focus = "detail-actions"
	FocusActions           Focus = "overlay-actions"
	FocusStatus            Focus = "activity-status"
)

type routePosition struct {
	Cursor, Scroll int
	ActiveKey      string
}

type LibraryStateFilter string

const (
	LibraryStateAll             LibraryStateFilter = "all"
	LibraryStateUpdateAvailable LibraryStateFilter = "update-available"
	LibraryStateManaged         LibraryStateFilter = "managed"
)

type LibrarySourceFilter string

const (
	LibrarySourceAll   LibrarySourceFilter = "all"
	LibrarySourceGit   LibrarySourceFilter = "git"
	LibrarySourceLocal LibrarySourceFilter = "local"
)

type configurationValidationDisplay struct {
	Attempted bool
	Valid     bool
	Message   string
}

type inputKind string

const (
	inputAddSource       inputKind = "add-source"
	inputPresetCreate    inputKind = "preset-create"
	inputPresetDuplicate inputKind = "preset-duplicate"
	inputPresetRename    inputKind = "preset-rename"
	inputRefChange       inputKind = "ref-change"
	inputProjectCreate   inputKind = "project-create"
	inputProjectName     inputKind = "project-name"
	inputProjectRename   inputKind = "project-rename"
	inputProjectPath     inputKind = "project-path"
)

type inputState struct {
	Kind   inputKind
	Prompt string
	Value  string
}

type Scope struct {
	Agent   string
	Project string
	Preset  string
	Level   string
}

type InventoryState struct {
	Generation uint64
	Items      []app.ScanItem
	Issues     []app.ScanIssue
	Completed  int
	Total      int
	Loading    bool
	Complete   bool
}

type row struct {
	Key               string
	ID                string
	Name              string
	Origin            string
	Target            string
	Source            string
	State             string
	Action            string
	Detail            string
	Enabled           bool
	Direct            bool
	Severity          rowSeverity
	DestinationView   View
	DestinationMode   Mode
	DestinationKey    string
	DestinationAction Action
}

type rowSeverity uint8

const (
	rowSeverityInfo rowSeverity = iota
	rowSeveritySuccess
	rowSeverityWarning
	rowSeverityConflict
	rowSeverityError
	rowSeverityRecovery
)

func severityRank(severity rowSeverity) int {
	switch severity {
	case rowSeverityRecovery, rowSeverityError:
		return 0
	case rowSeverityConflict:
		return 1
	case rowSeverityWarning:
		return 2
	default:
		return 3
	}
}

func (r row) selectionKey() string {
	if r.Key != "" {
		return r.Key
	}
	if r.Target != "" {
		return r.Origin + "\x00" + r.Target
	}
	return r.ID
}

type confirmReturnState struct {
	Valid         bool
	ActiveView    View
	Mode          Mode
	Focus         Focus
	Status        string
	ActionIndex   int
	Cursor        int
	Scroll        int
	DetailScroll  int
	OverlayScroll int
	Selected      map[string]bool
}

type Model struct {
	ctx             context.Context
	service         app.Service
	migration       app.MigrationService
	action          Action
	inventoryCancel context.CancelFunc
	inventoryEvents <-chan app.InventoryEvent

	ActiveView              View
	OverviewSection         overviewSectionID
	Mode                    Mode
	Focus                   Focus
	ActionIndex             int
	ActionPane              actionPane
	MorePane                actionPane
	CommandDraft            string
	CommandIndex            int
	NavigationIndex         int
	Scope                   Scope
	Cursor                  int
	Scroll                  int
	DetailScroll            int
	OverlayScroll           int
	errorDetailParent       Mode
	errorDetailReturn       confirmReturnState
	Filter                  string
	FilterDraft             string
	LibraryStateFilter      LibraryStateFilter
	LibrarySourceFilter     LibrarySourceFilter
	Help                    bool
	Detail                  bool
	Width                   int
	Height                  int
	Status                  string
	Err                     string
	Activity                Activity
	Busy                    bool
	MutationBusy            bool
	activityGeneration      uint64
	Snapshot                app.Snapshot
	Scan                    app.ScanResult
	Inventory               InventoryState
	Selected                map[string]bool
	Ignored                 map[string]bool
	routePositions          map[string]routePosition
	overviewPositions       map[overviewSectionID]routePosition
	Config                  app.ConfigurationDetail
	ConfigValidation        app.ConfigurationValidation
	ConfigValidationDisplay configurationValidationDisplay
	SkillDetail             app.SkillDetail
	Preview                 app.MutationPreview
	PlanPreview             app.Result
	AddPreview              app.AddPreview
	Compare                 app.CompareResult
	BatchResult             app.BatchResult
	ProjectPreview          app.ProjectEditPreview
	ProjectRegistration     app.ProjectRegistrationPreview
	ProjectResult           app.Result
	OperationResult         app.Result
	OperationName           string
	Picker                  pickerState
	RecoveryPreview         app.RecoveryPreview
	RecoveryResult          app.RecoveryResult
	FullError               string
	FullDetailTitle         string
	UpdateWarnings          []string
	UpdateFailures          []updatecheck.Result
	Input                   inputState
	confirm                 Action
	pendingID               string
	pendingDetailID         string
	pendingBinding          app.BindingPreviewRequest
	pendingRemove           app.RemoveRequest
	pendingScan             app.ScanRequest
	pendingSync             app.SyncRequest
	pendingAdd              app.AddRequest
	pendingPreset           app.PresetMutationRequest
	pendingBatch            app.BatchRequest
	pendingUpdate           app.UpdateRequest
	pendingProject          app.ProjectEditRequest
	pendingProjectPath      string
	pendingProjectOpen      string
	workspaceIntent         string
	pendingRecovery         app.RecoveryRequest
	forceAcknowledged       bool
	filterParent            Mode
	filterCursor            int
	filterScroll            int
	confirmReturn           confirmReturnState
	confirmReturnReady      bool
	startupUpdatePending    bool
}

func NewModel(ctx context.Context, service app.Service, migration app.MigrationService, initialView View, initialAction Action) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	scope := Scope{}
	switch initialView {
	case ViewAgents:
		initialView, scope.Level = ViewWorkspaces, "workspace-agents"
	case ViewProjects:
		initialView, scope.Level = ViewWorkspaces, "workspace-projects"
	case ViewWorkspaces:
		scope.Level = "workspace-projects"
	}
	if !validView(initialView) {
		initialView = ViewOverview
	}
	return Model{
		ctx: ctx, service: service, migration: migration, action: initialAction,
		ActiveView: initialView, OverviewSection: overviewUpdates, Mode: ModeTable, Focus: FocusList, Scope: scope, Width: 80, Height: 24,
		Status: "Loading local snapshot...", Selected: make(map[string]bool), Ignored: make(map[string]bool),
		startupUpdatePending: true,
		routePositions:       make(map[string]routePosition),
		overviewPositions:    make(map[overviewSectionID]routePosition),
		LibraryStateFilter:   LibraryStateAll, LibrarySourceFilter: LibrarySourceAll,
	}
}

func validView(view View) bool {
	if view == ViewMigration || view == ViewStatus || view == ViewWorkspaces || view == ViewConfiguration {
		return true
	}
	for _, current := range topViews {
		if current == view {
			return true
		}
	}
	return false
}

func (m Model) Init() tea.Cmd { return snapshotCmd(m.ctx, m.service) }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case activityResultMsg:
		if msg.Generation != m.activityGeneration {
			return m, nil
		}
		nextModel, command := m.Update(msg.Message)
		next := nextModel.(Model)
		if command != nil && (next.Inventory.Loading || next.activityGeneration != msg.Generation) {
			return next, command
		}
		if command != nil && (next.Busy || next.MutationBusy) {
			return next, next.beginTransitionActivity(command)
		}
		kind, label, review := next.finishActivityResult(msg.Message)
		expiry := next.finishActivity(kind, label, "", review)
		if command != nil {
			return next, command
		}
		return next, expiry
	case activityTickMsg:
		if msg.Generation != m.activityGeneration || (m.Activity.Kind != ActivityReading && m.Activity.Kind != ActivityNetwork && m.Activity.Kind != ActivityMutating) {
			return m, nil
		}
		m.Activity.Frame = (m.Activity.Frame + 1) % len(activitySpinnerFrames)
		if msg.results == nil {
			return m, nil
		}
		return m, waitActivityResultCmd(msg.Generation, msg.results)
	case activityExpireMsg:
		if msg.Generation == m.activityGeneration && m.Activity.Kind == ActivitySuccess {
			m.Activity = Activity{}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.Width, m.Height = max(0, msg.Width), max(0, msg.Height)
		m.ensureVisible()
		return m, nil
	case snapshotMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err = msg.err.Error()
			m.Status = "Snapshot failed"
			return m, nil
		}
		m.Snapshot = msg.snapshot
		m.Err = strings.Join(msg.snapshot.Updates.Warnings, "; ")
		m.Status = "Local snapshot ready"
		action := m.action
		m.action = ActionNone
		switch action {
		case ActionUpdate:
			m.openUpdates()
		case ActionScan:
			m.switchView(ViewMigration)
		case ActionEnable:
			m.ActiveView, m.Scope.Level = ViewWorkspaces, "workspace-agents"
		}
		m.clampCursor()
		if len(msg.snapshot.Config.PendingOperations) > 0 {
			ids := make([]string, 0, len(msg.snapshot.Config.PendingOperations))
			for _, operation := range msg.snapshot.Config.PendingOperations {
				ids = append(ids, operation.ID)
			}
			m.pendingRecovery = app.RecoveryRequest{OperationIDs: ids}
			m.Busy, m.Status = true, "Building recovery preview..."
			return m, m.beginTransitionActivity(recoveryPreviewCmd(m.ctx, m.service, m.pendingRecovery))
		}
		if m.pendingProjectOpen != "" {
			if _, ok := findProject(msg.snapshot.Config.Projects, m.pendingProjectOpen); ok {
				m.ActiveView = ViewWorkspaces
				m.Scope = Scope{Project: m.pendingProjectOpen, Level: "project-targets"}
				m.Cursor, m.Scroll, m.Detail, m.DetailScroll = 0, 0, false, 0
			}
			m.pendingProjectOpen = ""
		}
		return m.startInventory()
	case inventoryMsg:
		if !msg.ok || msg.event.Generation != m.Inventory.Generation {
			return m, nil
		}
		activeKey := m.activeKey()
		viewportOffset := m.Cursor - m.Scroll
		m.mergeInventory(msg.event)
		if m.ActiveView == ViewOverview {
			m.reconcileOverviewSelection()
		}
		m.restoreActiveKey(activeKey)
		if m.ActiveView == ViewOverview && activeKey != "" {
			m.Scroll = max(0, m.Cursor-viewportOffset)
			m.Scroll = m.visibleRowsLayout(ComputeLayout(m.Width, m.Height)).Start
		}
		if msg.event.Done {
			m.Inventory.Loading, m.Inventory.Complete = false, true
			m.Status = fmt.Sprintf("Inventory complete: %d roots", m.Inventory.Completed)
			if m.startupUpdatePending {
				m.startupUpdatePending = false
				m.Busy, m.Status = true, "Running startup update check..."
				return m, m.beginTransitionActivity(cachedUpdateCheckCmd(m.ctx, m.service))
			}
			if len(m.Inventory.Issues) > 0 {
				key := inventoryIssueKey(m.Inventory.Issues[0])
				return m, m.finishActivity(ActivityWarning, fmt.Sprintf("Inventory completed with %d issue(s)", len(m.Inventory.Issues)), m.Inventory.Issues[0].Message, ReviewTarget{Kind: ReviewInventoryIssue, Key: key})
			}
			return m, m.finishActivity(ActivitySuccess, m.Status, "", ReviewTarget{})
		}
		return m, waitInventoryCmd(m.inventoryEvents)
	case scanMsg:
		m.Busy = false
		if msg.err != nil {
			m.confirmReturn, m.confirmReturnReady = confirmReturnState{}, false
			m.Err = msg.err.Error()
			m.Status = "Migration preview failed"
			return m, nil
		}
		m.Scan = msg.result
		m.ActiveView = ViewMigration
		if msg.name == "migration preview" {
			m.enterConfirm(ActionScan)
			m.Status = "Review the exact migration plan, then confirm"
		} else {
			m.Mode = ModeScan
			m.Cursor, m.Scroll = 0, 0
			m.Selected = make(map[string]bool)
			m.Status = fmt.Sprintf("Found %d migration candidates", len(msg.result.Items))
		}
		m.Err = strings.Join(msg.result.Warnings, "; ")
		return m, nil
	case skillDetailMsg:
		if msg.skillID != m.pendingDetailID {
			return m, nil
		}
		m.pendingDetailID = ""
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Skill detail failed"
			return m, nil
		}
		m.SkillDetail, m.Detail, m.DetailScroll = msg.detail, true, 0
		m.Status = "Skill detail loaded"
		return m, nil
	case mutationPreviewMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), title(msg.name)+" preview failed"
			return m, nil
		}
		m.Preview = msg.preview
		m.enterConfirm(m.confirm)
		m.Status = "Review the exact change, then confirm"
		return m, nil
	case batchPreviewMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Batch preview failed"
			return m, nil
		}
		m.Preview = msg.preview.MutationPreview
		m.enterConfirm(ActionBatch)
		m.Status = "Review the exact atomic batch, then confirm"
		return m, nil
	case syncPreviewMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Sync preview failed"
			return m, nil
		}
		m.PlanPreview = msg.result
		m.enterConfirm(ActionSync)
		m.Status = "Review the exact sync plan, then confirm"
		return m, nil
	case addPreviewMsg:
		m.Busy, m.MutationBusy = false, false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Add source preview failed"
			return m, nil
		}
		m.AddPreview = msg.preview
		m.Selected = make(map[string]bool)
		m.Cursor, m.Scroll = 0, 0
		if msg.preview.NetworkRequired {
			m.enterConfirm(ActionDiscoverAdd)
			m.Preview = app.MutationPreview{
				Title: "Discover remote source", Summary: "Clone this repository into a temporary directory and list its skills. This does not change the Library.",
				Warnings: append([]string(nil), msg.preview.Warnings...), RequiresConfirmation: true,
			}
			m.Status = "Confirm network-enabled add"
			return m, nil
		}
		if msg.preview.ResolvedSource != "" {
			m.pendingAdd.Source = msg.preview.ResolvedSource
		}
		m.pendingAdd.Ref = msg.preview.Ref
		m.pendingAdd.ExpectedResolved = msg.preview.Resolved
		for _, suggestion := range msg.preview.SuggestedSelections {
			for _, candidate := range msg.preview.Candidates {
				if candidate.Name == suggestion || candidate.RelativePath == suggestion {
					m.Selected[candidate.RelativePath] = true
				}
			}
		}
		m.confirm = ActionNone
		m.confirmReturn = confirmReturnState{}
		m.Mode = ModeAddSelect
		m.Status = fmt.Sprintf("Select from %d discovered skill(s)", len(msg.preview.Candidates))
		if len(msg.preview.Warnings) > 0 {
			m.Status += " · Warning: " + msg.preview.Warnings[0]
		}
		return m, nil
	case compareMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Compare failed"
			return m, nil
		}
		m.Compare, m.Detail, m.DetailScroll = msg.result, true, 0
		m.Status = "Comparison ready"
		return m, nil
	case updateCheckMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Update check failed"
			return m, nil
		}
		m.Snapshot.Updates = msg.result.Updates
		m.UpdateWarnings = append([]string(nil), msg.result.Updates.Warnings...)
		m.UpdateFailures = nil
		for _, item := range msg.result.Updates.Results {
			if item.State == updatecheck.StateCheckFailed {
				m.UpdateFailures = append(m.UpdateFailures, item)
			}
		}
		if m.ActiveView == ViewOverview {
			m.Mode, m.OverviewSection, m.Focus = ModeTable, overviewUpdates, FocusList
			m.Cursor, m.Scroll = 0, 0
			m.reconcileOverviewSelection()
		} else {
			m.openUpdates()
		}
		if problems := len(m.UpdateWarnings) + len(m.UpdateFailures); problems > 0 {
			m.Status = fmt.Sprintf("Update check completed with %d problem(s); review details", problems)
		} else {
			m.Status = fmt.Sprintf("Update check complete: %d update(s) available", len(m.rows()))
		}
		return m, nil
	case configurationMsg:
		if msg.err != nil {
			m.Err = msg.err.Error()
			return m, nil
		}
		m.Config = msg.detail
		return m, nil
	case configurationValidationMsg:
		m.Busy = false
		m.ConfigValidation = msg.result
		m.ConfigValidationDisplay = configurationValidationDisplay{
			Attempted: true,
			Valid:     msg.err == nil && msg.result.Valid,
			Message:   firstNonEmpty(errorText(msg.err), msg.result.Path),
		}
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Configuration invalid; review details"
			return m, nil
		}
		m.Err = ""
		m.Status = "Configuration valid"
		return m, nil
	case configurationReloadMsg:
		if msg.err != nil {
			m.Busy = false
			m.Err, m.Status = msg.err.Error(), "Configuration reload failed"
			return m, nil
		}
		m.Config = msg.detail
		m.Status = "Reloading offline snapshot..."
		return m, offlineSnapshotCmd(m.ctx, m.service)
	case projectPreviewMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Project edit preview failed"
			return m, nil
		}
		m.ProjectPreview = msg.preview
		if msg.preview.PathIdentity != "" {
			m.pendingProject.ExpectedPathIdentity = msg.preview.PathIdentity
		}
		m.enterConfirm(ActionProjectEdit)
		m.Status = "Review cleanup and next project paths, then confirm"
		return m, nil
	case projectRegistrationPreviewMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Project registration preview failed"
			return m, nil
		}
		m.ProjectRegistration = msg.preview
		m.pendingProjectPath = msg.preview.Path
		if msg.preview.NeedsName {
			m.enterInput(inputState{Kind: inputProjectName, Prompt: "Project name"})
			if msg.preview.NameIssue == app.ProjectNameDuplicate {
				m.Status = "That project name is already in use; choose another name"
			} else {
				m.Status = "The derived project name is invalid; choose a project name"
			}
			return m, nil
		}
		m.ProjectPreview = msg.preview.Preview
		m.pendingProject = app.ProjectEditRequest{
			Name:                 msg.preview.Name,
			Path:                 msg.preview.Path,
			AddAgents:            append([]string(nil), msg.preview.Agents...),
			ExpectedPathIdentity: msg.preview.PathIdentity,
		}
		m.Preview = app.MutationPreview{
			Title:                "Create project",
			Summary:              fmt.Sprintf("Register project %q at %s", msg.preview.Name, msg.preview.Path),
			Warnings:             append([]string(nil), msg.preview.Warnings...),
			RequiresConfirmation: true,
		}
		m.enterConfirm(ActionProjectEdit)
		m.Status = "Review the detected agents and exact project paths, then confirm"
		return m, nil
	case projectOperationMsg:
		m.Busy, m.MutationBusy = false, false
		m.ProjectResult = msg.result
		m.Mode, m.confirm = ModeTable, ActionNone
		m.Input = inputState{}
		if msg.err != nil {
			if pending, ok := pendingRecoveryFromError(msg.err); ok {
				return m.openRecoveryPreview(pending.Operations)
			}
			m.Err, m.Status = msg.err.Error(), title(msg.name)+" failed; review details"
			return m, nil
		}
		m.Err = ""
		m.pendingProjectOpen = m.pendingProject.Project
		if m.pendingProject.Name != "" {
			m.pendingProjectOpen = m.pendingProject.Name
		}
		if msg.result.Exit == app.ExitPartial || len(msg.result.Plan.Issues)+len(msg.result.Link.Failures)+len(msg.result.Link.Issues) > 0 {
			m.Status = title(msg.name) + " partial; review issues"
		} else {
			m.Status = title(msg.name) + " completed"
		}
		return m, snapshotCmd(m.ctx, m.service)
	case recoveryPreviewMsg:
		m.Busy = false
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Recovery preview failed"
			return m, nil
		}
		m.RecoveryPreview = msg.preview
		m.pendingRecovery.OperationIDs = recoveryOperationIDs(msg.preview.Operations)
		m.enterConfirm(ActionRecovery)
		m.Status = "Review recovery and resume exact operations"
		return m, nil
	case recoveryOperationMsg:
		m.Busy, m.MutationBusy = false, false
		m.RecoveryResult = msg.result
		m.Mode, m.confirm = ModeTable, ActionNone
		if msg.err != nil {
			m.Err, m.Status = msg.err.Error(), "Recovery resume failed; review issues"
			return m, nil
		}
		m.Err = ""
		m.Status = fmt.Sprintf("Recovery resumed: %d completed", len(msg.result.Completed))
		return m, snapshotCmd(m.ctx, m.service)
	case operationMsg:
		m.Busy, m.MutationBusy = false, false
		m.OperationResult, m.OperationName = msg.result, msg.name
		if msg.err != nil {
			if pending, ok := pendingRecoveryFromError(msg.err); ok {
				return m.openRecoveryPreview(pending.Operations)
			}
			m.Err = msg.err.Error()
			m.Status = title(msg.name) + " failed"
			return m, nil
		}
		m.Err = ""
		if msg.name == "add" && len(msg.result.Skipped) > 0 {
			if len(msg.result.Skills) == 0 {
				m.Status = fmt.Sprintf("Already in Library · skipped %d skill(s)", len(msg.result.Skipped))
			} else {
				m.Status = fmt.Sprintf("Added %d skill(s) · skipped %d already in Library", len(msg.result.Skills), len(msg.result.Skipped))
			}
		} else if msg.name == "update" {
			m.Status = "Updated selected skills"
		} else {
			m.Status = title(msg.name) + " completed"
		}
		m.Mode = ModeTable
		m.confirm = ActionNone
		m.Preview = app.MutationPreview{}
		m.PlanPreview = app.Result{}
		m.AddPreview = app.AddPreview{}
		m.Input = inputState{}
		m.forceAcknowledged = false
		if msg.name != "preset" || m.ActiveView != ViewPresets || m.Scope.Level != "preset-skills" {
			m.Selected = make(map[string]bool)
		}
		if msg.name == "add" && !msg.result.Changed {
			return m, nil
		}
		return m, snapshotCmd(m.ctx, m.service)
	case batchOperationMsg:
		m.Busy, m.MutationBusy = false, false
		m.BatchResult = msg.result
		m.Mode, m.confirm = ModeTable, ActionNone
		m.Preview = app.MutationPreview{}
		if msg.err != nil {
			if pending, ok := pendingRecoveryFromError(msg.err); ok {
				return m.openRecoveryPreview(pending.Operations)
			}
			m.Err = msg.err.Error()
			if msg.result.Changed {
				m.Status = title(msg.name) + " failed after changes; review details"
			} else {
				m.Status = title(msg.name) + " failed; review details"
			}
			return m, nil
		}
		m.Err = ""
		if len(msg.result.Issues) > 0 || msg.result.Exit == app.ExitPartial {
			m.Status = fmt.Sprintf("%s completed with %d issue(s); review details", title(msg.name), len(msg.result.Issues))
		} else {
			m.Selected = make(map[string]bool)
			m.Status = title(msg.name) + " completed"
		}
		return m, snapshotCmd(m.ctx, m.service)
	case tea.KeyMsg:
		nextModel, command := m.updateKey(msg)
		next := nextModel.(Model)
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+q" {
			return next, command
		}
		return next, next.beginTransitionActivity(command)
	case tea.MouseMsg:
		nextModel, command := m.updateMouse(msg)
		next := nextModel.(Model)
		return next, next.beginTransitionActivity(command)
	}
	return m, nil
}

func (m Model) startInventory() (tea.Model, tea.Cmd) {
	if m.inventoryCancel != nil {
		m.inventoryCancel()
	}
	m.Inventory.Generation++
	m.Inventory.Items = nil
	m.Inventory.Issues = nil
	m.Inventory.Completed = 0
	m.Inventory.Total = 0
	m.Inventory.Loading = true
	m.Inventory.Complete = false
	m.Scroll, m.Cursor = 0, 0
	if m.migration == nil {
		m.Inventory.Loading = false
		m.Err = errUnavailable("migration service").Error()
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.inventoryCancel = cancel
	m.inventoryEvents = m.migration.Inventory(ctx, app.InventoryRequest{Generation: m.Inventory.Generation, AllProjects: true})
	m.Status = "Scanning local skills..."
	m.Busy = true
	m.activityGeneration++
	m.Activity = Activity{Kind: ActivityReading, Label: "Scanning local skills", Generation: m.activityGeneration}
	return m, waitInventoryCmd(m.inventoryEvents)
}

func (m *Model) mergeInventory(event app.InventoryEvent) {
	index := make(map[string]int, len(m.Inventory.Items))
	for i := range m.Inventory.Items {
		index[m.Inventory.Items[i].Key] = i
	}
	for _, item := range event.Items {
		if i, ok := index[item.Key]; ok {
			m.Inventory.Items[i] = item
			continue
		}
		index[item.Key] = len(m.Inventory.Items)
		m.Inventory.Items = append(m.Inventory.Items, item)
	}
	for _, issue := range event.Issues {
		if !containsIssue(m.Inventory.Issues, issue) {
			m.Inventory.Issues = append(m.Inventory.Issues, issue)
		}
	}
	m.Inventory.Completed = max(m.Inventory.Completed, event.Completed)
	m.Inventory.Total = max(m.Inventory.Total, event.Total)
}

func containsIssue(issues []app.ScanIssue, wanted app.ScanIssue) bool {
	for _, issue := range issues {
		if issue.State == wanted.State && issue.Origin == wanted.Origin && issue.Path == wanted.Path && issue.Message == wanted.Message {
			return true
		}
	}
	return false
}

func (m *Model) openUpdates() {
	m.Mode = ModeUpdates
	m.Focus, m.ActionIndex = FocusList, 0
	m.Scope = Scope{}
	m.Cursor, m.Scroll = 0, 0
	m.Detail, m.Help = false, false
	m.DetailScroll = 0
	m.Selected = make(map[string]bool)
	m.Status = "Select updates"
}

func (m *Model) switchView(view View) {
	if !validView(view) {
		return
	}
	m.ActiveView, m.Mode = view, ModeTable
	m.Focus, m.ActionIndex = FocusList, 0
	m.Scope = Scope{}
	if view == ViewWorkspaces {
		m.Scope.Level = "workspace-projects"
	}
	m.Cursor, m.Scroll = 0, 0
	m.Filter, m.FilterDraft = "", ""
	m.Detail, m.Help = false, false
	m.DetailScroll = 0
	m.Selected = make(map[string]bool)
	m.filterParent = ModeTable
}

func (m *Model) beginFilter() {
	m.filterParent = m.Mode
	m.filterCursor, m.filterScroll = m.Cursor, m.Scroll
	m.FilterDraft = m.Filter
	m.Mode = ModeFilter
	m.Cursor, m.Scroll = 0, 0
	m.clampCursor()
}

func (m *Model) applyFilterDraft() {
	m.Filter = m.FilterDraft
	m.Mode = m.filterParent
	m.Cursor, m.Scroll = 0, 0
	m.clampCursor()
}

func (m *Model) cancelFilterDraft() {
	m.Mode = m.filterParent
	m.FilterDraft = m.Filter
	m.Cursor, m.Scroll = m.filterCursor, m.filterScroll
	m.clampCursor()
}

func (m *Model) clampCursor() {
	n := len(m.rows())
	if n == 0 {
		m.Cursor = 0
	} else {
		m.Cursor = min(max(0, m.Cursor), n-1)
	}
}

func (m *Model) ensureVisible() {
	if m.ActiveView == ViewOverview && m.Mode == ModeTable {
		m.ensureOverviewVisible()
		return
	}
	m.clampCursor()
	m.Scroll = m.visibleRowsLayout(ComputeLayout(m.Width, m.Height)).Start
}

func (m Model) activeKey() string {
	rows := m.rows()
	if m.Cursor >= 0 && m.Cursor < len(rows) {
		return rows[m.Cursor].selectionKey()
	}
	return ""
}

func (m *Model) restoreActiveKey(key string) {
	if key == "" {
		m.clampCursor()
		return
	}
	for i, current := range m.rows() {
		if current.selectionKey() == key {
			m.Cursor = i
			return
		}
	}
	m.clampCursor()
}

func (m *Model) cancelInventory() {
	if m.inventoryCancel != nil {
		m.inventoryCancel()
		m.inventoryCancel = nil
	}
	if m.Inventory.Loading {
		m.Inventory.Loading = false
		m.Busy = false
	}
}

func title(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func recoveryOperationIDs(operations []app.RecoveryOperation) []string {
	ids := make([]string, 0, len(operations))
	for _, operation := range operations {
		ids = append(ids, operation.Operation.ID)
	}
	return ids
}

func pendingRecoveryFromError(err error) (*app.PendingRecoveryError, bool) {
	var pending *app.PendingRecoveryError
	return pending, errors.As(err, &pending)
}

func (m Model) openRecoveryPreview(operations []app.RecoveryOperation) (tea.Model, tea.Cmd) {
	m.pendingRecovery = app.RecoveryRequest{OperationIDs: recoveryOperationIDs(operations)}
	m.Busy, m.Status = true, "Building recovery preview..."
	return m, recoveryPreviewCmd(m.ctx, m.service, m.pendingRecovery)
}
