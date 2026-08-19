package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
)

type countingChecker struct{ calls int }

func (c *countingChecker) Check(context.Context, []config.Skill, updatecheck.CheckOptions) (updatecheck.CheckReport, error) {
	c.calls++
	return updatecheck.CheckReport{}, nil
}

type failingChecker struct{}

func (failingChecker) Check(context.Context, []config.Skill, updatecheck.CheckOptions) (updatecheck.CheckReport, error) {
	return updatecheck.CheckReport{Results: []updatecheck.Result{{SkillID: "remote/demo", State: updatecheck.StateCheckFailed, Error: "offline"}}}, errors.New("remote check failed")
}

type availableChecker struct{}

func (availableChecker) Check(_ context.Context, skills []config.Skill, _ updatecheck.CheckOptions) (updatecheck.CheckReport, error) {
	results := make([]updatecheck.Result, 0, len(skills))
	for _, skill := range skills {
		results = append(results, updatecheck.Result{SkillID: skill.ID, Current: skill.Resolved, Remote: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", State: updatecheck.StateUpdateAvailable})
	}
	return updatecheck.CheckReport{Results: results}, nil
}

func TestSnapshotOfflineKeepsFilesystemStatusAndDoesNotFetch(t *testing.T) {
	aikitHome := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	checker := &countingChecker{}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Updates: checker})

	snapshot, err := application.Snapshot(context.Background(), StatusRequest{Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if checker.calls != 0 {
		t.Fatalf("offline snapshot made %d update calls", checker.calls)
	}
	if snapshot.Status.LibrarySkills != 0 || len(snapshot.Status.Targets) == 0 {
		t.Fatalf("filesystem status was not preserved: %+v", snapshot.Status)
	}
}

func TestSnapshotCheckFailureReturnsExistingStatus(t *testing.T) {
	aikitHome := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Updates: failingChecker{}})
	snapshot, err := application.Snapshot(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("checker failure escaped snapshot: %v", err)
	}
	if len(snapshot.Status.Targets) == 0 || snapshot.Exit != ExitPartial || len(snapshot.Updates.Results) != 1 {
		t.Fatalf("snapshot lost status on check failure: %+v", snapshot)
	}
}

func TestEnableGlobalAndProjectAgentScopes(t *testing.T) {
	application, paths, userHome, project := testApp(t)

	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	assertLink(t, filepath.Join(userHome, ".cursor", "skills", "demo"))
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Projects[0].AgentBindings["cursor"].Skills; len(got) != 1 || got[0] != "local/demo" {
		t.Fatalf("project agent binding = %v", got)
	}
	if _, err := os.Lstat(filepath.Join(project, ".cursor", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("same-id project link was not suppressed: %v", err)
	}
	if _, err := application.Disable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	assertLink(t, filepath.Join(project, ".cursor", "skills", "demo"))
}

type fakeLibrary struct {
	root             string
	added            config.Skill
	updateCalls      int
	commits          int
	aborts           int
	commitErr        error
	commitPanic      bool
	removeBatchPanic bool
}

type fakeMutation struct {
	owner   *fakeLibrary
	entries []config.Skill
	commit  func() error
}

func (mutation *fakeMutation) Entries() []config.Skill {
	return append([]config.Skill(nil), mutation.entries...)
}
func (mutation *fakeMutation) Commit(context.Context) error {
	mutation.owner.commits++
	if mutation.owner.commitPanic {
		panic("injected crash during commit")
	}
	if mutation.owner.commitErr != nil {
		return mutation.owner.commitErr
	}
	if mutation.commit != nil {
		return mutation.commit()
	}
	return nil
}

type recordingRecovery struct {
	ledgers [][]config.Skill
	err     error
}

type recoveryFunc func(context.Context, []config.Skill) ([]library.RecoveryIssue, error)

func (fn recoveryFunc) RecoverBatches(ctx context.Context, ledger []config.Skill) ([]library.RecoveryIssue, error) {
	return fn(ctx, ledger)
}

func (recovery *recordingRecovery) RecoverBatches(_ context.Context, ledger []config.Skill) ([]library.RecoveryIssue, error) {
	recovery.ledgers = append(recovery.ledgers, append([]config.Skill(nil), ledger...))
	return nil, recovery.err
}
func (mutation *fakeMutation) Abort() error { mutation.owner.aborts++; return nil }

func (f *fakeLibrary) PrepareAdd(context.Context, AddPrepareRequest, []config.Skill) (LibraryMutation, error) {
	entry := f.added
	return &fakeMutation{owner: f, entries: []config.Skill{entry}, commit: func() error { return f.install(entry) }}, nil
}
func (f *fakeLibrary) install(entry config.Skill) error {
	path := filepath.Join(f.root, filepath.FromSlash(entry.ID))
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: "+entry.Name+"\n---\n"), 0o644)
}
func (f *fakeLibrary) PrepareUpdate(_ context.Context, items []UpdatePrepareItem) (LibraryMutation, error) {
	f.updateCalls++
	entries := make([]config.Skill, len(items))
	for i, item := range items {
		entries[i] = item.Skill
		entries[i].Resolved = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		if item.Ref != nil {
			entries[i].Ref = item.Ref
		}
	}
	return &fakeMutation{owner: f, entries: entries}, nil
}
func (f *fakeLibrary) PrepareRemove(_ context.Context, skill config.Skill) (LibraryMutation, error) {
	return &fakeMutation{owner: f, entries: []config.Skill{skill}, commit: func() error {
		return os.RemoveAll(filepath.Join(f.root, filepath.FromSlash(skill.ID)))
	}}, nil
}

func (f *fakeLibrary) PrepareRemoveBatch(_ context.Context, skills []config.Skill) (LibraryMutation, error) {
	if f.removeBatchPanic {
		panic("injected crash during remove prepare")
	}
	return &fakeMutation{owner: f, entries: append([]config.Skill(nil), skills...), commit: func() error {
		for _, skill := range skills {
			if err := os.RemoveAll(filepath.Join(f.root, filepath.FromSlash(skill.ID))); err != nil {
				return err
			}
		}
		return nil
	}}, nil
}

func TestAddOnlyAndAddWithEnable(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	fake := &fakeLibrary{root: paths.LibrarySkills, added: config.Skill{ID: "local/one", Name: "one", Hash: "one"}}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake})
	if _, err := application.Add(context.Background(), AddRequest{Source: "git-source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(userHome, ".cursor", "skills", "one")); !os.IsNotExist(err) {
		t.Fatalf("add-only changed overlay: %v", err)
	}
	fake.added = config.Skill{ID: "local/two", Name: "two", Hash: "two"}
	if _, err := application.Add(context.Background(), AddRequest{Source: "git-source-2", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	assertLink(t, filepath.Join(userHome, ".cursor", "skills", "two"))
}

func TestAddExactDuplicateIsReportedAsSkippedWithoutCommit(t *testing.T) {
	_, paths, userHome, _ := testApp(t)
	existing := config.Skill{ID: "local/demo", Name: "demo", Hash: "hash"}
	fake := &fakeLibrary{root: paths.LibrarySkills, added: existing}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake})

	result, err := application.Add(context.Background(), AddRequest{Source: "same-source"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || len(result.Skills) != 0 || len(result.Skipped) != 1 || result.Skipped[0].ID != existing.ID {
		t.Fatalf("duplicate result = %+v", result)
	}
	if fake.commits != 0 || fake.aborts != 1 {
		t.Fatalf("duplicate mutation commits=%d aborts=%d", fake.commits, fake.aborts)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "Already in Library") {
		t.Fatalf("duplicate warning = %v", result.Warnings)
	}
}

func TestAddPreflightFailureAbortsWithoutCommittingLibrary(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	fake := &fakeLibrary{root: paths.LibrarySkills, added: config.Skill{ID: "local/one", Name: "one", Hash: "one"}}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake})
	if _, err := application.Add(context.Background(), AddRequest{Source: "source", Agent: "unknown"}); err == nil {
		t.Fatal("invalid scope accepted")
	}
	if fake.commits != 0 {
		t.Fatalf("library committed before final preflight: %d", fake.commits)
	}
}

func TestAddRejectsChangedDiscoveryTokenBeforeLedgerCheckpoint(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	fake := &fakeLibrary{root: paths.LibrarySkills, added: config.Skill{
		ID: "acme/repo/demo", Name: "demo", Source: "acme/repo", SourcePath: "skills/demo",
		Resolved: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Hash: "new-hash",
	}}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake})
	request := AddRequest{
		Source: "https://github.com/acme/repo.git", Skills: []string{"skills/demo"},
		ExpectedResolved:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedCandidates: []ExpectedAddCandidate{{Name: "demo", RelativePath: "skills/demo", Hash: "old-hash"}},
	}
	if _, err := application.Add(context.Background(), request); err == nil {
		t.Fatal("changed remote discovery token was accepted")
	}
	if fake.commits != 0 || fake.aborts != 1 {
		t.Fatalf("changed discovery reached commit: commits=%d aborts=%d", fake.commits, fake.aborts)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Library.Skills) != 0 {
		t.Fatalf("changed discovery reached ledger: %+v", cfg.Library.Skills)
	}
}

func TestUpdateRejectsChangedConfirmationTokenBeforePrepare(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	remote := config.Skill{ID: "github.com/acme/repo/demo", Name: "demo", Source: "github.com/acme/repo", SourcePath: ".", Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{remote}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLibrary{root: paths.LibrarySkills}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake, Updates: availableChecker{}})
	_, err := application.Update(context.Background(), UpdateRequest{SkillIDs: []string{remote.ID}, Confirmed: true, Expected: map[string]ExpectedUpdate{remote.ID: {Resolved: "cccccccccccccccccccccccccccccccccccccccc"}}})
	if err == nil {
		t.Fatal("stale confirmation token accepted")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("prepared update before token validation: %d", fake.updateCalls)
	}
}

func TestUpdateRejectsMissingPerSkillConfirmationToken(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	remote := config.Skill{ID: "github.com/acme/repo/demo", Name: "demo", Source: "github.com/acme/repo", SourcePath: ".", Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{remote}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLibrary{root: paths.LibrarySkills}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake, Updates: availableChecker{}})
	_, err := application.Update(context.Background(), UpdateRequest{SkillIDs: []string{remote.ID}, Confirmed: true})
	if err == nil {
		t.Fatal("missing confirmation token accepted")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("prepared update before token validation: %d", fake.updateCalls)
	}
}

func TestAddCommitErrorRestoresOldLedgerAndRecoversTowardIt(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	fake := &fakeLibrary{root: paths.LibrarySkills, added: config.Skill{ID: "local/new", Name: "new", Hash: "new"}, commitErr: errors.New("commit failed")}
	recovery := &recordingRecovery{}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake, LibraryRecovery: recovery})
	if _, err := application.Add(context.Background(), AddRequest{Source: "source"}); err == nil {
		t.Fatal("commit error lost")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Library.Skills) != 0 {
		t.Fatalf("new ledger survived failed commit: %+v", cfg.Library.Skills)
	}
	if len(recovery.ledgers) < 2 || len(recovery.ledgers[len(recovery.ledgers)-1]) != 0 {
		t.Fatalf("recovery ledger = %+v", recovery.ledgers)
	}
	if fake.aborts != 0 {
		t.Fatalf("Commit error incorrectly invoked Abort: %d", fake.aborts)
	}
}

func TestRemoveCommitErrorRestoresSkillLedgerWithoutAbort(t *testing.T) {
	application, paths, _, _ := testApp(t)
	fake := &fakeLibrary{root: paths.LibrarySkills, commitErr: errors.New("commit failed")}
	recovery := &recordingRecovery{}
	application.deps.Library = fake
	application.deps.LibraryRecovery = recovery
	if _, err := application.Remove(context.Background(), RemoveRequest{SkillID: "local/demo", Force: true}); err == nil {
		t.Fatal("commit error lost")
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skillIndex(cfg, "local/demo") < 0 {
		t.Fatal("skill ledger not restored")
	}
	if fake.aborts != 0 {
		t.Fatalf("Commit error incorrectly invoked Abort: %d", fake.aborts)
	}
	last := recovery.ledgers[len(recovery.ledgers)-1]
	if len(last) != 1 || last[0].ID != "local/demo" {
		t.Fatalf("recovery ledger = %+v", last)
	}
}

func TestRemovePreCommitCheckpointFailureDoesNotCommitLibrary(t *testing.T) {
	application, paths, _, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLibrary{root: paths.LibrarySkills}
	recovery := &recordingRecovery{}
	application.deps.Library = fake
	application.deps.LibraryRecovery = recovery
	ctx, cancel := context.WithCancel(context.Background())
	application.deps.Recover = func(_ string, operations []config.PendingOperation, _ link.Selector, _ bool) link.Result {
		if len(operations) > 0 {
			cancel()
			return link.Result{Completed: []string{operations[0].ID}}
		}
		return link.Result{}
	}
	if _, err := application.Remove(ctx, RemoveRequest{SkillID: "local/demo", Force: true}); err == nil {
		t.Fatal("checkpoint cancellation was ignored")
	}
	if fake.commits != 0 {
		t.Fatalf("library committed before pre-remove checkpoint: %d", fake.commits)
	}
}

func TestUpdateCommitErrorRestoresOldLedgerAndRecoversTowardIt(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	remote := config.Skill{
		ID: "github.com/acme/repo/demo", Name: "demo", Source: "github.com/acme/repo", SourcePath: ".",
		Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{remote}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLibrary{root: paths.LibrarySkills, commitErr: errors.New("commit failed")}
	recovery := &recordingRecovery{}
	application := New(Dependencies{
		Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome,
		Library: fake, LibraryRecovery: recovery, Updates: availableChecker{},
	})
	request := UpdateRequest{
		SkillIDs: []string{remote.ID}, Confirmed: true,
		Expected: map[string]ExpectedUpdate{remote.ID: {
			Ref: remote.Ref, Resolved: remote.Resolved, Remote: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}},
	}
	result, err := application.Update(context.Background(), request)
	if err == nil {
		t.Fatal("commit error lost")
	}
	if result.Exit != ExitPartial {
		t.Fatalf("commit error exit = %v", result.Exit)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Library.Skills[0].Resolved; got != remote.Resolved {
		t.Fatalf("old resolved object was not restored: %s", got)
	}
	if fake.aborts != 0 {
		t.Fatalf("Commit error incorrectly invoked Abort: %d", fake.aborts)
	}
	last := recovery.ledgers[len(recovery.ledgers)-1]
	if len(last) != 1 || last[0].Resolved != remote.Resolved {
		t.Fatalf("recovery ledger = %+v", last)
	}
}

func TestNextMutationRecoversLibraryTowardCurrentLedger(t *testing.T) {
	application, paths, _, _ := testApp(t)
	recovery := &recordingRecovery{}
	application.deps.LibraryRecovery = recovery
	if _, err := application.Sync(context.Background(), SyncRequest{Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	if len(recovery.ledgers) == 0 {
		t.Fatal("mutation skipped library recovery")
	}
	got := recovery.ledgers[0]
	if len(got) != 1 || got[0].ID != "local/demo" {
		t.Fatalf("recovery did not receive durable ledger: %+v (home %s)", got, paths.Home)
	}
}

func TestPresetMutationReconcilesAffectedEffectiveViews(t *testing.T) {
	application, _, userHome, _ := testApp(t)
	if _, err := application.PutPreset(context.Background(), PresetRequest{Name: "dev", Skills: []string{"local/demo"}, Create: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Enable(context.Background(), BindingRequest{Preset: "dev", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(userHome, ".cursor", "skills", "demo")
	assertLink(t, path)
	if _, err := application.PutPreset(context.Background(), PresetRequest{Name: "dev", Skills: []string{"local/demo"}, Remove: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("preset member removal was not reconciled: %v", err)
	}
}

func TestUpdateRequiresConfirmationThenPersistsSelectedRemote(t *testing.T) {
	aikitHome, userHome := t.TempDir(), t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	paths := config.PathsForHome(aikitHome)
	remote := config.Skill{ID: "github.com/acme/repo/demo", Name: "demo", Source: "github.com/acme/repo", SourcePath: ".", Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Hash: "old"}
	if err := os.MkdirAll(filepath.Join(paths.LibrarySkills, filepath.FromSlash(remote.ID)), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{remote}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLibrary{root: paths.LibrarySkills}
	application := New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome, Library: fake, Updates: availableChecker{}})
	result, err := application.Update(context.Background(), UpdateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != ExitUpdatesAvailable || fake.updateCalls != 0 {
		t.Fatalf("unconfirmed update = %+v calls=%d", result, fake.updateCalls)
	}
	result, err = application.Update(context.Background(), UpdateRequest{
		Confirmed: true,
		Expected: map[string]ExpectedUpdate{remote.ID: {
			Ref: remote.Ref, Resolved: remote.Resolved, Remote: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || fake.updateCalls != 1 {
		t.Fatalf("confirmed update = %+v calls=%d", result, fake.updateCalls)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Library.Skills[0].Resolved != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("resolved = %s", cfg.Library.Skills[0].Resolved)
	}
}

func TestRemoveProtectsReferencesAndForcePrunesAndUnlinks(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(userHome, ".cursor", "skills", "demo")
	if _, err := application.Remove(context.Background(), RemoveRequest{SkillID: "local/demo"}); err == nil {
		t.Fatal("referenced skill removal was accepted without force")
	}
	assertLink(t, linkPath)
	if _, err := application.Remove(context.Background(), RemoveRequest{SkillID: "local/demo", Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("forced removal left managed link: %v", err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Library.Skills) != 0 || contains(cfg.Agents["cursor"].Skills, "local/demo") {
		t.Fatalf("forced removal did not prune ledger: %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(paths.LibrarySkills, "local", "demo")); !os.IsNotExist(err) {
		t.Fatalf("forced removal left library content: %v", err)
	}
}

func TestForceRemoveRetainsLibraryWhenUnlinkFails(t *testing.T) {
	application, paths, _, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	application.deps.Recover = func(_ string, operations []config.PendingOperation, _ link.Selector, _ bool) link.Result {
		result := link.Result{}
		if len(operations) > 0 {
			result.Failures = []link.Issue{{Kind: link.IssueIO, Message: "injected unlink failure"}}
		}
		return result
	}
	result, err := application.Remove(context.Background(), RemoveRequest{SkillID: "local/demo", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != ExitPartial {
		t.Fatalf("exit = %v", result.Exit)
	}
	if _, err := os.Stat(filepath.Join(paths.LibrarySkills, "local", "demo")); err != nil {
		t.Fatalf("library content removed before unlink succeeded: %v", err)
	}
}

func TestProjectCommonBindingReconcilesEveryDeclaredAgent(t *testing.T) {
	application, _, _, project := testApp(t)
	if _, err := application.EditProject(context.Background(), ProjectEditRequest{Project: "demo", AddAgents: []string{"codex"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Project: "demo"}); err != nil {
		t.Fatal(err)
	}
	assertLink(t, filepath.Join(project, ".cursor", "skills", "demo"))
	assertLink(t, filepath.Join(project, ".codex", "skills", "demo"))
}

func TestSyncDryRunDoesNotModifyDiskOrPendingLedger(t *testing.T) {
	application, paths, userHome, _ := testApp(t)
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]config.Binding{}
	}
	cfg.Agents["cursor"] = config.Binding{Skills: []string{"local/demo"}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	result, err := application.Sync(context.Background(), SyncRequest{Agent: "cursor", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.Actions) == 0 {
		t.Fatal("dry-run returned no planned action")
	}
	if _, err := os.Lstat(filepath.Join(userHome, ".cursor", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("dry-run changed disk: %v", err)
	}
}

func TestEnableCheckpointsLedgerBeforeFilesystemReconcile(t *testing.T) {
	application, paths, _, _ := testApp(t)
	application.deps.Execute = func(plan link.Plan, _ bool) link.Result {
		cfg, err := (config.Store{Paths: paths}).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.Agents["cursor"].Skills; len(got) != 1 || got[0] != "local/demo" {
			t.Fatalf("ledger was not checkpointed before reconcile: %v", got)
		}
		return link.Result{Actions: plan.Actions, Failures: []link.Issue{{Kind: link.IssueIO, Message: "injected"}}}
	}
	result, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exit != ExitPartial {
		t.Fatalf("exit = %v", result.Exit)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !contains(cfg.Agents["cursor"].Skills, "local/demo") {
		t.Fatal("reconcile failure rolled back durable ledger")
	}
}

func TestSyncDoesNotImplicitlyRecoverPendingOperation(t *testing.T) {
	application, paths, _, project := testApp(t)
	op, err := link.NewCleanupOperation("cleanup-test", config.Scope{Project: "demo", ProjectPath: project, Agent: "cursor"}, filepath.Join(project, ".cursor", "skills", "demo"), "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	application.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		return link.Result{Issues: []link.Issue{{Kind: link.IssuePendingCleanup, Operation: op.ID, Message: "changed"}}}
	}
	if _, err := application.Sync(context.Background(), SyncRequest{Project: "demo", Agent: "cursor"}); err == nil {
		t.Fatal("sync accepted pending recovery")
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 1 || cfg.PendingOperations[0].ID != op.ID {
		t.Fatalf("failed pending operation was cleared: %+v", cfg.PendingOperations)
	}
}

func TestResumeRecoveryClearsOnlyCompletedPendingOperation(t *testing.T) {
	application, paths, _, project := testApp(t)
	op, err := link.NewCleanupOperation("cleanup-complete", config.Scope{Project: "demo", ProjectPath: project, Agent: "cursor"}, filepath.Join(project, ".cursor", "skills", "demo"), "local/demo", "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.PendingOperations = []config.PendingOperation{op}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := application.ResumeRecovery(context.Background(), RecoveryRequest{OperationIDs: []string{op.ID}, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PendingOperations) != 0 {
		t.Fatalf("completed pending operation remains: %+v", cfg.PendingOperations)
	}
}

func TestPresetMemberChangeRejectsCrossLayerConflictBeforeCheckpoint(t *testing.T) {
	application, paths, userHome, project := testApp(t)
	other := config.Skill{ID: "local/demo-other", Name: "demo", Hash: "other"}
	otherPath := filepath.Join(paths.LibrarySkills, "local", "demo-other")
	if err := os.MkdirAll(otherPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherPath, "SKILL.md"), []byte("---\nname: demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills = append(cfg.Library.Skills, other)
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.PutPreset(context.Background(), PresetRequest{Name: "project", Create: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Enable(context.Background(), BindingRequest{Preset: "project", Project: "demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.PutPreset(context.Background(), PresetRequest{Name: "project", Skills: []string{other.ID}}); err == nil {
		t.Fatal("preset mutation introduced a known cross-layer conflict")
	}
	cfg, err = (config.Store{Paths: paths}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	preset, err := findPreset(cfg, "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(preset.Skills) != 0 {
		t.Fatalf("conflicting preset member was checkpointed: %v", preset.Skills)
	}
	assertLink(t, filepath.Join(userHome, ".cursor", "skills", "demo"))
	if _, err := os.Lstat(filepath.Join(project, ".cursor", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("conflicting project link was written: %v", err)
	}
}

func testApp(t *testing.T) (*App, config.Paths, string, string) {
	t.Helper()
	aikitHome := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("AIKIT_HOME", aikitHome)
	t.Setenv("HOME", userHome)
	project := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := config.PathsForHome(aikitHome)
	libraryPath := filepath.Join(paths.LibrarySkills, "local", "demo")
	if err := os.MkdirAll(libraryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libraryPath, "SKILL.md"), []byte("---\nname: demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.New()
	cfg.Library.Skills = []config.Skill{{ID: "local/demo", Name: "demo", Hash: "hash"}}
	cfg.Projects = []config.Project{{Name: "demo", Path: project, Agents: []string{"cursor"}, AgentBindings: map[string]config.Binding{}}}
	if err := (config.Store{Paths: paths}).Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	return New(Dependencies{Store: config.Store{Paths: paths}, Paths: paths, UserHome: userHome}), paths, userHome, project
}

func assertLink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", path)
	}
}
