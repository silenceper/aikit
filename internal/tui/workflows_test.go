package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/internal/status"
	"github.com/silenceper/aikit/pkg/config"
)

func TestLibraryDetailLoadsTypedContentAsynchronously(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), detail: app.SkillDetail{
		Skill:            config.Skill{ID: "acme/alpha", Name: "alpha", Source: "https://example.test/acme.git"},
		EnabledLocations: []config.Scope{{Agent: "codex"}},
		SkillMD:          "# Alpha\nLibrary detail body",
		Files:            []app.SkillFile{{Path: "SKILL.md", Kind: app.SkillFileRegular}, {Path: "scripts/run.sh", Kind: app.SkillFileRegular}},
	}}
	m := loadedModel(t, service, &fakeMigration{})

	next, cmd := apply(m, "enter")
	if cmd == nil || !next.Detail || service.detailCalls != 0 {
		t.Fatalf("detail must be deferred: cmd=%v detail=%v calls=%d", cmd != nil, next.Detail, service.detailCalls)
	}
	nextModel, _ := next.Update(cmd())
	m = nextModel.(Model)
	if service.detailCalls != 1 {
		t.Fatalf("SkillDetail calls=%d, want 1", service.detailCalls)
	}
	for _, wanted := range []string{"https://example.test/acme.git", "codex", "SKILL.md", "Library detail body", "+1 files"} {
		if !strings.Contains(m.ViewString(), wanted) {
			t.Fatalf("loaded detail missing %q:\n%s", wanted, m.ViewString())
		}
	}
}

func TestSkillContentOpensBoundedOverlayAndRestoresLibraryDetail(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 12
	m.Detail, m.Focus, m.SkillDetail = true, FocusActions, largeSkillDetail()
	m.Cursor, m.Scroll, m.DetailScroll = 0, 0, 2
	m.Status = "original library status"

	actions := m.primaryActions()
	if len(actions) == 0 || actions[0] != "View SKILL.md" {
		t.Fatalf("loaded detail actions=%v", actions)
	}
	next, cmd := m.performPrimaryAction(0)
	if cmd != nil {
		t.Fatal("viewing already-loaded content returned a command")
	}
	m = next.(Model)
	if m.Mode != ModeErrorDetail || !strings.Contains(stripANSI(m.ViewString()), "SKILL.md · alpha") {
		t.Fatalf("skill content overlay not opened: mode=%s\n%s", m.Mode, m.ViewString())
	}
	sawLastLine, sawTruncation := false, false
	for index := 0; index < 1000; index++ {
		view := stripANSI(m.ViewString())
		sawLastLine = sawLastLine || strings.Contains(view, "source-line-219")
		sawTruncation = sawTruncation || strings.Contains(view, "preview truncated at 64 KiB")
		m.moveOverlayScroll(1)
	}
	end := stripANSI(m.ViewString())
	if !sawLastLine || !sawTruncation {
		t.Fatalf("final loaded content/truncation marker unreachable at scroll=%d:\n%s", m.OverlayScroll, end)
	}
	next, cmd = m.performPrimaryAction(0)
	if cmd != nil {
		t.Fatal("closing content overlay returned a command")
	}
	m = next.(Model)
	if m.Mode != ModeTable || !m.Detail || m.Cursor != 0 || m.DetailScroll != 2 || m.Status != "original library status" {
		t.Fatalf("content close lost detail context: mode=%s detail=%v cursor=%d scroll=%d status=%q", m.Mode, m.Detail, m.Cursor, m.DetailScroll, m.Status)
	}
}

func TestSkillContentMouseUsesSameActionAsKeyboard(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 100, 20
	base.Detail, base.Focus, base.SkillDetail = true, FocusActions, largeSkillDetail()
	regions := base.hitRegions()
	if len(regions.Actions) == 0 {
		t.Fatal("loaded detail has no mouse action")
	}
	mouseNext, mouseCmd := base.Update(click(regions.Actions[0].X, regions.Actions[0].Y))
	keyNext, keyCmd := base.Update(actionKey(tea.KeyEnter))
	mouse, keyboard := mouseNext.(Model), keyNext.(Model)
	if mouseCmd != nil || keyCmd != nil || mouse.Mode != ModeErrorDetail || keyboard.Mode != ModeErrorDetail || mouse.FullError != keyboard.FullError {
		t.Fatalf("skill content parity mouse=%s keyboard=%s mouseCmd=%v keyCmd=%v", mouse.Mode, keyboard.Mode, mouseCmd != nil, keyCmd != nil)
	}
}

func TestSkillContentMouseScrollAndCloseRestoresExactState(t *testing.T) {
	base := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	base.Snapshot, base.Width, base.Height = testSnapshot(), 80, 12
	base.Detail, base.Focus, base.SkillDetail = true, FocusActions, largeSkillDetail()
	base.ActionIndex, base.Cursor, base.Scroll, base.DetailScroll = 0, 0, 0, 2
	base.Status = "ready"

	regions := base.hitRegions()
	opened, cmd := base.Update(click(regions.Actions[0].X, regions.Actions[0].Y))
	if cmd != nil {
		t.Fatal("mouse open returned a command")
	}
	m := opened.(Model)
	before := m.OverlayScroll
	scrolled, cmd := m.Update(tea.MouseMsg{X: m.Width / 2, Y: m.Height / 2, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if cmd != nil {
		t.Fatal("mouse wheel returned a command")
	}
	m = scrolled.(Model)
	if m.OverlayScroll <= before {
		t.Fatalf("mouse wheel did not scroll content: before=%d after=%d", before, m.OverlayScroll)
	}
	closeRegion := m.hitRegions().Actions[0]
	closed, cmd := m.Update(click(closeRegion.X, closeRegion.Y))
	if cmd != nil {
		t.Fatal("mouse close returned a command")
	}
	m = closed.(Model)
	if m.Mode != ModeTable || m.Focus != FocusActions || m.ActionIndex != 0 || m.DetailScroll != 2 || m.Status != "ready" {
		t.Fatalf("mouse close did not restore state: mode=%s focus=%s action=%d detailScroll=%d status=%q", m.Mode, m.Focus, m.ActionIndex, m.DetailScroll, m.Status)
	}
}

func TestLibraryRemovePreviewsAndRequiresSeparateForceConfirmation(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), removePreview: app.MutationPreview{
		Title: "Remove skill", Summary: "Remove acme/alpha", References: []string{"agent:codex"},
		RequiresConfirmation: true, RequiresForce: true,
		Plan: link.Plan{Actions: []link.Action{{Path: "/work/.codex/skills/alpha"}}},
	}}
	m := loadedModel(t, service, &fakeMigration{})

	m, preview := apply(m, "d")
	if preview == nil || service.previewRemoveCalls != 0 || service.removeCalls != 0 {
		t.Fatalf("remove preview not deferred: preview=%v previewCalls=%d removeCalls=%d", preview != nil, service.previewRemoveCalls, service.removeCalls)
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if service.previewRemoveCalls != 1 || m.Mode != ModeConfirm || !strings.Contains(m.ViewString(), "agent:codex") {
		t.Fatalf("remove preview not presented: mode=%s calls=%d\n%s", m.Mode, service.previewRemoveCalls, m.ViewString())
	}
	m, first := apply(m, "enter")
	if first != nil || service.removeCalls != 0 || !strings.Contains(m.ViewString(), "Force") {
		t.Fatalf("first confirm must be force gate: cmd=%v calls=%d\n%s", first != nil, service.removeCalls, m.ViewString())
	}
	m, remove := apply(m, "enter")
	if remove == nil || service.removeCalls != 0 {
		t.Fatalf("second confirm must defer removal: cmd=%v calls=%d", remove != nil, service.removeCalls)
	}
	_ = remove()
	if service.removeCalls != 1 || !service.lastRemove.Force {
		t.Fatalf("forced remove request=%+v calls=%d", service.lastRemove, service.removeCalls)
	}
}

func TestWorkspaceTogglePreviewsBeforeMutation(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), bindingPreview: app.MutationPreview{
		Title: "Disable skill", Summary: "Disable acme/alpha in codex", RequiresConfirmation: true,
	}}
	m := loadedModel(t, service, &fakeMigration{})
	m.switchView(ViewWorkspaces)
	m.Scope = Scope{Agent: "codex", Level: "agent-skills"}

	m, preview := apply(m, "space")
	if preview == nil || service.previewBindingCalls != 0 || service.bindingCalls != 0 {
		t.Fatalf("binding preview not deferred: cmd=%v previewCalls=%d mutationCalls=%d", preview != nil, service.previewBindingCalls, service.bindingCalls)
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || service.lastBindingPreview.Binding.SkillID != "acme/alpha" || service.lastBindingPreview.Enable {
		t.Fatalf("binding preview=%+v mode=%s", service.lastBindingPreview, m.Mode)
	}
	m, mutate := apply(m, "enter")
	if mutate == nil || service.bindingCalls != 0 {
		t.Fatalf("binding mutation not deferred: cmd=%v calls=%d", mutate != nil, service.bindingCalls)
	}
	_ = mutate()
	if service.bindingCalls != 1 || service.lastBindingEnable {
		t.Fatalf("binding mutation enable=%v request=%+v", service.lastBindingEnable, service.lastBinding)
	}
}

func TestMigrationActionUsesExactDryRunPreviewThenSelectorExecution(t *testing.T) {
	item := app.ScanItem{
		Key: "exact", Origin: "g/codex", Target: "/work/.codex/skills/loose", ObjectID: "object", RootObjectID: "root",
		ContentHash: "content", MatchedLibraryHash: "library", State: app.ScanStateUnmanaged, Action: app.ScanActionImport,
		Skill: config.Skill{ID: "local/loose", Name: "loose"},
	}
	migration := &fakeMigration{result: app.ScanResult{Items: []app.ScanItem{item}}}
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, migration)
	m.switchView(ViewMigration)
	m.Inventory.Items = []app.ScanItem{item}

	m, preview := apply(m, "enter")
	if preview == nil || migration.scanCalls != 0 {
		t.Fatalf("migration preview must be deferred: cmd=%v calls=%d", preview != nil, migration.scanCalls)
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if migration.scanCalls != 1 || !migration.requests[0].DryRun || migration.requests[0].Adopt || len(migration.requests[0].Selectors) != 1 {
		t.Fatalf("migration preview request=%+v", migration.requests[0])
	}
	m, execute := apply(m, "enter")
	if execute == nil {
		t.Fatal("migration confirmation did not produce execution command")
	}
	_ = execute()
	got := migration.requests[1]
	if got.DryRun || got.Adopt || len(got.Selectors) != 1 || got.Selectors[0].ExpectedObjectID != "object" || got.Selectors[0].ExpectedRootID != "root" || got.Selectors[0].ExpectedHash != "content" {
		t.Fatalf("migration execution lost exact selector: %+v", got)
	}
}

func TestStatusSyncUsesDryRunPreviewAndCancelDoesNotMutate(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), syncResult: app.Result{Plan: link.Plan{Actions: []link.Action{{Path: "/work/link"}}}}}
	m := loadedModel(t, service, &fakeMigration{})
	m.switchView(ViewStatus)
	m.Snapshot.Status.Items[0].Kind = status.Missing

	m, preview := apply(m, "s")
	if preview == nil || service.syncCalls != 0 {
		t.Fatalf("sync preview not deferred: cmd=%v calls=%d", preview != nil, service.syncCalls)
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if service.syncCalls != 1 || !service.lastSync.DryRun || m.Mode != ModeConfirm {
		t.Fatalf("sync preview request=%+v mode=%s", service.lastSync, m.Mode)
	}
	m, cancel := apply(m, "esc")
	if cancel != nil || service.syncCalls != 1 || m.Mode == ModeConfirm {
		t.Fatalf("sync cancel mutated: cmd=%v calls=%d mode=%s", cancel != nil, service.syncCalls, m.Mode)
	}
}

func TestLibraryAddLocalPreviewSelectConfirmAndRemoteNetworkGate(t *testing.T) {
	local := &fakeService{snapshot: testSnapshot(), addPreview: app.AddPreview{Candidates: []app.Candidate{
		{Name: "one", RelativePath: "skills/one"}, {Name: "two", RelativePath: "skills/two"},
	}}}
	m := loadedModel(t, local, &fakeMigration{})
	m, _ = apply(m, "a")
	if m.Mode != ModeInput {
		t.Fatalf("add source mode=%s", m.Mode)
	}
	for _, char := range "/tmp/source" {
		m, _ = apply(m, string(char))
	}
	m, preview := apply(m, "enter")
	if preview == nil || local.previewAddCalls != 0 || local.addCalls != 0 {
		t.Fatalf("local add preview cmd=%v previewCalls=%d addCalls=%d", preview != nil, local.previewAddCalls, local.addCalls)
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeAddSelect || len(m.rows()) != 2 {
		t.Fatalf("local add selection mode=%s rows=%+v", m.Mode, m.rows())
	}
	m, _ = apply(m, "space")
	m, _ = apply(m, "enter")
	if m.Mode != ModeConfirm || local.addCalls != 0 {
		t.Fatalf("local add did not stop at confirmation: mode=%s calls=%d", m.Mode, local.addCalls)
	}
	m, add := apply(m, "enter")
	if add == nil {
		t.Fatal("confirmed local add returned no command")
	}
	_ = add()
	if local.addCalls != 1 || local.lastAdd.Source != "/tmp/source" || len(local.lastAdd.Skills) != 1 || local.lastAdd.Skills[0] != "skills/one" {
		t.Fatalf("local add request=%+v", local.lastAdd)
	}

	remote := &fakeService{snapshot: testSnapshot()}
	remote.addPreviewFunc = func(request app.AddPreviewRequest) app.AddPreview {
		if !request.AllowNetwork {
			return app.AddPreview{
				NetworkRequired: true, ResolvedSource: "https://github.com/vercel-labs/agent-skills.git",
				SuggestedSelections: []string{"find-skills"}, Warnings: []string{"network required"},
			}
		}
		return app.AddPreview{
			ResolvedSource:      "https://github.com/vercel-labs/agent-skills.git",
			SuggestedSelections: []string{"find-skills"}, Ref: &config.Ref{Kind: "branch", Value: "main"},
			Resolved: strings.Repeat("a", 40), Candidates: []app.Candidate{
				{Name: "other", RelativePath: "skills/other", Hash: "other-hash"},
				{Name: "find-skills", RelativePath: "skills/find-skills", Hash: "find-hash"},
			},
		}
	}
	m = loadedModel(t, remote, &fakeMigration{})
	m, _ = apply(m, "a")
	for _, char := range "https://skills.sh/vercel-labs/agent-skills/find-skills" {
		m, _ = apply(m, string(char))
	}
	m, preview = apply(m, "enter")
	next, _ = m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || remote.addCalls != 0 || !strings.Contains(m.ViewString(), "network") {
		t.Fatalf("remote source skipped network gate: mode=%s calls=%d\n%s", m.Mode, remote.addCalls, m.ViewString())
	}
	cancelled, cancelCmd := apply(m, "esc")
	if cancelCmd != nil || remote.addCalls != 0 || cancelled.Mode == ModeConfirm {
		t.Fatalf("network gate cancel mutated: cmd=%v calls=%d mode=%s", cancelCmd != nil, remote.addCalls, cancelled.Mode)
	}
	regions := m.hitRegions()
	if regions.Confirm.Empty() {
		t.Fatal("network confirmation has no mouse hit region")
	}
	next, discover := m.Update(click(regions.Confirm.X, regions.Confirm.Y))
	m = next.(Model)
	if discover == nil || remote.addCalls != 0 {
		t.Fatalf("network confirmation did not defer mutation: cmd=%v calls=%d", discover != nil, remote.addCalls)
	}
	next, _ = m.Update(discover())
	m = next.(Model)
	if !remote.lastPreviewAdd.AllowNetwork || m.Mode != ModeAddSelect || !m.Selected["skills/find-skills"] || m.Selected["skills/other"] {
		t.Fatalf("remote discovery request=%+v mode=%s selected=%v", remote.lastPreviewAdd, m.Mode, m.Selected)
	}
	m, _ = apply(m, "enter")
	if m.Mode != ModeConfirm {
		t.Fatalf("remote selection did not reach final confirmation: %s", m.Mode)
	}
	m, add = apply(m, "enter")
	_ = add()
	if remote.addCalls != 1 || remote.lastAdd.Source != "https://github.com/vercel-labs/agent-skills.git" || len(remote.lastAdd.Skills) != 1 || remote.lastAdd.Skills[0] != "skills/find-skills" {
		t.Fatalf("remote add request=%+v calls=%d", remote.lastAdd, remote.addCalls)
	}
	if remote.lastAdd.ExpectedResolved != strings.Repeat("a", 40) || len(remote.lastAdd.ExpectedCandidates) != 1 || remote.lastAdd.ExpectedCandidates[0].Hash != "find-hash" {
		t.Fatalf("remote confirmation token=%+v", remote.lastAdd)
	}
}

func TestRemoteAddShowsStaleSkillsShSuggestionWarningWithCurrentCandidates(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot()}
	service.addPreviewFunc = func(request app.AddPreviewRequest) app.AddPreview {
		if !request.AllowNetwork {
			return app.AddPreview{NetworkRequired: true, ResolvedSource: "https://github.com/vercel-labs/agent-skills.git"}
		}
		return app.AddPreview{
			ResolvedSource: "https://github.com/vercel-labs/agent-skills.git",
			Candidates:     []app.Candidate{{Name: "current-skill", RelativePath: "skills/current-skill"}},
			Warnings:       []string{"skills.sh suggested skill find-skills is no longer present in the repository; choose from the current candidates"},
		}
	}
	m := loadedModel(t, service, &fakeMigration{})
	m, _ = apply(m, "a")
	for _, char := range "https://skills.sh/vercel-labs/agent-skills/find-skills" {
		m, _ = apply(m, string(char))
	}
	m, preview := apply(m, "enter")
	next, _ := m.Update(preview())
	m = next.(Model)
	m, discover := apply(m, "enter")
	next, _ = m.Update(discover())
	m = next.(Model)
	if m.Mode != ModeAddSelect || !strings.Contains(m.Status, "find-skills") || !strings.Contains(m.Status, "no longer present") {
		t.Fatalf("stale skills.sh warning hidden: mode=%s status=%q", m.Mode, m.Status)
	}
}

func TestPresetCreateAndMemberEditUseConfirmedTypedMutation(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), presetMutationPreview: app.MutationPreview{Title: "Create preset", Summary: "Create preset new"}}
	m := loadedModel(t, service, &fakeMigration{})
	m.switchView(ViewPresets)
	m, _ = apply(m, "n")
	for _, char := range "new" {
		m, _ = apply(m, string(char))
	}
	m, preview := apply(m, "enter")
	if preview == nil || service.previewPresetMutationCalls != 0 || service.mutatePresetCalls != 0 {
		t.Fatalf("preset create preview cmd=%v previewCalls=%d mutationCalls=%d", preview != nil, service.previewPresetMutationCalls, service.mutatePresetCalls)
	}
	next, _ := m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || service.lastPresetMutationPreview.Operation != app.PresetCreate || service.lastPresetMutationPreview.Name != "new" {
		t.Fatalf("preset create preview=%+v mode=%s", service.lastPresetMutationPreview, m.Mode)
	}
	m, create := apply(m, "enter")
	_ = create()
	if service.mutatePresetCalls != 1 || service.lastPresetMutation.Operation != app.PresetCreate || service.lastPresetMutation.Name != "new" || !service.lastPresetMutation.Confirmed {
		t.Fatalf("preset create mutation=%+v", service.lastPresetMutation)
	}

	service.mutatePresetCalls = 0
	service.snapshotCalls = 0
	m = loadedModel(t, service, &fakeMigration{})
	m.switchView(ViewPresets)
	m, _ = apply(m, "enter")
	m.Cursor = 1
	m, cmd := apply(m, "space")
	if cmd != nil || service.mutatePresetCalls != 0 {
		t.Fatalf("member toggle mutated immediately: cmd=%v calls=%d", cmd != nil, service.mutatePresetCalls)
	}
	m, preview = apply(m, "w")
	if preview == nil || service.mutatePresetCalls != 0 {
		t.Fatalf("member save skipped typed preview: cmd=%v calls=%d", preview != nil, service.mutatePresetCalls)
	}
	next, _ = m.Update(preview())
	m = next.(Model)
	if m.Mode != ModeConfirm || service.mutatePresetCalls != 0 {
		t.Fatalf("member preview did not enter confirmation: mode=%s calls=%d", m.Mode, service.mutatePresetCalls)
	}
	m, save := apply(m, "enter")
	_ = save()
	if service.mutatePresetCalls != 1 || service.lastPresetMutation.Operation != app.PresetEditMembers || len(service.lastPresetMutation.Skills) != 2 {
		t.Fatalf("member mutation=%+v", service.lastPresetMutation)
	}
}

func TestPresetMembersCanBeChangedWithMouseAndSaved(t *testing.T) {
	service := &fakeService{
		snapshot:              testSnapshot(),
		presetMutationPreview: app.MutationPreview{Title: "Edit preset members", Summary: "Update review"},
	}
	m := loadedModel(t, service, &fakeMigration{})
	m.Width, m.Height = 100, 24
	m.switchView(ViewPresets)
	m, _ = apply(m, "enter")

	regions := m.hitRegions()
	if len(regions.Rows) != 2 || len(regions.Checkboxes) != 2 {
		t.Fatalf("preset member hit regions rows=%d checkboxes=%d, want 2 each", len(regions.Rows), len(regions.Checkboxes))
	}

	// Clicking the member row itself is equivalent to Space in this checklist.
	betaRow := regions.Rows[1]
	next, _ := m.Update(tea.MouseMsg{X: betaRow.X + betaRow.Width - 1, Y: betaRow.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if !m.Selected["acme/beta"] {
		t.Fatal("clicking beta member did not select it")
	}

	// The checkbox remains an explicit hit target and can remove an existing member.
	regions = m.hitRegions()
	alphaCheckbox := regions.Checkboxes[0]
	next, _ = m.Update(tea.MouseMsg{X: alphaCheckbox.X, Y: alphaCheckbox.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.Selected["acme/alpha"] {
		t.Fatal("clicking alpha checkbox did not remove it")
	}

	m, preview := mouseActionByLabel(t, m, "Save")
	if preview == nil {
		t.Fatal("Save did not defer the preset preview")
	}
	_ = preview()
	want := []string{"acme/beta"}
	if !reflect.DeepEqual(service.lastPresetMutationPreview.Skills, want) {
		t.Fatalf("preview skills=%v, want %v", service.lastPresetMutationPreview.Skills, want)
	}
}

func TestPresetMemberSelectionSurvivesSuccessfulSaveUntilSnapshot(t *testing.T) {
	m := Model{
		ActiveView: ViewPresets,
		Scope:      Scope{Preset: "review", Level: "preset-skills"},
		Selected:   map[string]bool{"acme/beta": true},
	}
	next, _ := m.Update(operationMsg{name: "preset"})
	got := next.(Model)
	if !got.Selected["acme/beta"] {
		t.Fatal("successful preset save cleared the edited members before the refreshed snapshot arrived")
	}
}

func TestPrimaryActionButtonsHaveKeyboardAndMouseParity(t *testing.T) {
	views := []View{ViewLibrary, ViewWorkspaces, ViewPresets, ViewMigration, ViewStatus}
	for _, view := range views {
		t.Run(string(view), func(t *testing.T) {
			base := NewModel(nil, &fakeService{}, &fakeMigration{}, view, ActionNone)
			base.Snapshot = testSnapshot()
			base.Width, base.Height = 100, 20
			if view == ViewMigration {
				base.Inventory.Items = testInventoryItems()
			}
			actions := base.primaryActions()
			regions := base.hitRegions()
			if len(actions) == 0 || len(actions) != len(regions.Actions) {
				t.Fatalf("actions=%v regions=%d", actions, len(regions.Actions))
			}
			for i := range actions {
				mouseModel, mouseCmd := base.Update(click(regions.Actions[i].X, regions.Actions[i].Y))
				keyboardModel, keyboardCmd := base.performPrimaryAction(i)
				mouse := mouseModel.(Model)
				keyboard := keyboardModel.(Model)
				if mouse.ActiveView != keyboard.ActiveView || mouse.Mode != keyboard.Mode || mouse.Detail != keyboard.Detail || (mouseCmd == nil) != (keyboardCmd == nil) {
					t.Fatalf("action %q parity mismatch mouse=(%s,%s,%v,%v) keyboard=(%s,%s,%v,%v)", actions[i], mouse.ActiveView, mouse.Mode, mouse.Detail, mouseCmd != nil, keyboard.ActiveView, keyboard.Mode, keyboard.Detail, keyboardCmd != nil)
				}
			}
		})
	}
}

func TestPrimaryActionsExposeLibraryAddAndSessionMigrationIgnore(t *testing.T) {
	m := NewModel(nil, &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	m.Snapshot = testSnapshot()
	m.Width, m.Height = 100, 20
	if got := strings.Join(m.primaryActions(), ","); !strings.Contains(got, "Add source") || !strings.Contains(got, "More") || len(m.primaryActions()) > 3 {
		t.Fatalf("library actions=%v", m.primaryActions())
	}
	regions := m.hitRegions()
	addIndex := -1
	for i, label := range m.primaryActions() {
		if label == "Add source" {
			addIndex = i
		}
	}
	if addIndex < 0 {
		t.Fatal("Add source button missing")
	}
	next, cmd := m.Update(click(regions.Actions[addIndex].X, regions.Actions[addIndex].Y))
	if cmd != nil || next.(Model).Mode != ModeInput {
		t.Fatalf("Add source click mode=%s cmd=%v", next.(Model).Mode, cmd != nil)
	}

	m.switchView(ViewMigration)
	m.Inventory.Items = testInventoryItems()
	if got := strings.Join(m.primaryActions(), ","); !strings.Contains(got, "Ignore") {
		t.Fatalf("migration actions=%v", m.primaryActions())
	}
	ignore := 0
	for i, label := range m.primaryActions() {
		if label == "Ignore" {
			ignore = i
		}
	}
	next, cmd = m.performPrimaryAction(ignore)
	m = next.(Model)
	if cmd != nil || len(m.rows()) != 0 || len(m.Inventory.Items) != 1 {
		t.Fatalf("session ignore mutated inventory or remained visible: rows=%d inventory=%d cmd=%v", len(m.rows()), len(m.Inventory.Items), cmd != nil)
	}
}

func TestMigrationConfirmationShowsExactLibraryBindingAndPath(t *testing.T) {
	item := app.ScanItem{
		Key: "adopt", Origin: "p/aikit/codex", Target: "/work/aikit/.codex/skills/loose", Project: "aikit", Agent: "codex",
		State: app.ScanStateUnmanaged, Action: app.ScanActionAdopt, Skill: config.Skill{ID: "local/loose", Name: "loose"},
	}
	migration := &fakeMigration{result: app.ScanResult{Items: []app.ScanItem{item}}}
	m := loadedModel(t, &fakeService{snapshot: testSnapshot()}, migration)
	m.switchView(ViewMigration)
	m.Inventory.Items = []app.ScanItem{item}
	m, preview := apply(m, "enter")
	next, _ := m.Update(preview())
	got := next.(Model).ViewString()
	for _, wanted := range []string{"Library: local/loose", "Binding: aikit / codex", "Replace: /work/aikit/.codex/skills/loose"} {
		if !strings.Contains(got, wanted) {
			t.Fatalf("migration preview missing %q:\n%s", wanted, got)
		}
	}
}

func TestMigrationCompareUsesTypedAppCompareForMatchedLibraryItems(t *testing.T) {
	service := &fakeService{snapshot: testSnapshot(), compareResult: app.CompareResult{
		Left:  app.CompareSide{Skill: config.Skill{ID: "acme/alpha", Name: "alpha"}},
		Right: app.CompareSide{Skill: config.Skill{ID: "acme/beta", Name: "beta"}},
		Files: []app.FileDiff{{Path: "SKILL.md", Kind: app.FileChanged}},
	}}
	m := loadedModel(t, service, &fakeMigration{})
	m.switchView(ViewMigration)
	m.Inventory.Items = []app.ScanItem{{Key: "conflict", MatchedLibraryID: "acme/alpha", Skill: config.Skill{ID: "acme/beta", Name: "beta"}, State: app.ScanStateNameConflict, Action: app.ScanActionConflict}}
	compareIndex := 0
	for i, label := range m.primaryActions() {
		if label == "Compare" {
			compareIndex = i
		}
	}
	model, cmd := m.performPrimaryAction(compareIndex)
	m = model.(Model)
	if cmd == nil || service.compareCalls != 0 {
		t.Fatalf("compare cmd=%v calls=%d", cmd != nil, service.compareCalls)
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if service.compareCalls != 1 || service.lastCompare.LeftSkillID != "acme/alpha" || service.lastCompare.RightSkillID != "acme/beta" || !strings.Contains(m.ViewString(), "SKILL.md") {
		t.Fatalf("compare request=%+v calls=%d\n%s", service.lastCompare, service.compareCalls, m.ViewString())
	}
}
