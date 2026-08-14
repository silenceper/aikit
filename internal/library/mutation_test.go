package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/pkg/config"
)

func TestPrepareRemoveBatchSecondMoveFailureRecoversEveryDestination(t *testing.T) {
	root := t.TempDir()
	skills := []config.Skill{{ID: "local/one", Name: "one", Hash: "one"}, {ID: "local/two", Name: "two", Hash: "two"}}
	for _, skill := range skills {
		destination, _ := SafeLibraryPath(root, skill.ID)
		writeTestFile(t, filepath.Join(destination, "SKILL.md"), skill.Name, 0o644)
	}
	moves := 0
	service := Service{LibraryRoot: root, Rename: func(old, new string) error {
		if strings.Contains(filepath.Base(new), ".aikit-batch-quarantine-") {
			moves++
			if moves == 2 {
				return errors.New("injected second move failure")
			}
		}
		return moveNoReplace(old, new)
	}}
	mutation, err := service.PrepareRemoveBatch(context.Background(), skills)
	if err != nil {
		t.Fatal(err)
	}
	if err := mutation.Commit(context.Background()); err == nil {
		t.Fatal("second move failure was lost")
	}
	issues, err := service.RecoverBatches(context.Background(), skills)
	if err != nil || len(issues) != 0 {
		t.Fatalf("recover remove batch = %+v, %v", issues, err)
	}
	for _, skill := range skills {
		destination, _ := SafeLibraryPath(root, skill.ID)
		if got, err := os.ReadFile(filepath.Join(destination, "SKILL.md")); err != nil || string(got) != skill.Name {
			t.Fatalf("skill %s not restored: %q %v", skill.ID, got, err)
		}
	}
}

func TestPrepareLocalStagesAllSelectionsWithoutChangingDestinations(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "one", "SKILL.md"), "---\nname: one\n---\n", 0o644)
	writeTestFile(t, filepath.Join(source, "two", "SKILL.md"), "---\nname: two\n---\n", 0o644)
	mutation, err := (Service{LibraryRoot: root}).PrepareLocal(context.Background(), source, []string{"two", "one"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mutation.Skills) != 2 {
		t.Fatalf("prepared skills = %#v", mutation.Skills)
	}
	for _, skill := range mutation.Skills {
		destination, err := SafeLibraryPath(root, skill.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(destination); !os.IsNotExist(err) {
			t.Fatalf("prepare changed destination %q: %v", destination, err)
		}
	}
	if err := mutation.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, skill := range mutation.Skills {
		destination, _ := SafeLibraryPath(root, skill.ID)
		if _, err := os.Stat(filepath.Join(destination, "SKILL.md")); err != nil {
			t.Fatalf("commit did not install %q: %v", skill.ID, err)
		}
	}
}

func TestPrepareLocalAbortLeavesDestinationsAbsent(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "SKILL.md"), "skill", 0o644)
	mutation, err := (Service{LibraryRoot: root}).PrepareLocal(context.Background(), source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mutation.Abort(); err != nil {
		t.Fatal(err)
	}
	destination, _ := SafeLibraryPath(root, mutation.Skills[0].ID)
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("abort changed destination: %v", err)
	}
}

func TestPrepareRemoveDoesNotChangeDestinationAndCommitRemovesOwnedTree(t *testing.T) {
	root := t.TempDir()
	skill := config.Skill{ID: "local/demo", Name: "demo"}
	destination, _ := SafeLibraryPath(root, skill.ID)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	mutation, err := (Service{LibraryRoot: root}).PrepareRemove(context.Background(), skill)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "SKILL.md")); err != nil {
		t.Fatalf("prepare remove changed destination: %v", err)
	}
	if mutation.Removed == nil || mutation.Removed.ID != skill.ID {
		t.Fatalf("removed preview = %#v", mutation.Removed)
	}
	if err := mutation.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("remove commit retained destination: %v", err)
	}
}

func TestRecoverPreparedRemoveKeepsDestination(t *testing.T) {
	root := t.TempDir()
	skill := config.Skill{ID: "local/demo", Name: "demo"}
	destination, _ := SafeLibraryPath(root, skill.ID)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	if _, err := (Service{LibraryRoot: root}).PrepareRemove(context.Background(), skill); err != nil {
		t.Fatal(err)
	}
	issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), []config.Skill{skill})
	if err != nil || len(issues) != 0 {
		t.Fatalf("recover prepared remove = %#v, %v", issues, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "SKILL.md")); err != nil {
		t.Fatalf("prepared remove recovery removed destination: %v", err)
	}
}

func TestRecoverMutatingRemoveFinishesAuthenticatedDeletion(t *testing.T) {
	root := t.TempDir()
	skill := config.Skill{ID: "local/demo", Name: "demo"}
	destination, _ := SafeLibraryPath(root, skill.ID)
	writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
	service := Service{LibraryRoot: root}
	mutation, err := service.PrepareRemove(context.Background(), skill)
	if err != nil {
		t.Fatal(err)
	}
	plan := mutation.commit
	_ = plan
	// Reopen the journal as a crash would, change only the durable phase, then
	// let recovery complete the still-present authenticated destination.
	entries, _ := filepath.Glob(filepath.Join(root, ".aikit-batch-*.journal"))
	journal, _, err := readBatchJournal(root, entries[0])
	if err != nil {
		t.Fatal(err)
	}
	journal.Phase = "mutating"
	if err := writeBatchJournal(entries[0], journal); err != nil {
		t.Fatal(err)
	}
	issues, err := service.RecoverBatches(context.Background(), nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("recover mutating remove = %#v, %v", issues, err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("mutating remove recovery retained destination: %v", err)
	}
}

func TestLedgerDrivenPreparedAddRollsForwardOrBack(t *testing.T) {
	for _, test := range []struct {
		name   string
		ledger bool
		want   bool
	}{{"checkpoint-before-recovery", true, true}, {"no-checkpoint", false, false}} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := t.TempDir()
			writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
			service := Service{LibraryRoot: root}
			mutation, err := service.PrepareLocal(context.Background(), source, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var ledger []config.Skill
			if test.ledger {
				ledger = mutation.Skills
			}
			issues, err := service.RecoverBatches(context.Background(), ledger)
			if err != nil || len(issues) != 0 {
				t.Fatalf("recovery = %#v, %v", issues, err)
			}
			destination, _ := SafeLibraryPath(root, mutation.Skills[0].ID)
			_, statErr := os.Stat(filepath.Join(destination, "SKILL.md"))
			if (statErr == nil) != test.want {
				t.Fatalf("destination exists = %v, want %v (%v)", statErr == nil, test.want, statErr)
			}
		})
	}
}

func TestLedgerDrivenUpdateRecoversMidCommitInLedgerDirection(t *testing.T) {
	for _, useNew := range []bool{false, true} {
		t.Run(map[bool]string{false: "old", true: "new"}[useNew], func(t *testing.T) {
			root := t.TempDir()
			destination, err := SafeLibraryPath(root, "owner/repo/demo")
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
			source := t.TempDir()
			writeTestFile(t, filepath.Join(source, "SKILL.md"), "new", 0o644)
			batch, err := PrepareBatch([]CopySpec{{Source: source, Destination: destination}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			oldSkill := config.Skill{ID: "owner/repo/demo", Name: "demo", Hash: "old-hash", Resolved: "old-object"}
			newSkill := config.Skill{ID: oldSkill.ID, Name: "demo", Hash: "new-hash", Resolved: "new-object"}
			if err := (Service{LibraryRoot: root}).configureBatchSkills(batch, []config.Skill{newSkill}, []config.Skill{oldSkill}); err != nil {
				t.Fatal(err)
			}
			if err := batch.prepareMutationJournal(); err != nil {
				t.Fatal(err)
			}
			copy := batch.Copies[0]
			if err := moveNoReplace(copy.Destination, copy.Backup); err != nil {
				t.Fatal(err)
			}
			ledger := []config.Skill{oldSkill}
			want := "old"
			if useNew {
				ledger, want = []config.Skill{newSkill}, "new"
			}
			issues, err := (Service{LibraryRoot: root}).RecoverBatches(context.Background(), ledger)
			if err != nil || len(issues) != 0 {
				t.Fatalf("recovery = %#v, %v", issues, err)
			}
			content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
			if err != nil || string(content) != want {
				t.Fatalf("destination = %q, %v; want %q", content, err, want)
			}
		})
	}
}

func TestLedgerDrivenRemoveCommittedJournalRollsForwardOrBack(t *testing.T) {
	for _, keepLedger := range []bool{false, true} {
		t.Run(map[bool]string{false: "removed-ledger", true: "old-ledger"}[keepLedger], func(t *testing.T) {
			root := t.TempDir()
			skill := config.Skill{ID: "local/demo", Name: "demo", Hash: "old-hash"}
			destination, _ := SafeLibraryPath(root, skill.ID)
			writeTestFile(t, filepath.Join(destination, "SKILL.md"), "old", 0o644)
			service := Service{LibraryRoot: root}
			mutation, err := service.PrepareRemove(context.Background(), skill)
			if err != nil {
				t.Fatal(err)
			}
			if err := mutation.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			var ledger []config.Skill
			if keepLedger {
				ledger = []config.Skill{skill}
			}
			issues, err := service.RecoverBatches(context.Background(), ledger)
			if err != nil || len(issues) != 0 {
				t.Fatalf("recovery = %#v, %v", issues, err)
			}
			_, statErr := os.Stat(filepath.Join(destination, "SKILL.md"))
			if (statErr == nil) != keepLedger {
				t.Fatalf("destination exists = %v, want %v", statErr == nil, keepLedger)
			}
		})
	}
}
