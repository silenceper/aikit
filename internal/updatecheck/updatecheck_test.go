package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/pkg/config"
)

const (
	oldOID = "1111111111111111111111111111111111111111"
	newOID = "2222222222222222222222222222222222222222"
)

type runnerCall struct {
	source string
	branch string
}

type fakeRunner struct {
	heads map[string]string
	errs  map[string]error
	calls []runnerCall
}

func (r *fakeRunner) RemoteBranchHead(_ context.Context, source, branch string) (string, error) {
	r.calls = append(r.calls, runnerCall{source: source, branch: branch})
	key := source + "\x00" + branch
	if err := r.errs[key]; err != nil {
		return "", err
	}
	return r.heads[key], nil
}

func branchSkill(id, source, branch, resolved string) config.Skill {
	return config.Skill{
		ID: id, Name: filepath.Base(id), Source: source, SourcePath: ".",
		Ref: &config.Ref{Kind: "branch", Value: branch}, Resolved: resolved,
	}
}

func TestCanonicalSourceNormalizesCommonGitForms(t *testing.T) {
	for _, source := range []string{
		"acme/skills",
		"https://GitHub.com/acme/skills.git/",
		"git@github.com:acme/skills.git",
		"ssh://git@github.com/acme/skills.git",
		"alice@gitlab.example.com:group/skills.git",
		"https://Gitea.Example.COM:443/team/skills.git",
	} {
		want, wantErr := library.NormalizeSource(source)
		got, gotErr := CanonicalSource(source)
		if (gotErr != nil) != (wantErr != nil) || got != want {
			t.Errorf("CanonicalSource(%q) = (%q, %v), library.NormalizeSource = (%q, %v)", source, got, gotErr, want, wantErr)
		}
	}
}

func TestCacheKeyContainsEveryRefTupleComponent(t *testing.T) {
	branch := cacheKey("github.com/acme/skills", &config.Ref{Kind: "branch", Value: "v1"})
	tag := cacheKey("github.com/acme/skills", &config.Ref{Kind: "tag", Value: "v1"})
	otherSource := cacheKey("github.com/acme/other", &config.Ref{Kind: "branch", Value: "v1"})
	otherValue := cacheKey("github.com/acme/skills", &config.Ref{Kind: "branch", Value: "v2"})
	if branch == tag || branch == otherSource || branch == otherValue {
		t.Fatalf("cache keys collided: branch=%q tag=%q source=%q value=%q", branch, tag, otherSource, otherValue)
	}
}

func TestCheckSharesCacheAcrossEquivalentCanonicalSources(t *testing.T) {
	runner := &fakeRunner{heads: map[string]string{"https://github.com/acme/skills.git\x00main": newOID}}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	if _, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/first", "https://github.com/acme/skills.git", "main", oldOID),
	}, CheckOptions{}); err != nil {
		t.Fatal(err)
	}
	results, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/second", "git@github.com:acme/skills.git", "main", oldOID),
	}, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("equivalent source missed canonical cache entry: %d calls", len(runner.calls))
	}
	if !results.Results[0].FromCache || results.Results[0].Remote != newOID {
		t.Fatalf("canonical cache result = %#v", results.Results[0])
	}
}

type commandCall struct {
	dir  string
	args []string
}

type fakeCommandExecutor struct {
	calls    []commandCall
	fetchErr error
	origin   string
}

func (executor *fakeCommandExecutor) Run(_ context.Context, dir string, args ...string) (string, error) {
	executor.calls = append(executor.calls, commandCall{dir: dir, args: append([]string(nil), args...)})
	if len(args) > 0 && args[0] == "fetch" && executor.fetchErr != nil {
		return "", executor.fetchErr
	}
	if reflect.DeepEqual(args, []string{"remote", "get-url", "origin"}) {
		return executor.origin + "\n", nil
	}
	if len(args) > 0 && args[0] == "rev-parse" {
		return newOID + "\n", nil
	}
	return "", nil
}

func TestCommandGitRunnerRedactsAndLimitsFetchErrors(t *testing.T) {
	cacheRoot := t.TempDir()
	repository, err := library.RepoCachePath(cacheRoot, "gitlab.example.com/group/repo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	executor := &fakeCommandExecutor{
		origin: "https://gitlab.example.com/group/repo.git",
		fetchErr: errors.New(
			"fatal: unable to access 'https://alice:ghp_supersecret@gitlab.example.com/group/repo.git?access_token=querysecret&private_token=anothersecret'; Authorization: Bearer bearerSecret: " + strings.Repeat("x", 5000),
		),
	}
	runner := CommandGitRunner{CacheRoot: cacheRoot, Executor: executor}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	report, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("gitlab/skill", "gitlab.example.com/group/repo", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].State != StateCheckFailed {
		t.Fatalf("fetch failure report = %#v", report)
	}
	message := report.Results[0].Error
	for _, secret := range []string{"alice", "ghp_supersecret", "querysecret", "anothersecret", "bearerSecret"} {
		if strings.Contains(message, secret) {
			t.Errorf("Result.Error leaked %q: %q", secret, message)
		}
	}
	if !strings.Contains(message, "REDACTED") {
		t.Errorf("Result.Error did not mark redaction: %q", message)
	}
	if len(message) > 1400 {
		t.Errorf("Result.Error was not bounded: %d bytes", len(message))
	}
}

func TestCheckRedactsCredentialsFromMalformedCanonicalSource(t *testing.T) {
	runner := &fakeRunner{}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	report, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("broken/source", "https://alice:supersecret@gitlab.example.com/group/%zz.git", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].State != StateCheckFailed {
		t.Fatalf("malformed source result = %#v", report)
	}
	message := report.Results[0].Error
	for _, secret := range []string{"alice", "supersecret"} {
		if strings.Contains(message, secret) {
			t.Errorf("canonical source error leaked %q: %q", secret, message)
		}
	}
	if !strings.Contains(message, "REDACTED") {
		t.Errorf("canonical source error did not mark redaction: %q", message)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("malformed source reached runner: %#v", runner.calls)
	}
}

func TestDirectAPIErrorsRedactCredentials(t *testing.T) {
	malformed := "https://alice:supersecret@gitlab.example.com/group/%zz.git"
	if _, err := CanonicalSource(malformed); err == nil {
		t.Fatal("CanonicalSource accepted malformed source")
	} else if strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("CanonicalSource leaked credentials: %q", err)
	}
	runner := CommandGitRunner{CacheRoot: t.TempDir(), Executor: &fakeCommandExecutor{}}
	if _, err := runner.RemoteBranchHead(context.Background(), malformed, "main"); err == nil {
		t.Fatal("RemoteBranchHead accepted malformed source")
	} else if strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("RemoteBranchHead leaked credentials: %q", err)
	}
}

func TestSanitizeGitErrorClearsURLQueryFragmentAndAuthorizationForms(t *testing.T) {
	err := errors.New("fatal https://user:pass@example.com/repo.git?X-Amz-Credential=awssecret&innocent=stillsecret#fragmentsecret\nAuthorization=plainSecret\nAuthorization: token tokenSecret\nAuthorization: AWS4-HMAC-SHA256 Credential=awsHeaderSecret,SignedHeaders=host,Signature=signatureSecret")
	message := sanitizeGitError(err)
	for _, secret := range []string{"user", "pass", "awssecret", "stillsecret", "fragmentsecret", "plainSecret", "tokenSecret", "awsHeaderSecret", "signatureSecret"} {
		if strings.Contains(message, secret) {
			t.Errorf("sanitized error leaked %q: %q", secret, message)
		}
	}
	if !strings.Contains(message, "REDACTED") {
		t.Fatalf("sanitized error did not mark redaction: %q", message)
	}
}

func TestMaliciousRemoteObjectIDIsNotEchoed(t *testing.T) {
	malicious := "https://user:secret@example.com/repo?token=querysecret#fragmentsecret" + strings.Repeat("x", 5000)
	runner := &fakeRunner{heads: map[string]string{"acme/skills\x00main": malicious}}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	report, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/skill", "acme/skills", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	message := report.Results[0].Error
	if message != "remote did not return a full hexadecimal object id" {
		t.Fatalf("malicious object id was reflected: %q", message)
	}
}

type branchBarrierRunner struct {
	ready   chan string
	release <-chan struct{}
	heads   map[string]string
}

func (runner *branchBarrierRunner) RemoteBranchHead(ctx context.Context, _ string, branch string) (string, error) {
	select {
	case runner.ready <- branch:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case <-runner.release:
		return runner.heads[branch], nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestConcurrentCheckerInstancesMergeDifferentCacheEntries(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), ".update-check")
	ready := make(chan string, 2)
	release := make(chan struct{})
	runner := &branchBarrierRunner{ready: ready, release: release, heads: map[string]string{"main": newOID, "release": oldOID}}
	checkers := []*Checker{New(cachePath, runner), New(cachePath, runner)}
	branches := []string{"main", "release"}
	errorsByCheck := make(chan error, 2)
	start := make(chan struct{})
	for i := range checkers {
		go func(i int) {
			<-start
			_, checkErr := checkers[i].Check(context.Background(), []config.Skill{
				branchSkill("acme/"+branches[i], "acme/skills", branches[i], oldOID),
			}, CheckOptions{ForceRefresh: true})
			errorsByCheck <- checkErr
		}(i)
	}
	close(start)
	<-ready
	<-ready
	close(release)
	for range checkers {
		if err := <-errorsByCheck; err != nil {
			t.Fatal(err)
		}
	}
	disk := loadCacheForTest(t, cachePath)
	if len(disk.Entries) != 2 {
		t.Fatalf("concurrent cache writes lost an entry: %#v", disk.Entries)
	}
}

type delayedOIDRunner struct {
	started chan struct{}
	release chan struct{}
	oid     string
}

func (runner *delayedOIDRunner) RemoteBranchHead(ctx context.Context, _, _ string) (string, error) {
	close(runner.started)
	select {
	case <-runner.release:
		return runner.oid, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestConcurrentCheckerInstancesDoNotOverwriteNewerSameKeyEntry(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), ".update-check")
	olderTime := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	newerTime := olderTime.Add(time.Minute)
	olderRunner := &delayedOIDRunner{started: make(chan struct{}), release: make(chan struct{}), oid: oldOID}
	olderChecker := New(cachePath, olderRunner, WithNow(func() time.Time { return olderTime }))
	newerRunner := &fakeRunner{heads: map[string]string{"acme/skills\x00main": newOID}}
	newerChecker := New(cachePath, newerRunner, WithNow(func() time.Time { return newerTime }))
	skill := branchSkill("acme/skill", "acme/skills", "main", oldOID)

	olderDone := make(chan error, 1)
	go func() {
		_, err := olderChecker.Check(context.Background(), []config.Skill{skill}, CheckOptions{ForceRefresh: true})
		olderDone <- err
	}()
	<-olderRunner.started
	if _, err := newerChecker.Check(context.Background(), []config.Skill{skill}, CheckOptions{ForceRefresh: true}); err != nil {
		t.Fatal(err)
	}
	close(olderRunner.release)
	if err := <-olderDone; err != nil {
		t.Fatal(err)
	}

	disk := loadCacheForTest(t, cachePath)
	entry := disk.Entries[cacheKey("acme/skills", skill.Ref)]
	if entry.Remote != newOID || !entry.CheckedAt.Equal(newerTime) {
		t.Fatalf("older concurrent result overwrote newer cache entry: %#v", entry)
	}
}

func TestEqualTimestampCacheMergeChoosesOIDDeterministically(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), ".update-check")
	checkedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	ref := &config.Ref{Kind: "branch", Value: "main"}
	key := cacheKey("acme/skills", ref)
	entry := func(oid string) cacheEntry {
		return cacheEntry{Source: "acme/skills", RefKind: ref.Kind, RefValue: ref.Value, Remote: oid, CheckedAt: checkedAt}
	}

	for _, order := range [][]string{{oldOID, newOID}, {newOID, oldOID}} {
		if err := mergeAndWriteCache(context.Background(), cachePath, map[string]cacheEntry{key: entry(order[0])}); err != nil {
			t.Fatal(err)
		}
		if err := mergeAndWriteCache(context.Background(), cachePath, map[string]cacheEntry{key: entry(order[1])}); err != nil {
			t.Fatal(err)
		}
		if got := loadCacheForTest(t, cachePath).Entries[key].Remote; got != newOID {
			t.Fatalf("equal-timestamp merge depends on arrival order %v: got %q", order, got)
		}
		if err := os.Remove(cachePath); err != nil {
			t.Fatal(err)
		}
	}
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
	calls   int
	mu      sync.Mutex
}

func (runner *blockingRunner) RemoteBranchHead(ctx context.Context, _, _ string) (string, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()
	select {
	case runner.started <- struct{}{}:
	default:
	}
	select {
	case <-runner.release:
		return newOID, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestCheckCancellationWhileWaitingForInstanceGate(t *testing.T) {
	runner := &blockingRunner{started: make(chan struct{}, 1), release: make(chan struct{})}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	firstDone := make(chan error, 1)
	go func() {
		_, err := checker.Check(context.Background(), []config.Skill{branchSkill("acme/one", "acme/skills", "main", oldOID)}, CheckOptions{ForceRefresh: true})
		firstDone <- err
	}()
	<-runner.started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := checker.Check(ctx, []config.Skill{branchSkill("acme/two", "acme/skills", "release", oldOID)}, CheckOptions{ForceRefresh: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("gate wait error = %v, want context canceled", err)
	}
	close(runner.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.calls != 1 {
		t.Fatalf("canceled gate waiter called runner %d times", runner.calls)
	}
}

func TestCacheFileLockWaitHonorsContextCancellation(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".update-check.lock")
	unlock, err := acquireCacheLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	secondUnlock, err := acquireCacheLock(ctx, lockPath)
	if secondUnlock != nil {
		secondUnlock()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock wait error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled lock wait returned too slowly: %v", elapsed)
	}
}

type cancelAfterFirstRunner struct {
	cancel context.CancelFunc
	calls  int
}

func (runner *cancelAfterFirstRunner) RemoteBranchHead(_ context.Context, _, _ string) (string, error) {
	runner.calls++
	runner.cancel()
	return newOID, nil
}

func TestCheckCancellationStopsSkillLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &cancelAfterFirstRunner{cancel: cancel}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	report, err := checker.Check(ctx, []config.Skill{
		branchSkill("acme/one", "acme/one", "main", oldOID),
		branchSkill("acme/two", "acme/two", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("loop cancellation error = %v, want context canceled", err)
	}
	if runner.calls != 1 || len(report.Results) != 1 {
		t.Fatalf("canceled loop continued: calls=%d report=%#v", runner.calls, report)
	}
}

func loadCacheForTest(t *testing.T, path string) cacheFile {
	t.Helper()
	cache, corrupt, err := loadCache(path)
	if err != nil || corrupt {
		t.Fatalf("load cache: corrupt=%v err=%v", corrupt, err)
	}
	return cache
}

type concurrentCommandExecutor struct {
	mu     sync.Mutex
	calls  []commandCall
	origin string
}

func (executor *concurrentCommandExecutor) Run(_ context.Context, dir string, args ...string) (string, error) {
	executor.mu.Lock()
	executor.calls = append(executor.calls, commandCall{dir: dir, args: append([]string(nil), args...)})
	executor.mu.Unlock()
	if reflect.DeepEqual(args, []string{"remote", "get-url", "origin"}) {
		return executor.origin + "\n", nil
	}
	if len(args) > 0 && args[0] == "rev-parse" {
		return newOID + "\n", nil
	}
	return "", nil
}

func TestConcurrentCommandGitRunnerChecksNeverFetchPersistentRepository(t *testing.T) {
	cacheRoot := t.TempDir()
	repository, err := library.RepoCachePath(cacheRoot, "acme/skills")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	executor := &concurrentCommandExecutor{origin: "ssh://git@example.com:2222/acme/skills.git"}
	runner := CommandGitRunner{CacheRoot: cacheRoot, Executor: executor}

	var wait sync.WaitGroup
	errorsByBranch := make(chan error, 2)
	for _, branch := range []string{"main", "release"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, runErr := runner.RemoteBranchHead(context.Background(), "acme/skills", branch)
			errorsByBranch <- runErr
		}()
	}
	wait.Wait()
	close(errorsByBranch)
	for runErr := range errorsByBranch {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}

	executor.mu.Lock()
	calls := append([]commandCall(nil), executor.calls...)
	executor.mu.Unlock()
	fetchDirs := make(map[string]struct{})
	for _, call := range calls {
		if call.dir == repository && !reflect.DeepEqual(call.args, []string{"remote", "get-url", "origin"}) {
			t.Fatalf("persistent repository received a write-capable command: %#v", call)
		}
		if len(call.args) > 0 && call.args[0] == "fetch" {
			if call.dir == repository {
				t.Fatal("concurrent check fetched in persistent repository")
			}
			fetchDirs[call.dir] = struct{}{}
		}
	}
	if len(fetchDirs) != 2 {
		t.Fatalf("concurrent checks did not use independent temporary repositories: %#v", fetchDirs)
	}
	for directory := range fetchDirs {
		if _, err := os.Lstat(directory); !os.IsNotExist(err) {
			t.Fatalf("temporary repository was not removed: %q err=%v", directory, err)
		}
	}
}

func TestCommandGitRunnerFetchesCanonicalNonGitHubSources(t *testing.T) {
	for _, test := range []struct {
		name      string
		source    string
		canonical string
	}{
		{name: "gitlab", source: "https://gitlab.example.com/group/repo.git", canonical: "gitlab.example.com/group/repo"},
		{name: "gitea with port", source: "https://gitea.example.com:8443/team/repo.git", canonical: "gitea.example.com:8443/team/repo"},
		{name: "ssh with non-default port", source: "ssh://git@gitlab.example.com:2222/group/repo.git", canonical: "gitlab.example.com:2222/group/repo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := &fakeCommandExecutor{origin: test.source}
			cacheRoot := t.TempDir()
			repoPath, err := library.RepoCachePath(cacheRoot, test.canonical)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(repoPath, 0o700); err != nil {
				t.Fatal(err)
			}
			runner := CommandGitRunner{CacheRoot: cacheRoot, Executor: executor}
			got, err := runner.RemoteBranchHead(context.Background(), test.source, "main")
			if err != nil {
				t.Fatal(err)
			}
			if got != newOID {
				t.Fatalf("RemoteBranchHead() = %q, want %q", got, newOID)
			}

			var readOrigin, cloned, setOrigin, fetched, resolved bool
			fetchIndex, resolveIndex := -1, -1
			var temporary string
			for i, call := range executor.calls {
				if call.dir == repoPath && reflect.DeepEqual(call.args, []string{"remote", "get-url", "origin"}) {
					readOrigin = true
				}
				if call.dir == repoPath && len(call.args) > 0 && call.args[0] != "remote" {
					t.Fatalf("persistent repository received a write-capable command: %#v", call)
				}
				if len(call.args) >= 6 && call.args[0] == "clone" && call.args[1] == "--mirror" && call.args[2] == "--no-local" && call.args[4] == repoPath {
					cloned = true
					temporary = call.args[5]
				}
				if len(call.args) == 4 && call.args[0] == "remote" && call.args[1] == "set-url" && call.args[2] == "origin" && call.args[3] == test.source {
					setOrigin = true
				}
				if len(call.args) >= 5 && call.args[0] == "fetch" && call.args[3] == "origin" && call.args[4] == "+refs/heads/main:refs/remotes/aikit/main" {
					fetched = true
					fetchIndex = i
					if call.dir == repoPath {
						t.Fatal("fetch mutated persistent repository")
					}
				}
				if reflect.DeepEqual(call.args, []string{"rev-parse", "--verify", "refs/remotes/aikit/main^{commit}"}) {
					resolved = true
					resolveIndex = i
				}
			}
			if !readOrigin || !cloned || !setOrigin || !fetched || !resolved || fetchIndex >= resolveIndex {
				t.Fatalf("runner did not fetch/resolve in order: %#v", executor.calls)
			}
			if temporary == "" {
				t.Fatal("runner did not expose a temporary mirror destination")
			}
			if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
				t.Fatalf("temporary mirror was not removed: %q err=%v", temporary, err)
			}
		})
	}
}

func TestCommandGitRunnerRequiresExistingSharedRepository(t *testing.T) {
	cacheRoot := t.TempDir()
	executor := &fakeCommandExecutor{}
	runner := CommandGitRunner{CacheRoot: cacheRoot, Executor: executor}
	if _, err := runner.RemoteBranchHead(context.Background(), "acme/skills", "main"); err == nil || !strings.Contains(err.Error(), "re-add") {
		t.Fatalf("missing shared repository error = %v, want re-add guidance", err)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("runner invoked git before rejecting unsafe cache: %#v", executor.calls)
	}
}

func TestCommandGitRunnerRejectsEmptyCacheRoot(t *testing.T) {
	executor := &fakeCommandExecutor{}
	runner := CommandGitRunner{Executor: executor}
	if _, err := runner.RemoteBranchHead(context.Background(), "acme/skills", "main"); err == nil || !strings.Contains(err.Error(), "cache root") {
		t.Fatalf("empty cache root error = %v", err)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("runner invoked git with empty cache root: %#v", executor.calls)
	}
}

func TestMissingSharedRepositoryBecomesSingleCheckFailure(t *testing.T) {
	executor := &fakeCommandExecutor{}
	runner := CommandGitRunner{CacheRoot: t.TempDir(), Executor: executor}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	report, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/skill", "acme/skills", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatalf("missing shared repository became top-level error: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].State != StateCheckFailed || !strings.Contains(report.Results[0].Error, "re-add") {
		t.Fatalf("missing repository report = %#v", report)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("missing repository invoked git: %#v", executor.calls)
	}
}

func TestCommandGitRunnerRejectsUnsafeFetchTargetBeforeGit(t *testing.T) {
	for _, branch := range []string{"main:refs/heads/injected", "feature name", "@"} {
		executor := &fakeCommandExecutor{}
		runner := CommandGitRunner{CacheRoot: t.TempDir(), Executor: executor}
		if _, err := runner.RemoteBranchHead(context.Background(), "acme/skills", branch); err == nil || !strings.Contains(err.Error(), "invalid branch") {
			t.Errorf("unsafe fetch target %q error = %v, want invalid branch", branch, err)
		}
		if len(executor.calls) != 0 {
			t.Errorf("unsafe fetch target %q invoked git: %#v", branch, executor.calls)
		}
	}
}

func TestCheckCacheKeyIncludesCanonicalSourceKindAndValue(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	runner := &fakeRunner{heads: map[string]string{
		"https://github.com/acme/skills.git\x00main": newOID,
		"git@github.com:acme/skills.git\x00release":  oldOID,
	}}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner, WithNow(func() time.Time { return now }))
	skills := []config.Skill{
		branchSkill("acme/main", "https://github.com/acme/skills.git", "main", oldOID),
		branchSkill("acme/release", "git@github.com:acme/skills.git", "release", oldOID),
	}

	first, err := checker.Check(context.Background(), skills, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("first check runner calls = %d, want 2", len(runner.calls))
	}
	if first.Results[0].State != StateUpdateAvailable || first.Results[0].Remote != newOID {
		t.Fatalf("main result = %#v", first.Results[0])
	}
	if first.Results[1].State != StateCurrent || first.Results[1].Remote != oldOID {
		t.Fatalf("release result = %#v", first.Results[1])
	}

	second, err := checker.Check(context.Background(), skills, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("fresh cache made runner calls: got %d, want 2 total", len(runner.calls))
	}
	if !second.Results[0].FromCache || !second.Results[1].FromCache {
		t.Fatalf("second results not served from independent cache entries: %#v", second)
	}
}

func TestCheckUsesTenMinuteTTLAndForceRefresh(t *testing.T) {
	current := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	runner := &fakeRunner{heads: map[string]string{"acme/skills\x00main": newOID}}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner, WithNow(func() time.Time { return current }))
	skills := []config.Skill{branchSkill("acme/skill", "acme/skills", "main", oldOID)}

	if _, err := checker.Check(context.Background(), skills, CheckOptions{}); err != nil {
		t.Fatal(err)
	}
	current = current.Add(9*time.Minute + 59*time.Second)
	if _, err := checker.Check(context.Background(), skills, CheckOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("cache younger than ten minutes was not reused: %d calls", len(runner.calls))
	}

	if _, err := checker.Check(context.Background(), skills, CheckOptions{ForceRefresh: true}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("forced refresh did not call runner: %d calls", len(runner.calls))
	}

	current = current.Add(time.Second)
	if _, err := checker.Check(context.Background(), skills, CheckOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("forced refresh did not replace cache timestamp: %d calls", len(runner.calls))
	}

	current = current.Add(10 * time.Minute)
	if _, err := checker.Check(context.Background(), skills, CheckOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("cache at TTL boundary was reused: %d calls", len(runner.calls))
	}
}

func TestCheckReturnsFullObjectIDsAndRejectsAbbreviatedRunnerResult(t *testing.T) {
	runner := &fakeRunner{heads: map[string]string{
		"acme/skills\x00main":  newOID,
		"acme/skills\x00short": "2222222",
	}}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	results, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/good", "acme/skills", "main", oldOID),
		branchSkill("acme/bad", "acme/skills", "short", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if results.Results[0].Remote != newOID || len(results.Results[0].Remote) != 40 {
		t.Fatalf("full object id not preserved: %#v", results.Results[0])
	}
	if results.Results[1].State != StateCheckFailed || !strings.Contains(results.Results[1].Error, "full hexadecimal object id") {
		t.Fatalf("abbreviated object id accepted: %#v", results.Results[1])
	}
}

func TestCheckSkipsPinnedLocalAndOfflineEntries(t *testing.T) {
	runner := &fakeRunner{}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	results, err := checker.Check(context.Background(), []config.Skill{
		{ID: "local/one", Name: "one"},
		{ID: "acme/tag", Name: "tag", Source: "acme/skills", SourcePath: ".", Ref: &config.Ref{Kind: "tag", Value: "v1"}, Resolved: oldOID},
		{ID: "acme/commit", Name: "commit", Source: "acme/skills", SourcePath: ".", Ref: &config.Ref{Kind: "commit", Value: oldOID}, Resolved: oldOID},
		branchSkill("acme/branch", "acme/skills", "main", oldOID),
	}, CheckOptions{Offline: true, ForceRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("offline check made %d runner calls", len(runner.calls))
	}
	want := []State{StateLocal, StatePinned, StatePinned, StateOffline}
	for i := range want {
		if results.Results[i].State != want[i] {
			t.Errorf("result[%d].State = %q, want %q", i, results.Results[i].State, want[i])
		}
	}
}

func TestCheckFailureDoesNotBlockOtherEntries(t *testing.T) {
	fetchErr := errors.New("remote unavailable")
	runner := &fakeRunner{
		heads: map[string]string{"acme/good\x00main": newOID},
		errs:  map[string]error{"acme/bad\x00main": fetchErr},
	}
	checker := New(filepath.Join(t.TempDir(), ".update-check"), runner)
	results, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/bad", "acme/bad", "main", oldOID),
		branchSkill("acme/good", "acme/good", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatalf("single fetch error became top-level error: %v", err)
	}
	if results.Results[0].State != StateCheckFailed || !strings.Contains(results.Results[0].Error, fetchErr.Error()) {
		t.Fatalf("failed result = %#v", results.Results[0])
	}
	if results.Results[1].State != StateUpdateAvailable || results.Results[1].Remote != newOID {
		t.Fatalf("successful result after failure = %#v", results.Results[1])
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner stopped after failure: %d calls", len(runner.calls))
	}
}

func TestCorruptCacheIsRebuiltWithAtomicRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".update-check")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{heads: map[string]string{"acme/skills\x00main": newOID}}
	checker := New(path, runner)
	if _, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/skill", "acme/skills", "main", oldOID),
	}, CheckOptions{}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk cacheFile
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("cache was not rebuilt as JSON: %v", err)
	}
	if len(disk.Entries) != 1 {
		t.Fatalf("rebuilt cache entries = %d, want 1", len(disk.Entries))
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".update-check.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("atomic cache write left temporary files: %v", temps)
	}
}

func TestCheckAtomicallyRebuildsCorruptCacheWithoutSuccessfulFetch(t *testing.T) {
	for _, test := range []struct {
		name      string
		skill     config.Skill
		options   CheckOptions
		runner    *fakeRunner
		wantState State
	}{
		{name: "offline", skill: branchSkill("acme/offline", "acme/skills", "main", oldOID), options: CheckOptions{Offline: true}, runner: &fakeRunner{}, wantState: StateOffline},
		{name: "pinned", skill: config.Skill{ID: "acme/pin", Name: "pin", Source: "acme/skills", SourcePath: ".", Ref: &config.Ref{Kind: "tag", Value: "v1"}, Resolved: oldOID}, runner: &fakeRunner{}, wantState: StatePinned},
		{name: "fetch failed", skill: branchSkill("acme/fail", "acme/skills", "main", oldOID), runner: &fakeRunner{errs: map[string]error{"acme/skills\x00main": errors.New("fetch failed")}}, wantState: StateCheckFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".update-check")
			if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
				t.Fatal(err)
			}
			checker := New(path, test.runner)
			results, err := checker.Check(context.Background(), []config.Skill{test.skill}, test.options)
			if err != nil {
				t.Fatalf("corrupt cache rebuild blocked check: %v", err)
			}
			if results.Results[0].State != test.wantState {
				t.Fatalf("result state = %q, want %q", results.Results[0].State, test.wantState)
			}
			if test.options.Offline && len(test.runner.calls) != 0 {
				t.Fatalf("offline check used runner: %d calls", len(test.runner.calls))
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var rebuilt cacheFile
			if err := json.Unmarshal(data, &rebuilt); err != nil || rebuilt.Version != cacheVersion || rebuilt.Entries == nil {
				t.Fatalf("check did not rebuild corrupt cache: cache=%#v err=%v", rebuilt, err)
			}
			temps, err := filepath.Glob(filepath.Join(dir, ".update-check.tmp-*"))
			if err != nil || len(temps) != 0 {
				t.Fatalf("rebuild left temp files: %v (glob error %v)", temps, err)
			}
		})
	}
}

func TestCacheWriteFailureReturnsReportWithWarning(t *testing.T) {
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{heads: map[string]string{"acme/skills\x00main": newOID}}
	checker := New(filepath.Join(blockingFile, ".update-check"), runner)
	report, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/skill", "acme/skills", "main", oldOID),
	}, CheckOptions{ForceRefresh: true})
	if err != nil {
		t.Fatalf("cache write failure became top-level error: %v", err)
	}
	if report.Results[0].State != StateUpdateAvailable || report.Results[0].Remote != newOID {
		t.Fatalf("cache write failure discarded remote result: %#v", report.Results[0])
	}
	if len(report.Warnings) == 0 || !strings.Contains(strings.Join(report.Warnings, " "), "cache") {
		t.Fatalf("cache write failure was not reported as warning: %#v", report)
	}
}

func TestEmptySkillsStillReturnCacheWarnings(t *testing.T) {
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	checker := New(filepath.Join(blockingFile, ".update-check"), &fakeRunner{})
	report, err := checker.Check(context.Background(), nil, CheckOptions{})
	if err != nil {
		t.Fatalf("cache read warning became top-level error: %v", err)
	}
	if len(report.Results) != 0 || len(report.Warnings) == 0 {
		t.Fatalf("empty check report = %#v, want no results and a cache warning", report)
	}
}

func TestSemanticallyCorruptCacheEntryIsRefetched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".update-check")
	ref := &config.Ref{Kind: "branch", Value: "main"}
	corrupt := cacheFile{Version: cacheVersion, Entries: map[string]cacheEntry{
		cacheKey("acme/skills", ref): {
			Source: "acme/skills", RefKind: "branch", RefValue: "main",
			Remote: "2222222", CheckedAt: time.Now(),
		},
	}}
	data, err := json.Marshal(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{heads: map[string]string{"acme/skills\x00main": newOID}}
	checker := New(path, runner)
	results, err := checker.Check(context.Background(), []config.Skill{
		branchSkill("acme/skill", "acme/skills", "main", oldOID),
	}, CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || results.Results[0].Remote != newOID || results.Results[0].FromCache {
		t.Fatalf("invalid cached object id was trusted: calls=%d result=%#v", len(runner.calls), results.Results[0])
	}
}
