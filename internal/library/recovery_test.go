package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedBatchJournalAllowsSafeRecovery(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: filepath.Join(root, "skill")}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Journal == "" {
		t.Fatal("PrepareBatch did not persist a recovery journal")
	}
	if _, err := os.Stat(batch.Journal); err != nil {
		t.Fatalf("recovery journal missing: %v", err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("prepared recovery issues = %#v", issues)
	}
	if _, err := os.Lstat(batch.Copies[0].Staging); !os.IsNotExist(err) {
		t.Fatalf("owned staging was not removed: %v", err)
	}
	if _, err := os.Lstat(batch.Journal); !os.IsNotExist(err) {
		t.Fatalf("completed journal was not removed: %v", err)
	}
}

func TestRecoverMutatingBatchDoesNotClobberLaterDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Abort()
	if err := batch.prepareMutationJournal(); err != nil {
		t.Fatal(err)
	}
	copy := &batch.Copies[0]
	if err := moveNoReplace(destination, copy.Backup); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(copy.Staging, destination); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(destination, destination+"-installed-aside"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "later", 0o644)

	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("concurrent replacement was not reported")
	}
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(content) != "later" {
		t.Fatalf("recovery clobbered later destination: %q %v", content, err)
	}
	old, err := os.ReadFile(filepath.Join(copy.Backup, "SKILL.md"))
	if err != nil || string(old) != "old" {
		t.Fatalf("recovery did not retain old backup: %q %v", old, err)
	}
}

func TestRecoverMutatingBatchRollsBackAuthenticatedTrees(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.prepareMutationJournal(); err != nil {
		t.Fatal(err)
	}
	copy := &batch.Copies[0]
	if err := moveNoReplace(destination, copy.Backup); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(copy.Staging, destination); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("recovery = %#v, %v", issues, err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(content) != "old" {
		t.Fatalf("old destination was not restored: %q %v", content, err)
	}
	if _, err := os.Lstat(batch.Journal); !os.IsNotExist(err) {
		t.Fatalf("recovered journal remains: %v", err)
	}
}

func TestRecoverPreservesTreesForTamperedJournal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: filepath.Join(root, "skill")}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(batch.Journal)
	if err != nil {
		t.Fatal(err)
	}
	payload = bytes.Replace(payload, []byte(`"phase": "prepared"`), []byte(`"phase": "committed"`), 1)
	if err := os.WriteFile(batch.Journal, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil || len(issues) == 0 {
		t.Fatalf("tampered recovery = %#v, %v", issues, err)
	}
	if _, err := os.Stat(filepath.Join(batch.Copies[0].Staging, "SKILL.md")); err != nil {
		t.Fatalf("tampered journal caused deletion: %v", err)
	}
}

func TestRecoverReportsUnknownBatchDirectoryWithoutDeletingIt(t *testing.T) {
	root := t.TempDir()
	unknown := filepath.Join(root, ".aikit-batch-stage-unknown")
	writeTestFile(t, filepath.Join(unknown, "keep"), "user", 0o644)
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("unknown batch path was not reported")
	}
	if _, err := os.Stat(filepath.Join(unknown, "keep")); err != nil {
		t.Fatalf("unknown batch path was deleted: %v", err)
	}
}

func TestRecoverCommittedBatchPreservesEveryBackupWhenAnyDestinationIsMissing(t *testing.T) {
	root := t.TempDir()
	var specs []CopySpec
	for _, name := range []string{"one", "two"} {
		source := filepath.Join(t.TempDir(), name)
		destination := filepath.Join(root, name)
		writeTestFile(t, filepath.Join(source, "SKILL.md"), "new-"+name, 0o644)
		writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old-"+name, 0o644)
		specs = append(specs, CopySpec{Source: source, Destination: destination})
	}
	batch, err := PrepareBatch(specs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.prepareMutationJournal(); err != nil {
		t.Fatal(err)
	}
	for index := range batch.Copies {
		copy := &batch.Copies[index]
		if err := moveNoReplace(copy.Destination, copy.Backup); err != nil {
			t.Fatal(err)
		}
		if err := moveNoReplace(copy.Staging, copy.Destination); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.persistJournal("committed"); err != nil {
		t.Fatal(err)
	}
	missing := batch.Copies[0].Destination
	missingOwner := batch.Copies[0].stageOwner
	if err := quarantineAndRemove(missing, missingOwner, moveNoReplace); err != nil {
		t.Fatal(err)
	}

	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("missing committed destination was not reported")
	}
	for _, copy := range batch.Copies {
		if _, err := os.Stat(filepath.Join(copy.Backup, "SKILL.md")); err != nil {
			t.Fatalf("backup %q was removed despite incomplete committed batch: %v", copy.Backup, err)
		}
	}
	if _, err := os.Stat(batch.Journal); err != nil {
		t.Fatalf("journal was removed despite incomplete committed batch: %v", err)
	}
}

func TestRecoverRejectsResignedJournalWhoseArtifactsAreNotBoundToID(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: filepath.Join(root, "skill")}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(root, ".aikit-batch-stage-not-this-batch")
	writeTestFile(t, filepath.Join(victim, "SKILL.md"), "keep", 0o644)
	victimOwner, err := inspectOwnedTree(victim)
	if err != nil {
		t.Fatal(err)
	}
	journal, _, err := readBatchJournal(root, batch.Journal)
	if err != nil {
		t.Fatal(err)
	}
	journal.Copies[0].Staging = victim
	journal.Copies[0].New = ownerForJournal(victimOwner)
	if err := writeBatchJournal(batch.Journal, journal); err != nil {
		t.Fatal(err)
	}

	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("resigned journal with unbound artifact was accepted")
	}
	if content, err := os.ReadFile(filepath.Join(victim, "SKILL.md")); err != nil || string(content) != "keep" {
		t.Fatalf("forged journal deleted victim: %q, %v", content, err)
	}
}

func TestRecoverDoesNotReadJournalSymlink(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(external, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".aikit-batch-0123456789abcdef0123456789abcdef.journal")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Detail, "unknown batch path") {
		t.Fatalf("journal symlink was read instead of treated as unknown: %#v", issues)
	}
}

func TestCommitDoesNotRollbackAfterCommittedMarkerBecameVisible(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	batch.SyncJournalDirectory = func(string) error {
		calls++
		if calls == 2 {
			return errors.New("injected committed journal directory sync failure")
		}
		return nil
	}
	err = batch.Commit()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "recover") {
		t.Fatalf("Commit error = %v, want recovery-required error", err)
	}
	content, readErr := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if readErr != nil || string(content) != "new" {
		t.Fatalf("visible committed marker caused rollback: %q, %v", content, readErr)
	}
	journal, _, readErr := readBatchJournal(root, batch.Journal)
	if readErr != nil || journal.Phase != "committed" {
		t.Fatalf("visible committed journal = %#v, %v", journal, readErr)
	}
}

func TestRecoverPreparedBatchRetriesNewOwnerQuarantineCleanup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: filepath.Join(root, "skill")}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	copy := batch.Copies[0]
	if err := moveNoReplace(copy.Staging, copy.Quarantine); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("prepared quarantine recovery = %#v, %v", issues, err)
	}
	if _, err := os.Lstat(copy.Quarantine); !os.IsNotExist(err) {
		t.Fatalf("prepared quarantine remains: %v", err)
	}
	if _, err := os.Lstat(batch.Journal); !os.IsNotExist(err) {
		t.Fatalf("prepared journal remains: %v", err)
	}
}

func TestRecoverMutatingBatchRetriesNewOwnerQuarantineCleanup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.prepareMutationJournal(); err != nil {
		t.Fatal(err)
	}
	copy := batch.Copies[0]
	if err := moveNoReplace(destination, copy.Backup); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(copy.Staging, copy.Quarantine); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("mutating quarantine recovery = %#v, %v", issues, err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(content) != "old" {
		t.Fatalf("old destination was not restored: %q, %v", content, err)
	}
	if _, err := os.Lstat(copy.Quarantine); !os.IsNotExist(err) {
		t.Fatalf("mutating quarantine remains: %v", err)
	}
}

func TestRecoverCommittedBatchRetriesOldOwnerQuarantineCleanup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(root, "skill")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.prepareMutationJournal(); err != nil {
		t.Fatal(err)
	}
	copy := batch.Copies[0]
	if err := moveNoReplace(destination, copy.Backup); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(copy.Staging, destination); err != nil {
		t.Fatal(err)
	}
	if err := batch.persistJournal("committed"); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(copy.Backup, copy.Quarantine); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("committed quarantine recovery = %#v, %v", issues, err)
	}
	if _, err := os.Lstat(copy.Quarantine); !os.IsNotExist(err) {
		t.Fatalf("committed quarantine remains: %v", err)
	}
	if _, err := os.Lstat(batch.Journal); !os.IsNotExist(err) {
		t.Fatalf("committed journal remains: %v", err)
	}
}

func TestRecoverPreservesLaterQuarantineReplacementAndJournal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
	batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: filepath.Join(root, "skill")}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	copy := batch.Copies[0]
	writeTestFile(t, filepath.Join(copy.Quarantine, "SKILL.md"), "later", 0o644)
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("later quarantine replacement was not reported")
	}
	content, err := os.ReadFile(filepath.Join(copy.Quarantine, "SKILL.md"))
	if err != nil || string(content) != "later" {
		t.Fatalf("later quarantine replacement was deleted: %q, %v", content, err)
	}
	if _, err := os.Stat(batch.Journal); err != nil {
		t.Fatalf("journal was cleared despite quarantine issue: %v", err)
	}
}
