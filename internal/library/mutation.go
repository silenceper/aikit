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

type removeBatchMutation struct {
	service Service
	journal batchJournal
	path    string
	owners  []ownedTree
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
	return s.PrepareRemoveBatch(ctx, []config.Skill{skill})
}

func (s Service) PrepareRemoveBatch(ctx context.Context, skills []config.Skill) (*Mutation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("remove batch is empty")
	}
	id, err := newBatchID()
	if err != nil {
		return nil, err
	}
	journal := batchJournal{Version: batchJournalVersion, ID: id, Operation: "remove", Phase: "prepared"}
	owners := make([]ownedTree, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for index, skill := range skills {
		destination, err := SafeLibraryPath(s.LibraryRoot, skill.ID)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[destination]; duplicate {
			return nil, fmt.Errorf("duplicate remove destination %q", destination)
		}
		seen[destination] = struct{}{}
		owner, err := inspectOwnedTree(destination)
		if err != nil {
			if os.IsNotExist(err) {
				owner = ownedTree{}
			} else {
				return nil, err
			}
		}
		owners = append(owners, owner)
		journal.Copies = append(journal.Copies, journalCopy{
			Operation: "remove", Destination: destination,
			Quarantine: batchArtifactPath(filepath.Dir(destination), "quarantine", id, index),
			Existed:    owner.identity != "", Old: ownerForJournal(owner), OldSkill: &skills[index],
		})
	}
	journalPath := filepath.Join(s.LibraryRoot, ".aikit-batch-"+id+".journal")
	if err := os.MkdirAll(s.LibraryRoot, 0o755); err != nil {
		return nil, err
	}
	if _, err := writeBatchJournalWithSync(journalPath, journal, s.syncDirectory); err != nil {
		return nil, err
	}
	plan := &removeBatchMutation{service: s, journal: journal, path: journalPath, owners: owners}
	entries := append([]config.Skill(nil), skills...)
	mutation := &Mutation{
		Skills: entries,
		commit: plan.commit,
		abort:  plan.abort,
	}
	if len(entries) == 1 {
		mutation.Removed = &mutation.Skills[0]
	}
	return mutation, nil
}

func (p *removeBatchMutation) commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for index, copy := range p.journal.Copies {
		if !copy.Existed {
			continue
		}
		if err := verifyOwnedTree(copy.Destination, p.owners[index]); err != nil {
			return fmt.Errorf("remove library batch preflight: %w", err)
		}
	}
	p.journal.Phase = "mutating"
	if _, err := writeBatchJournalWithSync(p.path, p.journal, p.service.syncDirectory); err != nil {
		return fmt.Errorf("remove batch journal became visible; run RecoverBatches: %w", err)
	}
	for index, copy := range p.journal.Copies {
		if !copy.Existed {
			continue
		}
		if err := p.service.rename(copy.Destination, copy.Quarantine); err != nil {
			return fmt.Errorf("remove library batch; run RecoverBatches: %w", err)
		}
		if err := verifyOwnedTree(copy.Quarantine, p.owners[index]); err != nil {
			return fmt.Errorf("verify removed library batch; run RecoverBatches: %w", err)
		}
	}
	p.journal.Phase = "committed"
	if _, err := writeBatchJournalWithSync(p.path, p.journal, p.service.syncDirectory); err != nil {
		return fmt.Errorf("commit remove batch journal; run RecoverBatches: %w", err)
	}
	return nil
}

func (p *removeBatchMutation) abort() error {
	if p.journal.Phase != "prepared" {
		return fmt.Errorf("remove batch phase %q requires RecoverBatches", p.journal.Phase)
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
