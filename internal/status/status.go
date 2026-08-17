// Package status provides a strictly read-only view of ledger/filesystem drift.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

type Kind string

const (
	Missing        Kind = "missing"
	LibraryMissing Kind = "library-missing"
	Conflict       Kind = "conflict"
	ScopeConflict  Kind = "scope-conflict"
	Unmanaged      Kind = "unmanaged"
	OrphanedLink   Kind = "orphaned-link"
	PendingCleanup Kind = "pending-cleanup"
	AdoptRecovery  Kind = "adopt-recovery"
	IOError        Kind = "io-error"
	ProjectMissing Kind = "project-missing"
)

type Item struct {
	Kind       Kind
	Scope      config.Scope
	Path       string
	Name       string
	SkillID    string
	IDs        []string
	Operation  string
	Backup     string
	Journal    string
	DeleteRoot string
	Manifest   string
	Message    string
}

type TargetSummary struct {
	Scope      config.Scope
	Dir        string
	Expected   int
	Linked     int
	PathExists bool
}

type Report struct {
	LibrarySkills int
	Targets       []TargetSummary
	Items         []Item
	Warnings      []Item
}

func (r Report) Healthy() bool { return len(r.Items) == 0 }

func Inspect(cfg *config.Config, home, libraryRoot string) Report {
	if cfg == nil {
		return Report{}
	}
	r := Report{LibrarySkills: len(cfg.Library.Skills)}
	for _, a := range agent.All() {
		v := scope.Global(cfg, a.Name())
		appendScopeIssues(&r, v)
		if len(v.Issues) == 0 {
			dir := a.GlobalSkillDir(home)
			if err := link.ValidateTarget(link.Target{Scope: v.Scope, Home: home, Dir: dir}); err != nil {
				r.Items = append(r.Items, Item{Kind: IOError, Scope: v.Scope, Path: dir, Message: err.Error()})
			} else {
				inspectTarget(&r, v.Scope, dir, v.Skills, libraryRoot)
			}
		} else {
			r.Targets = append(r.Targets, TargetSummary{Scope: v.Scope, Dir: a.GlobalSkillDir(home), Expected: len(v.Skills)})
		}
	}
	for _, project := range cfg.Projects {
		pathInfo, pathErr := os.Stat(project.Path)
		for _, agentName := range project.Agents {
			v := scope.Project(cfg, project, agentName)
			appendScopeIssues(&r, v)
			if pathErr != nil || !pathInfo.IsDir() || len(v.Issues) != 0 {
				if a, ok := agent.ByName(agentName); ok {
					r.Targets = append(r.Targets, TargetSummary{Scope: v.Scope, Dir: a.ProjectSkillDir(project.Path), Expected: len(v.Skills), PathExists: pathErr == nil && pathInfo.IsDir()})
				}
				if pathErr != nil || !pathInfo.IsDir() {
					r.Warnings = append(r.Warnings, Item{Kind: ProjectMissing, Scope: v.Scope, Path: project.Path, Message: "project path is missing or not a directory"})
				}
				continue
			}
			a, ok := agent.ByName(agentName)
			if !ok {
				continue
			}
			dir := a.ProjectSkillDir(project.Path)
			if err := link.ValidateTarget(link.Target{Scope: v.Scope, Dir: dir}); err != nil {
				r.Items = append(r.Items, Item{Kind: IOError, Scope: v.Scope, Path: dir, Message: err.Error()})
			} else {
				inspectTarget(&r, v.Scope, dir, v.Skills, libraryRoot)
			}
		}
	}
	for _, op := range cfg.PendingOperations {
		kind := PendingCleanup
		if op.Kind == config.OperationAdopt {
			kind = AdoptRecovery
		}
		item := Item{Kind: kind, Scope: op.Scope, Path: op.Target, SkillID: op.SkillID, Operation: op.ID, Backup: op.Backup, Message: "pending operation requires sync or manual recovery"}
		if op.Kind == config.OperationAdopt {
			item.Journal = op.JournalHash
			item.DeleteRoot, item.Manifest = link.DeleteJournalPaths(op)
		}
		r.Items = append(r.Items, item)
	}
	sort.SliceStable(r.Items, func(i, j int) bool {
		if r.Items[i].Kind == r.Items[j].Kind {
			return r.Items[i].Path < r.Items[j].Path
		}
		return r.Items[i].Kind < r.Items[j].Kind
	})
	return r
}

func appendScopeIssues(r *Report, v scope.View) {
	for _, issue := range v.Issues {
		kind := Conflict
		if issue.Kind == scope.IssueScopeConflict {
			kind = ScopeConflict
		}
		r.Items = append(r.Items, Item{Kind: kind, Scope: issue.Scope, Name: issue.Name, IDs: append([]string(nil), issue.IDs...), Message: issue.Message})
	}
}

func inspectTarget(r *Report, s config.Scope, dir string, desired map[string]config.Skill, libraryRoot string) {
	summary := TargetSummary{Scope: s, Dir: dir, Expected: len(desired)}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		summary.PathExists = true
	}
	defer func() { r.Targets = append(r.Targets, summary) }()
	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		skill := desired[name]
		path, safe := statusTargetPath(dir, name)
		if !safe {
			r.Items = append(r.Items, Item{Kind: Conflict, Scope: s, Path: dir, Name: name, SkillID: skill.ID, Message: "unsafe target name"})
			continue
		}
		state, stateErr := link.Inspect(path, libraryRoot)
		if stateErr != nil {
			r.Items = append(r.Items, Item{Kind: IOError, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: stateErr.Error()})
			continue
		}
		occupiedConflict := state.Kind == link.StateDirectory || state.Kind == link.StateFile || state.Kind == link.StateExternalLink
		if occupiedConflict {
			r.Items = append(r.Items, Item{Kind: Conflict, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: fmt.Sprintf("expected path is occupied by %s", state.Kind)})
		}
		lib, librarySafe := link.LibraryPath(libraryRoot, skill.ID)
		if !librarySafe {
			r.Items = append(r.Items, Item{Kind: LibraryMissing, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: "library path is unsafe or escapes library root"})
			continue
		}
		if info, err := os.Stat(lib); err != nil || !info.IsDir() {
			r.Items = append(r.Items, Item{Kind: LibraryMissing, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: "library directory is missing"})
			continue
		}
		if occupiedConflict {
			continue
		}
		switch state.Kind {
		case link.StateAbsent:
			r.Items = append(r.Items, Item{Kind: Missing, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: "expected link is absent"})
		case link.StateManagedLink:
			if state.SkillID != skill.ID || state.Broken {
				r.Items = append(r.Items, Item{Kind: Missing, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: fmt.Sprintf("link points to %s", state.SkillID)})
			} else {
				summary.Linked++
			}
		default:
			r.Items = append(r.Items, Item{Kind: Conflict, Scope: s, Path: path, Name: name, SkillID: skill.ID, Message: fmt.Sprintf("expected path is occupied by %s", state.Kind)})
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			r.Items = append(r.Items, Item{Kind: IOError, Scope: s, Path: dir, Message: err.Error()})
		}
		return
	}
	for _, entry := range entries {
		if _, expected := desired[entry.Name()]; expected {
			continue
		}
		// Agent-owned hidden directories (for example Codex's .system bundle)
		// are implementation metadata, not user-installable skill entries.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		state, err := link.Inspect(path, libraryRoot)
		if err != nil {
			r.Items = append(r.Items, Item{Kind: IOError, Scope: s, Path: path, Name: entry.Name(), Message: err.Error()})
			continue
		}
		switch state.Kind {
		case link.StateManagedLink:
			r.Items = append(r.Items, Item{Kind: OrphanedLink, Scope: s, Path: path, Name: entry.Name(), SkillID: state.SkillID, Message: "aikit link has no binding"})
		case link.StateDirectory, link.StateFile, link.StateExternalLink:
			r.Items = append(r.Items, Item{Kind: Unmanaged, Scope: s, Path: path, Name: entry.Name(), Message: fmt.Sprintf("unmanaged %s", state.Kind)})
		}
	}
}

func statusTargetPath(root, name string) (string, bool) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", false
	}
	p := filepath.Join(root, name)
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return p, err == nil && rel == name && !filepath.IsAbs(rel)
}
