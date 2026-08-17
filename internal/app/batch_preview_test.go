package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/silenceper/aikit/internal/link"
	"github.com/silenceper/aikit/pkg/config"
)

func TestPreviewBatchBindingsIsReadOnlyAndAtomic(t *testing.T) {
	application, paths, _, _ := testApp(t)
	store := config.Store{Paths: paths}
	before, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	recoverCalls, executeCalls := 0, 0
	application.deps.Recover = func(string, []config.PendingOperation, link.Selector, bool) link.Result {
		recoverCalls++
		return link.Result{}
	}
	application.deps.Execute = func(link.Plan, bool) link.Result {
		executeCalls++
		return link.Result{}
	}
	request := BatchRequest{Operation: BatchEnable, Bindings: []BindingRequest{
		{SkillID: "local/demo", Agent: "cursor"},
		{SkillID: "local/demo", Project: "demo", Agent: "cursor"},
	}}
	preview, err := application.PreviewBatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 2 || preview.Title == "" || !preview.RequiresConfirmation || len(preview.AffectedScopes) == 0 {
		t.Fatalf("binding preview = %+v", preview)
	}
	after, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || recoverCalls != 0 || executeCalls != 0 {
		t.Fatalf("preview mutated state: config=%v recover=%d execute=%d", !reflect.DeepEqual(before, after), recoverCalls, executeCalls)
	}
}

func TestPreviewBatchRemoveAggregatesReferencesAndRequiresForce(t *testing.T) {
	application, _, _, _ := testApp(t)
	if _, err := application.Enable(context.Background(), BindingRequest{SkillID: "local/demo", Agent: "cursor"}); err != nil {
		t.Fatal(err)
	}
	preview, err := application.PreviewBatch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresForce || len(preview.References) == 0 || len(preview.Items) != 1 || preview.Items[0].Item != "local/demo" {
		t.Fatalf("remove preview = %+v", preview)
	}
	forced, err := application.PreviewBatch(context.Background(), BatchRequest{Operation: BatchRemove, SkillIDs: []string{"local/demo"}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if forced.RequiresForce || len(forced.Plan.Actions) == 0 {
		t.Fatalf("forced remove preview = %+v", forced)
	}
}

func TestPreviewBatchUpdateRequiresCompleteCurrentAndRemoteIdentity(t *testing.T) {
	application, paths, _, _ := testApp(t)
	store := config.Store{Paths: paths}
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Library.Skills[0].Source = "https://example.test/demo.git"
	cfg.Library.Skills[0].SourcePath = "."
	cfg.Library.Skills[0].Resolved = strings.Repeat("a", 40)
	cfg.Library.Skills[0].Ref = &config.Ref{Kind: "branch", Value: "main"}
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	request := BatchRequest{Operation: BatchUpdate, SkillIDs: []string{"local/demo"}, Expected: map[string]ExpectedUpdate{
		"local/demo": {Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40)},
	}}
	if _, err := application.PreviewBatch(context.Background(), request); err == nil || !strings.Contains(err.Error(), "remote") {
		t.Fatalf("incomplete update identity error = %v", err)
	}
	request.Expected["local/demo"] = ExpectedUpdate{Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("a", 40), Remote: strings.Repeat("b", 40)}
	preview, err := application.PreviewBatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 1 || !strings.Contains(preview.Summary, "local/demo") || !strings.Contains(preview.Summary, strings.Repeat("b", 8)) {
		t.Fatalf("update preview = %+v", preview)
	}
	request.Expected["local/demo"] = ExpectedUpdate{Ref: &config.Ref{Kind: "branch", Value: "main"}, Resolved: strings.Repeat("c", 40), Remote: strings.Repeat("b", 40)}
	if _, err := application.PreviewBatch(context.Background(), request); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale update identity error = %v", err)
	}
}
