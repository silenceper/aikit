package app

import (
	"context"
	"fmt"
	"os"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/pkg/config"
)

type libraryAdapter struct{ service library.Service }

func NewLibraryService(service library.Service) LibraryService {
	return libraryAdapter{service: service}
}

func (adapter libraryAdapter) PrepareAdd(ctx context.Context, request AddPrepareRequest, existing []config.Skill) (LibraryMutation, error) {
	if info, err := os.Stat(request.Source); err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("local source %q is not a directory", request.Source)
		}
		mutation, err := adapter.service.PrepareLocal(ctx, request.Source, request.Selections, existing)
		if err != nil {
			return nil, err
		}
		return libraryMutation{mutation}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	mutation, err := adapter.service.PrepareGit(ctx, request.Source, library.GitAddOptions{
		SourcePath: request.SourcePath, Ref: request.Ref, Skills: request.Selections,
		Existing: existing, Force: request.Force,
	})
	if err != nil {
		return nil, err
	}
	return libraryMutation{mutation}, nil
}

func (adapter libraryAdapter) PrepareUpdate(ctx context.Context, items []UpdatePrepareItem) (LibraryMutation, error) {
	specs := make([]library.UpdateSpec, len(items))
	for i, item := range items {
		specs[i] = library.UpdateSpec{Old: item.Skill, Ref: item.Ref}
	}
	mutation, err := adapter.service.PrepareUpdates(ctx, specs)
	if err != nil {
		return nil, err
	}
	return libraryMutation{mutation}, nil
}

func (adapter libraryAdapter) PrepareRemove(ctx context.Context, skill config.Skill) (LibraryMutation, error) {
	mutation, err := adapter.service.PrepareRemove(ctx, skill)
	if err != nil {
		return nil, err
	}
	return libraryMutation{mutation}, nil
}

type libraryMutation struct{ mutation *library.Mutation }

func (mutation libraryMutation) Entries() []config.Skill {
	if mutation.mutation == nil {
		return nil
	}
	if mutation.mutation.Updated != nil {
		return []config.Skill{*mutation.mutation.Updated}
	}
	if mutation.mutation.Removed != nil {
		return []config.Skill{*mutation.mutation.Removed}
	}
	return append([]config.Skill(nil), mutation.mutation.Skills...)
}

func (mutation libraryMutation) Commit(ctx context.Context) error {
	return mutation.mutation.Commit(ctx)
}
func (mutation libraryMutation) Abort() error { return mutation.mutation.Abort() }
