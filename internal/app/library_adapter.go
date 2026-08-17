package app

import (
	"context"

	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/pkg/config"
)

type libraryAdapter struct{ service library.Service }

func NewLibraryService(service library.Service) LibraryService {
	return libraryAdapter{service: service}
}

func (adapter libraryAdapter) Preview(ctx context.Context, request AddPreviewRequest) (AddPreview, error) {
	if err := ctx.Err(); err != nil {
		return AddPreview{}, err
	}
	resolved, err := library.ResolveAddSource(request.Source)
	if err != nil {
		return AddPreview{}, err
	}
	preview := AddPreview{
		ResolvedSource:      resolved.Source,
		SuggestedSelections: append([]string(nil), resolved.SuggestedSelections...),
	}
	if resolved.Kind == library.AddSourceLocal {
		candidates, err := library.Discover(resolved.Source)
		if err != nil {
			return AddPreview{}, err
		}
		preview.Candidates = make([]Candidate, len(candidates))
		for i, candidate := range candidates {
			preview.Candidates[i] = Candidate{
				Name: candidate.Name, Description: candidate.Description,
				RelativePath: candidate.RelativePath, Hash: candidate.Hash,
			}
		}
		return preview, nil
	}
	if !request.AllowNetwork {
		preview.NetworkRequired = true
		preview.Warnings = []string{"remote source discovery requires explicit network access"}
		return preview, nil
	}
	gitPreview, err := adapter.service.PreviewGit(ctx, resolved.Source, request.SourcePath, request.Ref)
	if err != nil {
		return AddPreview{}, err
	}
	preview.Candidates = make([]Candidate, len(gitPreview.Candidates))
	for i, candidate := range gitPreview.Candidates {
		preview.Candidates[i] = Candidate{
			Name: candidate.Name, Description: candidate.Description,
			RelativePath: candidate.RelativePath, Hash: candidate.Hash,
		}
	}
	for _, suggestion := range preview.SuggestedSelections {
		matched := false
		for _, candidate := range preview.Candidates {
			if candidate.Name == suggestion || candidate.RelativePath == suggestion {
				matched = true
				break
			}
		}
		if !matched {
			preview.Warnings = append(preview.Warnings, "skills.sh suggested skill "+suggestion+" is no longer present in the repository; choose from the current candidates")
			preview.SuggestedSelections = nil
			break
		}
	}
	preview.Ref = gitPreview.Ref
	preview.Resolved = gitPreview.Resolved
	return preview, nil
}

func (adapter libraryAdapter) PrepareAdd(ctx context.Context, request AddPrepareRequest, existing []config.Skill) (LibraryMutation, error) {
	resolved, err := library.ResolveAddSource(request.Source)
	if err != nil {
		return nil, err
	}
	if resolved.Kind == library.AddSourceLocal {
		mutation, err := adapter.service.PrepareLocal(ctx, resolved.Source, request.Selections, existing)
		if err != nil {
			return nil, err
		}
		return libraryMutation{mutation}, nil
	}
	selections := append([]string(nil), request.Selections...)
	if len(selections) == 0 {
		selections = append(selections, resolved.SuggestedSelections...)
	}
	mutation, err := adapter.service.PrepareGit(ctx, resolved.Source, library.GitAddOptions{
		SourcePath: request.SourcePath, Ref: request.Ref, Skills: selections,
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

func (adapter libraryAdapter) PrepareRemoveBatch(ctx context.Context, skills []config.Skill) (LibraryMutation, error) {
	mutation, err := adapter.service.PrepareRemoveBatch(ctx, skills)
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
