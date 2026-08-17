// Package app coordinates the skills manager's durable ledger and filesystem.
// Frontends depend on the interfaces in this file and never write either one.
package app

import (
	"context"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type Service interface {
	Snapshot(context.Context, StatusRequest) (Snapshot, error)
	PreviewAdd(context.Context, AddPreviewRequest) (AddPreview, error)
	PreviewBinding(context.Context, BindingPreviewRequest) (MutationPreview, error)
	PreviewRemove(context.Context, RemoveRequest) (MutationPreview, error)
	PreviewPreset(context.Context, PresetPreviewRequest) (MutationPreview, error)
	PreviewPresetMutation(context.Context, PresetMutationRequest) (MutationPreview, error)
	SkillDetail(context.Context, string) (SkillDetail, error)
	Configuration(context.Context) (ConfigurationDetail, error)
	ValidateConfiguration(context.Context) (ConfigurationValidation, error)
	Add(context.Context, AddRequest) (Result, error)
	Remove(context.Context, RemoveRequest) (Result, error)
	Enable(context.Context, BindingRequest) (Result, error)
	Disable(context.Context, BindingRequest) (Result, error)
	PutPreset(context.Context, PresetRequest) (Result, error)
	RemovePreset(context.Context, PresetRemoveRequest) (Result, error)
	Sync(context.Context, SyncRequest) (Result, error)
	Update(context.Context, UpdateRequest) (Result, error)
	EditProject(context.Context, ProjectEditRequest) (Result, error)
	PreviewProjectRegistration(context.Context, ProjectRegistrationRequest) (ProjectRegistrationPreview, error)
	PreviewProjectEdit(context.Context, ProjectEditRequest) (ProjectEditPreview, error)
	PreviewProjectRemove(context.Context, ProjectRemoveRequest) (MutationPreview, error)
	RemoveProject(context.Context, ProjectRemoveRequest) (Result, error)
	PreviewBatch(context.Context, BatchRequest) (BatchPreview, error)
	Batch(context.Context, BatchRequest) (BatchResult, error)
	MutatePreset(context.Context, PresetMutationRequest) (Result, error)
	Compare(context.Context, CompareRequest) (CompareResult, error)
	PreviewRecovery(context.Context, RecoveryRequest) (RecoveryPreview, error)
	ResumeRecovery(context.Context, RecoveryRequest) (RecoveryResult, error)
	RollbackRecovery(context.Context, RecoveryRequest) (RecoveryResult, error)
}

type MigrationService interface {
	Scan(context.Context, ScanRequest) (ScanResult, error)
	Inventory(context.Context, InventoryRequest) <-chan InventoryEvent
	Migrate(context.Context, MigrateRequest) (MigrateResult, error)
}

type StatusRequest struct {
	Offline      bool
	ForceRefresh bool
}

type Snapshot struct {
	Config  config.Config
	Status  status.Report
	Updates updatecheck.CheckReport
	Exit    ExitClass
}

type AddRequest struct {
	Source             string
	Skills             []string
	SourcePath         string
	Ref                *config.Ref
	ExpectedResolved   string
	ExpectedCandidates []ExpectedAddCandidate
	Force              bool
	Agent              string
	Project            string
}

type AddPreviewRequest struct {
	Source       string
	SourcePath   string
	Ref          *config.Ref
	AllowNetwork bool
}

type ExpectedAddCandidate struct {
	Name         string
	RelativePath string
	Hash         string
}

type Candidate struct {
	Name         string
	Description  string
	RelativePath string
	Hash         string
}

type AddPreview struct {
	Candidates          []Candidate
	Warnings            []string
	NetworkRequired     bool
	ResolvedSource      string
	SuggestedSelections []string
	Ref                 *config.Ref
	Resolved            string
}

type BindingRequest struct {
	SkillID string
	Preset  string
	Agent   string
	Project string
}

// BindingPreviewRequest describes the same logical edit as Enable or Disable
// without authorizing a mutation.
type BindingPreviewRequest struct {
	Binding BindingRequest
	Enable  bool
}

// PresetPreviewRequest previews either a member edit or preset deletion. Remove
// removes Skills from the preset; Delete removes the preset itself.
type PresetPreviewRequest struct {
	Name   string
	Skills []string
	Create bool
	Remove bool
	Delete bool
	Force  bool
}

// MutationPreview is safe for presentation by CLI, TUI, or JSON frontends. The
// link plan remains authoritative for exact filesystem paths and actions.
type MutationPreview struct {
	Title                string
	Summary              string
	AffectedScopes       []config.Scope
	References           []string
	Plan                 link.Plan
	Warnings             []string
	Conflicts            []string
	RequiresForce        bool
	RequiresConfirmation bool
}

type SkillFileKind string

const (
	SkillFileRegular   SkillFileKind = "file"
	SkillFileDirectory SkillFileKind = "directory"
	SkillFileSymlink   SkillFileKind = "symlink"
	SkillFileOther     SkillFileKind = "other"
)

// SkillPreviewByteLimit bounds the SKILL.md body returned to interactive
// frontends. Truncation is byte-based and reported separately.
const SkillPreviewByteLimit = 64 << 10

type SkillFile struct {
	Path       string
	Kind       SkillFileKind
	Size       int64
	LinkTarget string
}

type SkillDetail struct {
	Skill            config.Skill
	EnabledLocations []config.Scope
	SkillMD          string
	SkillMDTruncated bool
	Files            []SkillFile
}

type ConfigurationDetail struct {
	Config  string
	Library string
	Cache   string
}

type ConfigurationValidation struct {
	Path  string
	Valid bool
}

type RemoveRequest struct {
	SkillID string
	Force   bool
}

type PresetRequest struct {
	Name   string
	Skills []string
	Create bool
	Remove bool
}

type PresetRemoveRequest struct {
	Name  string
	Force bool
}

type SyncRequest struct {
	Agent   string
	Project string
	DryRun  bool
}

type UpdateRequest struct {
	SkillIDs  []string
	Ref       *config.Ref
	Expected  map[string]ExpectedUpdate
	Force     bool
	CheckOnly bool
	Offline   bool
	Refresh   bool
	Confirmed bool
}

type ExpectedUpdate struct {
	Ref      *config.Ref
	Resolved string
	Remote   string
}

type ProjectEditRequest struct {
	Project              string
	Name                 string
	Path                 string
	AddAgents            []string
	RemoveAgents         []string
	ExpectedPathIdentity string
	Confirmed            bool
}

type ProjectNameIssue string

const (
	ProjectNameInvalid   ProjectNameIssue = "invalid"
	ProjectNameDuplicate ProjectNameIssue = "duplicate"
)

type ProjectRegistrationRequest struct {
	Path string
	Name string
}

type ProjectRegistrationPreview struct {
	Name         string
	Path         string
	PathIdentity string
	Agents       []string
	Preview      ProjectEditPreview
	Warnings     []string
	NeedsName    bool
	NameIssue    ProjectNameIssue
}

type ProjectRemoveRequest struct {
	Project   string
	Confirmed bool
}

type ProjectEditPreview struct {
	Cleanup      link.Plan
	Next         link.Plan
	PathIdentity string
}

// BatchOperation identifies a single transaction spanning all selected items.
// A batch is never implemented by composing the public single-item methods.
type BatchOperation string

const (
	BatchEnable  BatchOperation = "enable"
	BatchDisable BatchOperation = "disable"
	BatchRemove  BatchOperation = "remove"
	BatchUpdate  BatchOperation = "update"
)

type BatchRequest struct {
	Operation BatchOperation
	Bindings  []BindingRequest
	SkillIDs  []string
	Force     bool
	Ref       *config.Ref
	Expected  map[string]ExpectedUpdate
	Confirmed bool
}

// OperationIssue retains enough context for a frontend to present a partial
// or rejected item without parsing an error string.
type OperationIssue struct {
	Operation string
	Item      string
	Path      string
	Message   string
	Err       error
}

type BatchItemResult struct {
	Item    string
	Changed bool
	Issue   *OperationIssue
}

type BatchResult struct {
	Result
	Items  []BatchItemResult
	Issues []OperationIssue
}

type BatchPreview struct {
	MutationPreview
	Items  []BatchItemResult
	Issues []OperationIssue
}

type PresetMutationOperation string

const (
	PresetCreate      PresetMutationOperation = "create"
	PresetDuplicate   PresetMutationOperation = "duplicate"
	PresetRename      PresetMutationOperation = "rename"
	PresetEditMembers PresetMutationOperation = "edit-members"
	PresetDelete      PresetMutationOperation = "delete"
	PresetApply       PresetMutationOperation = "apply"
)

type PresetMutationRequest struct {
	Operation PresetMutationOperation
	Name      string
	NewName   string
	Skills    []string
	Binding   BindingRequest
	Force     bool
	Confirmed bool
}

type CompareRequest struct {
	LeftSkillID  string
	RightSkillID string
}

type CompareSide struct {
	Skill config.Skill
	Files []SkillFile
}

type MetadataDiff struct {
	Field string
	Left  string
	Right string
}

type FileDiffKind string

const (
	FileOnlyLeft  FileDiffKind = "only-left"
	FileOnlyRight FileDiffKind = "only-right"
	FileChanged   FileDiffKind = "changed"
)

type FileDiff struct {
	Path string
	Kind FileDiffKind
}

type ContentDiff struct {
	Path           string
	Left           string
	Right          string
	LeftTruncated  bool
	RightTruncated bool
}

type CompareResult struct {
	Left     CompareSide
	Right    CompareSide
	Metadata []MetadataDiff
	Files    []FileDiff
	Content  []ContentDiff
	Equal    bool
}

type RecoveryRequest struct {
	OperationIDs []string
	Confirmed    bool
}

type RecoveryOperation struct {
	Operation           config.PendingOperation
	CanResume           bool
	CanRollback         bool
	RollbackUnavailable string
}

type RecoveryPreview struct {
	Operations []RecoveryOperation
	Plan       link.Plan
	Issues     []OperationIssue
}

type RecoveryResult struct {
	Completed []string
	Issues    []OperationIssue
	Changed   bool
}

// PendingRecoveryError is returned before an ordinary mutation performs any
// recovery, checkpoint, library preparation, or filesystem write.
type PendingRecoveryError struct {
	Operations []RecoveryOperation
}

func (e *PendingRecoveryError) Error() string {
	return "pending recovery requires review before starting another mutation"
}

type ExitClass int

const (
	ExitOK ExitClass = iota
	ExitPartial
	ExitUpdatesAvailable
)

type Result struct {
	Skills   []config.Skill
	Updates  updatecheck.CheckReport
	Plan     link.Plan
	Link     link.Result
	Warnings []string
	Changed  bool
	Exit     ExitClass
}

type ScanRequest struct {
	Agent       string
	Project     string
	Skills      []string
	Targets     []string
	Selectors   []ScanSelector
	All         bool
	AllProjects bool
	Adopt       bool
	DryRun      bool
}

// ScanSelector is the complete identity supplied by an inventory frontend.
// Key is a stable display/merge identity, not an authorization token: the
// migration service must also match Origin and the canonical Target.
type ScanSelector struct {
	Key                 string
	Origin              string
	Target              string
	ExpectedHash        string
	ExpectedObjectID    string
	ExpectedRootID      string
	ExpectedState       ScanState
	ExpectedSkillID     string
	ExpectedLibraryHash string
}

type ScanState string

const (
	ScanStateManaged         ScanState = "managed"
	ScanStateUnmanaged       ScanState = "unmanaged"
	ScanStateSameContent     ScanState = "same-content"
	ScanStateNameConflict    ScanState = "name-conflict"
	ScanStateBrokenLink      ScanState = "broken-link"
	ScanStateDrifted         ScanState = "drifted"
	ScanStateUpdateAvailable ScanState = "update-available"
	ScanStatePendingRecovery ScanState = "pending-recovery"
	ScanStateError           ScanState = "error"
)

type ScanAction string

const (
	ScanActionNone         ScanAction = "none"
	ScanActionImport       ScanAction = "import"
	ScanActionAdopt        ScanAction = "adopt"
	ScanActionLinkExisting ScanAction = "link-existing"
	ScanActionConflict     ScanAction = "conflict"
)

type ScanIssue struct {
	State   ScanState
	Origin  string
	Path    string
	Message string
}

type ScanItem struct {
	Key                      string
	Origin                   string
	Target                   string
	Scope                    config.Scope
	Agent                    string
	Project                  string
	Discovered               Candidate
	DiscoveredHash           string
	ContentHash              string
	ObjectID                 string
	RootObjectID             string
	MatchedLibraryID         string
	MatchedLibraryHash       string
	MatchedLibraryActualHash string
	State                    ScanState
	ManagementState          ScanState
	DiagnosticState          ScanState
	Issues                   []ScanIssue
	Action                   ScanAction
	Skill                    config.Skill
	Adopted                  bool
	Error                    string
}

type ScanResult struct {
	Items    []ScanItem
	Issues   []ScanIssue
	Warnings []string
	Exit     ExitClass
}

type InventoryRequest struct {
	Generation  uint64
	AllProjects bool
}

type InventoryEvent struct {
	Generation uint64
	Root       string
	Items      []ScanItem
	Issues     []ScanIssue
	Completed  int
	Total      int
	Done       bool
}

type MigrateRequest struct {
	ProjectPaths []string
	Adopt        bool
	DryRun       bool
}

type MigrateResult struct {
	Imported     int
	PendingAdopt int
	Skipped      int
	Failed       int
	Warnings     []string
	Exit         ExitClass
}
