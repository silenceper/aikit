package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/library"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

const inventoryWorkerLimit = 4

type inventoryRootJob struct {
	index int
	root  scanRoot
}

type inventoryRootResult struct {
	index    int
	root     scanRoot
	found    []discovered
	warnings []string
	issues   []app.ScanIssue
}

// Inventory streams one read-only event for every authorized root. Roots are
// enumerated from a single config snapshot, and events retain that stable
// ordering even though discovery runs concurrently.
func (s *Service) Inventory(ctx context.Context, request app.InventoryRequest) <-chan app.InventoryEvent {
	events := make(chan app.InventoryEvent)
	go s.inventory(ctx, request, events)
	return events
}

func (s *Service) inventory(ctx context.Context, request app.InventoryRequest, events chan<- app.InventoryEvent) {
	defer close(events)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return
	}

	cfg, err := s.deps.Store.Load(ctx)
	if err != nil {
		sendInventoryEvent(ctx, events, app.InventoryEvent{
			Generation: request.Generation,
			Issues: []app.ScanIssue{{
				State: app.ScanStateError, Message: fmt.Sprintf("load inventory config: %v", err),
			}},
			Done: true,
		})
		return
	}
	roots, err := s.scanRoots(cfg, app.ScanRequest{AllProjects: request.AllProjects, DryRun: true})
	if err != nil {
		sendInventoryEvent(ctx, events, app.InventoryEvent{
			Generation: request.Generation,
			Issues: []app.ScanIssue{{
				State: app.ScanStateError, Message: fmt.Sprintf("enumerate inventory roots: %v", err),
			}},
			Done: true,
		})
		return
	}
	if len(roots) == 0 {
		sendInventoryEvent(ctx, events, app.InventoryEvent{Generation: request.Generation, Done: true})
		return
	}

	jobs := make(chan inventoryRootJob, len(roots))
	results := make(chan inventoryRootResult, len(roots))
	for index, root := range roots {
		jobs <- inventoryRootJob{index: index, root: root}
	}
	close(jobs)

	workerCount := min(inventoryWorkerLimit, len(roots))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					found, warnings, issues, err := s.discoverInventoryRoot(ctx, job.root)
					if err != nil {
						return
					}
					result := inventoryRootResult{
						index: job.index, root: job.root, found: found, warnings: warnings, issues: issues,
					}
					select {
					case <-ctx.Done():
						return
					case results <- result:
					}
				}
			}
		}()
	}
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(results)
		close(workersDone)
	}()
	// Closing the public stream also means every worker and the results waiter
	// have exited. InventoryInspect must cooperate with ctx; an arbitrary OS
	// call that ignores cancellation cannot be forcibly killed by Go.
	defer func() { <-workersDone }()

	pending := make(map[int]inventoryRootResult, len(roots))
	next := 0
	allocationLedger := append([]config.Skill(nil), cfg.Library.Skills...)
	allocatedIDs := make(map[string]struct{}, len(allocationLedger))
	for _, skill := range allocationLedger {
		allocatedIDs[skill.ID] = struct{}{}
	}
	for next < len(roots) {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-results:
			if !ok {
				return
			}
			pending[result.index] = result
		}
		for {
			result, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			if ctx.Err() != nil {
				return
			}
			found := s.appendPendingDiscovered(cfg, []scanRoot{result.root}, result.found)
			if ctx.Err() != nil {
				return
			}
			found, allocationErr := allocateDiscovered(found, allocationLedger)
			if allocationErr != nil {
				appendScanIssue(&result.warnings, &result.issues, result.root, result.root.path, "allocate", allocationErr)
				found = nil
			}
			items := make([]app.ScanItem, 0, len(found))
			for _, item := range found {
				if ctx.Err() != nil {
					return
				}
				inventoryItem := s.inventoryItem(cfg, item, false)
				if ctx.Err() != nil {
					return
				}
				items = append(items, inventoryItem)
				if item.allocated.ID != "" {
					if _, exists := allocatedIDs[item.allocated.ID]; !exists {
						allocationLedger = append(allocationLedger, item.allocated)
						allocatedIDs[item.allocated.ID] = struct{}{}
					}
				}
			}
			sort.SliceStable(items, func(i, j int) bool {
				if items[i].Target != items[j].Target {
					return items[i].Target < items[j].Target
				}
				return items[i].Key < items[j].Key
			})
			completed := next + 1
			event := app.InventoryEvent{
				Generation: request.Generation,
				Root:       result.root.origin,
				Items:      items,
				Issues:     result.issues,
				Completed:  completed,
				Total:      len(roots),
				Done:       completed == len(roots),
			}
			if !sendInventoryEvent(ctx, events, event) {
				return
			}
			next++
		}
	}
}

// discoverInventoryRoot mirrors the strict no-follow inventory discovery path
// while threading cancellation through injected inspection. Filesystem calls
// and hashing are synchronous, so cancellation is checked before and after
// each such operation rather than wrapping them in unbounded goroutines.
func (s *Service) discoverInventoryRoot(ctx context.Context, root scanRoot) ([]discovered, []string, []app.ScanIssue, error) {
	var found []discovered
	var warnings []string
	var issues []app.ScanIssue
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	rootInfo, err := s.validateScanRoot(root)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, nil, ctxErr
	}
	if os.IsNotExist(err) {
		return nil, nil, nil, nil
	}
	if err != nil {
		appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
		return nil, warnings, issues, nil
	}
	entries, err := os.ReadDir(root.path)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, nil, ctxErr
	}
	if os.IsNotExist(err) {
		return nil, nil, nil, nil
	}
	if err != nil {
		appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
		return nil, warnings, issues, nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		target := filepath.Join(root.path, entry.Name())
		if err := revalidatePathIdentity(root.path, rootInfo, "inventory root"); err != nil {
			appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		targetInfo, statErr := os.Lstat(target)
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		if statErr != nil {
			appendScanIssue(&warnings, &issues, root, target, "inspect", statErr)
			continue
		}
		state, inspectErr := s.deps.InventoryInspect(ctx, target, s.deps.Paths.LibrarySkills)
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		if inspectErr != nil {
			appendScanIssue(&warnings, &issues, root, target, "inspect", inspectErr)
			continue
		}
		if err := revalidatePathIdentity(root.path, rootInfo, "inventory root"); err != nil {
			appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
			break
		}
		if err := revalidatePathIdentity(target, targetInfo, "inventory target"); err != nil {
			appendScanIssue(&warnings, &issues, root, target, "inspect", err)
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		if state.Kind == link.StateManagedLink {
			found = append(found, discovered{root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo, linkState: state, managedID: state.SkillID})
			continue
		}
		if state.Kind == link.StateExternalLink && state.Broken {
			found = append(found, discovered{root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo, linkState: state})
			continue
		}
		if state.Kind != link.StateDirectory && state.Kind != link.StateExternalLink {
			continue
		}
		candidates, discoverErr := library.Discover(target)
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		if discoverErr != nil || len(candidates) != 1 {
			if discoverErr == nil {
				discoverErr = fmt.Errorf("expected one skill, found %d", len(candidates))
			}
			appendScanIssue(&warnings, &issues, root, target, "discover", discoverErr)
			continue
		}
		if err := revalidatePathIdentity(root.path, rootInfo, "inventory root"); err != nil {
			appendScanIssue(&warnings, &issues, root, root.path, "scan", err)
			break
		}
		if err := revalidatePathIdentity(target, targetInfo, "inventory target"); err != nil {
			appendScanIssue(&warnings, &issues, root, target, "discover", err)
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		found = append(found, discovered{root: root, rootInfo: rootInfo, target: target, targetInfo: targetInfo, linkState: state, candidate: candidates[0]})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].target < found[j].target })
	return found, warnings, issues, nil
}

func sendInventoryEvent(ctx context.Context, events chan<- app.InventoryEvent, event app.InventoryEvent) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
