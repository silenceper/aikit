// Package link is the only package that mutates agent skill overlays.
package link

import "github.com/silenceper/aikit/pkg/config"

type StateKind string

const (
	StateAbsent       StateKind = "absent"
	StateManagedLink  StateKind = "managed-link"
	StateExternalLink StateKind = "external-link"
	StateDirectory    StateKind = "directory"
	StateFile         StateKind = "file"
)

type State struct {
	Kind       StateKind
	SkillID    string
	LinkTarget string
	Broken     bool
}

type Target struct {
	Scope   config.Scope
	Home    string // required for global scopes; ignored for project scopes
	Dir     string
	Desired map[string]string // target short name -> library id
	Blocked string            // non-empty skips the complete target
}

type Selector struct {
	Agent   string
	Project string
}

func (s Selector) Matches(scope config.Scope) bool {
	if s.Project == "" && s.Agent == "" {
		return true
	}
	if s.Project == "" {
		return scope.Project == "" && scope.Agent == s.Agent
	}
	if scope.Project != s.Project {
		return false
	}
	return s.Agent == "" || scope.Agent == s.Agent
}

type ActionKind string

const (
	ActionCreate  ActionKind = "create"
	ActionReplace ActionKind = "replace"
	ActionRemove  ActionKind = "remove"
	ActionRecover ActionKind = "recover"
)

type Action struct {
	Kind       ActionKind
	Scope      config.Scope
	Path       string
	SkillID    string
	Operation  string
	Library    string
	TargetDir  string
	PreviousID string
	Home       string
}

type IssueKind string

const (
	IssueLibraryMissing IssueKind = "library-missing"
	IssueConflict       IssueKind = "conflict"
	IssueScopeConflict  IssueKind = "scope-conflict"
	IssueIO             IssueKind = "io"
	IssuePendingCleanup IssueKind = "pending-cleanup"
	IssueAdoptRecovery  IssueKind = "adopt-recovery"
	IssueProjectMissing IssueKind = "project-missing"
	IssueUnsafePath     IssueKind = "unsafe-path"
)

type Issue struct {
	Kind      IssueKind
	Scope     config.Scope
	Path      string
	SkillID   string
	Operation string
	Message   string
	Err       error
}

type Plan struct {
	Actions  []Action
	Issues   []Issue
	Warnings []Issue
}

type Result struct {
	Actions   []Action
	Applied   []Action
	Failures  []Issue
	Issues    []Issue
	Warnings  []Issue
	Completed []string
}
