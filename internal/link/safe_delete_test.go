package link

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silenceper/aikit/pkg/config"
)

func TestDeleteJournalRejectsReplacedManifestThatAuthorizesNewContent(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "original"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, err := FingerprintPath(backup)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, ".aikit-adopt-delete-op")
	manifest := root + ".manifest"
	journalHash, err := DeleteJournalHash(backup, fp)
	if err != nil {
		t.Fatal(err)
	}
	if err = prepareDeleteJournal(backup, root, manifest, &fp, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(root, "new-user-file")
	if err = os.WriteFile(added, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	addedFP, err := FingerprintPath(added)
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadDeleteManifest(manifest, journalHash)
	if err != nil {
		t.Fatal(err)
	}
	m.Entries = append(m.Entries, deleteManifestEntry{Path: "new-user-file", Kind: "file", Hash: addedFP.Hash})
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifest, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = resumeDeleteJournal(root, manifest, journalHash, testDeleteOps(os.Remove)); err == nil {
		t.Fatal("replaced manifest was accepted")
	}
	if got, readErr := os.ReadFile(added); readErr != nil || string(got) != "keep" {
		t.Fatalf("new content was not retained: %q %v", got, readErr)
	}
}

func TestDeleteJournalValidatesRelativeSymlinkObjectAfterItsTargetWasDeleted(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup")
	if err := os.MkdirAll(filepath.Join(backup, "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(backup, "links"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "deeper", "target"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "deeper", "target"), filepath.Join(backup, "links", "x")); err != nil {
		t.Fatal(err)
	}
	fp, err := FingerprintPath(backup)
	if err != nil {
		t.Fatal(err)
	}
	journalHash, err := DeleteJournalHash(backup, fp)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, ".aikit-adopt-delete-op")
	manifest := root + ".manifest"
	if err = prepareDeleteJournal(backup, root, manifest, &fp, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatal(err)
	}
	if err = resumeDeleteJournal(root, manifest, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatalf("relative symlink object should remain verifiable after target deletion: %v", err)
	}
}

func TestDeleteJournalRejectsIntermediateDirectoryReplacedBySymlink(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup")
	if err := os.MkdirAll(filepath.Join(backup, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "nested", "same"), []byte("valuable"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, err := FingerprintPath(backup)
	if err != nil {
		t.Fatal(err)
	}
	journalHash := testDeleteJournalHash(t, backup, fp)
	deleteRoot := filepath.Join(dir, ".aikit-adopt-delete-op")
	manifest := deleteRoot + ".manifest"
	if err = prepareDeleteJournal(backup, deleteRoot, manifest, &fp, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside")
	if err = os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "same")
	if err = os.WriteFile(outsideFile, []byte("valuable"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(deleteRoot, "nested")); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(deleteRoot, "nested")); err != nil {
		t.Fatal(err)
	}
	if err = resumeDeleteJournal(deleteRoot, manifest, journalHash, testDeleteOps(os.Remove)); err == nil {
		t.Fatal("intermediate symlink replacement was accepted")
	}
	if got, readErr := os.ReadFile(outsideFile); readErr != nil || string(got) != "valuable" {
		t.Fatalf("outside file was deleted or changed: %q %v", got, readErr)
	}
}

func TestAdoptRecoveryResumesAfterRelativeSymlinkTargetWasDeleted(t *testing.T) {
	root := t.TempDir()
	libraryRoot := filepath.Join(root, "library", "skills")
	librarySkill := filepath.Join(libraryRoot, "local", "demo")
	for _, tree := range []string{filepath.Join(root, "original"), librarySkill} {
		if err := os.MkdirAll(filepath.Join(tree, "deeper"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(tree, "links"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tree, "deeper", "target"), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "deeper", "target"), filepath.Join(tree, "links", "x")); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(root, ".cursor", "skills", "demo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(target), ".aikit-adopt-backup-nonce")
	if err := os.Rename(filepath.Join(root, "original"), backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(librarySkill, target); err != nil {
		t.Fatal(err)
	}
	fp, err := FingerprintPath(backup)
	if err != nil {
		t.Fatal(err)
	}
	op := config.PendingOperation{ID: "op", Kind: config.OperationAdopt, Scope: config.Scope{Project: "p", ProjectPath: root, Agent: "cursor"}, Target: target, SkillID: "local/demo", Temp: filepath.Join(filepath.Dir(target), ".aikit-adopt-temp-nonce"), Backup: backup, Original: &fp}
	op.JournalHash = testDeleteJournalHash(t, backup, fp)
	deleteRoot, manifest := DeleteJournalPaths(op)
	if err = prepareDeleteJournal(backup, deleteRoot, manifest, &fp, op.JournalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatal(err)
	}
	partialOps := testDeleteOps(os.Remove)
	partialOps.MoveNoReplace = func(from, to string) error {
		if strings.HasSuffix(from, filepath.Join("links", "x")) {
			return errors.New("stop before quarantining relative symlink")
		}
		return moveNoReplace(from, to)
	}
	if err = resumeDeleteJournal(deleteRoot, manifest, op.JournalHash, partialOps); err == nil {
		t.Fatal("expected interruption")
	}
	complete, issue := recoverAdopt(libraryRoot, op, testDeleteOps(os.Remove))
	if issue != nil || !complete {
		t.Fatalf("journal did not resume past dangling relative symlink: complete=%v issue=%#v", complete, issue)
	}
}

func testDeleteOps(remove func(string) error) FileOps {
	return FileOps{MoveNoReplace: moveNoReplace, Symlink: os.Symlink, Remove: remove}
}

func testDeleteJournalHash(t *testing.T, root string, fp config.Fingerprint) string {
	t.Helper()
	hash, err := DeleteJournalHash(root, fp)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestDeleteJournalRootFileAndSymlinkRemoveFailureRetry(t *testing.T) {
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			backup := filepath.Join(dir, "backup")
			if kind == "file" {
				if err := os.WriteFile(backup, []byte("data"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				target := filepath.Join(dir, "target")
				if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, backup); err != nil {
					t.Fatal(err)
				}
			}
			fp, err := FingerprintPath(backup)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(dir, ".aikit-adopt-delete-op")
			manifest := root + ".manifest"
			journalHash := testDeleteJournalHash(t, backup, fp)
			if err = prepareDeleteJournal(backup, root, manifest, &fp, journalHash, testDeleteOps(os.Remove)); err != nil {
				t.Fatal(err)
			}
			failed := false
			ops := testDeleteOps(func(path string) error {
				if !failed {
					failed = true
					return errors.New("denied")
				}
				return os.Remove(path)
			})
			if err = resumeDeleteJournal(root, manifest, journalHash, ops); err == nil {
				t.Fatal("expected remove failure")
			}
			if err = resumeDeleteJournal(root, manifest, journalHash, testDeleteOps(os.Remove)); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if existsLstat(manifest) {
				t.Fatal("manifest remains")
			}
		})
	}
}

func TestDeleteJournalDirectoryRemoveFailureRetry(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup")
	if err := os.MkdirAll(filepath.Join(backup, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, _ := FingerprintPath(backup)
	root := filepath.Join(dir, ".aikit-adopt-delete-op")
	manifest := root + ".manifest"
	journalHash := testDeleteJournalHash(t, backup, fp)
	if err := prepareDeleteJournal(backup, root, manifest, &fp, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatal(err)
	}
	failed := false
	ops := testDeleteOps(func(path string) error {
		if info, err := os.Lstat(path); err == nil && info.IsDir() && !failed {
			failed = true
			return errors.New("dir denied")
		}
		return os.Remove(path)
	})
	if err := resumeDeleteJournal(root, manifest, journalHash, ops); err == nil {
		t.Fatal("expected dir failure")
	}
	if err := resumeDeleteJournal(root, manifest, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestDeleteJournalCorruptManifestFailsClosed(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "delete")
	if err := os.WriteFile(root, []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := root + ".manifest"
	if err := os.WriteFile(manifest, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resumeDeleteJournal(root, manifest, strings.Repeat("0", 64), testDeleteOps(os.Remove)); err == nil {
		t.Fatal("corrupt manifest accepted")
	}
	b, err := os.ReadFile(root)
	if err != nil || string(b) != "user" {
		t.Fatalf("user content changed: %q %v", b, err)
	}
}

func TestDeleteJournalRetriesCrashAfterManifestBeforeMove(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup")
	if err := os.WriteFile(backup, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, _ := FingerprintPath(backup)
	root := filepath.Join(dir, ".aikit-adopt-delete-op")
	manifest := root + ".manifest"
	m, err := buildDeleteManifest(backup, fp)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeManifestAtomic(manifest, m); err != nil {
		t.Fatal(err)
	}
	journalHash, err := deleteManifestHash(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = continueDeleteJournal(backup, root, manifest, &fp, journalHash, testDeleteOps(os.Remove)); err != nil {
		t.Fatalf("continue journal: %v", err)
	}
	if existsLstat(backup) || existsLstat(root) || existsLstat(manifest) {
		t.Fatal("journal did not complete")
	}
}
