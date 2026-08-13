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
	Add(context.Context, AddRequest) (Result, error)
	Remove(context.Context, RemoveRequest) (Result, error)
	Enable(context.Context, BindingRequest) (Result, error)
	Disable(context.Context, BindingRequest) (Result, error)
	PutPreset(context.Context, PresetRequest) (Result, error)
	RemovePreset(context.Context, PresetRemoveRequest) (Result, error)
	Sync(context.Context, SyncRequest) (Result, error)
	Update(context.Context, UpdateRequest) (Result, error)
	EditProject(context.Context, ProjectEditRequest) (Result, error)
	PreviewProjectEdit(context.Context, ProjectEditRequest) (ProjectEditPreview, error)
	RemoveProject(context.Context, ProjectRemoveRequest) (Result, error)
}

type MigrationService interface {
	Scan(context.Context, ScanRequest) (ScanResult, error)
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
	Source     string
	Skills     []string
	SourcePath string
	Ref        *config.Ref
	Force      bool
	Agent      string
	Project    string
}

type AddPreviewRequest struct {
	Source     string
	SourcePath string
	Ref        *config.Ref
}

type Candidate struct {
	Name         string
	Description  string
	RelativePath string
	Hash         string
}

type AddPreview struct {
	Candidates []Candidate
	Warnings   []string
}

type BindingRequest struct {
	SkillID string
	Preset  string
	Agent   string
	Project string
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
	Project      string
	Name         string
	Path         string
	AddAgents    []string
	RemoveAgents []string
	Confirmed    bool
}

type ProjectRemoveRequest struct {
	Project   string
	Confirmed bool
}

type ProjectEditPreview struct {
	Cleanup link.Plan
	Next    link.Plan
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
	Agent   string
	Project string
	Skills  []string
	All     bool
	Adopt   bool
}

type ScanItem struct {
	Origin  string
	Skill   config.Skill
	Adopted bool
	Error   string
}

type ScanResult struct {
	Items    []ScanItem
	Warnings []string
	Exit     ExitClass
}

type MigrateRequest struct {
	ProjectPaths []string
	Adopt        bool
}

type MigrateResult struct {
	Imported int
	Skipped  int
	Failed   int
	Warnings []string
	Exit     ExitClass
}
