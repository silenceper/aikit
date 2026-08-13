package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/pkg/config"
)

// Mutation is a durable, staged library change. Callers may inspect its
// result fields and validate/checkpoint the corresponding ledger before
// Commit changes a library destination.
type Mutation struct {
	Skills  []config.Skill
	Updated *config.Skill
	Removed *config.Skill

	batch     *Batch
	commit    func(context.Context) error
	abort     func() error
	committed bool
}

type removeMutation struct {
	service Service
	journal batchJournal
	path    string
	owner   ownedTree
}

func (m *Mutation) Commit(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("nil library mutation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.committed {
		return fmt.Errorf("library mutation already committed")
	}
	var err error
	if m.commit != nil {
		err = m.commit(ctx)
	} else if m.batch != nil {
		err = m.batch.Commit()
	}
	if err == nil {
		m.committed = true
	}
	return err
}

func (m *Mutation) Abort() error {
	if m == nil || m.committed {
		return nil
	}
	if m.abort != nil {
		return m.abort()
	}
	if m.batch != nil {
		return m.batch.Abort()
	}
	return nil
}

func (s Service) PrepareRemove(ctx context.Context, skill config.Skill) (*Mutation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	destination, err := SafeLibraryPath(s.LibraryRoot, skill.ID)
	if err != nil {
		return nil, err
	}
	owner, err := inspectOwnedTree(destination)
	if err != nil {
		if os.IsNotExist(err) {
			removed := skill
			return &Mutation{Removed: &removed}, nil
		}
		return nil, err
	}
	id, err := newBatchID()
	if err != nil {
		return nil, err
	}
	journalPath := filepath.Join(filepath.Dir(destination), ".aikit-batch-"+id+".journal")
	journal := batchJournal{
		Version: batchJournalVersion, ID: id, Operation: "remove", Phase: "prepared",
		Copies: []journalCopy{{Operation: "remove", Destination: destination, Quarantine: batchArtifactPath(filepath.Dir(destination), "quarantine", id, 0), Existed: true, Old: ownerForJournal(owner), OldSkill: &skill}},
	}
	if _, err := writeBatchJournalWithSync(journalPath, journal, s.syncDirectory); err != nil {
		return nil, err
	}
	plan := &removeMutation{service: s, journal: journal, path: journalPath, owner: owner}
	removed := skill
	return &Mutation{
		Removed: &removed,
		commit:  plan.commit,
		abort:   plan.abort,
	}, nil
}

func (p *removeMutation) commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.journal.Phase = "mutating"
	visible, err := writeBatchJournalWithSync(p.path, p.journal, p.service.syncDirectory)
	if err != nil {
		if visible {
			return fmt.Errorf("remove journal became visible; run RecoverBatches: %w", err)
		}
		return err
	}
	copy := p.journal.Copies[0]
	if err := verifyOwnedTree(copy.Destination, p.owner); err != nil {
		return fmt.Errorf("remove library skill; run RecoverBatches: %w", err)
	}
	if err := p.service.rename(copy.Destination, copy.Quarantine); err != nil {
		return fmt.Errorf("remove library skill; run RecoverBatches: %w", err)
	}
	if err := verifyOwnedTree(copy.Quarantine, p.owner); err != nil {
		return fmt.Errorf("verify removed library skill; run RecoverBatches: %w", err)
	}
	p.journal.Phase = "committed"
	if _, err := writeBatchJournalWithSync(p.path, p.journal, p.service.syncDirectory); err != nil {
		return fmt.Errorf("commit remove journal; run RecoverBatches: %w", err)
	}
	return nil
}

func (p *removeMutation) abort() error {
	if p.journal.Phase != "prepared" {
		return fmt.Errorf("remove mutation phase %q requires RecoverBatches", p.journal.Phase)
	}
	journalOwner, err := journalOwnerAt(filepath.Dir(p.path), p.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return removeOwnedJournal(filepath.Dir(p.path), p.path, p.journal.ID, journalOwner)
}
