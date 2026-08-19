package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/silenceper/aikit/internal/app"
)

func TestActivityKindsKeepDistinctSemanticMarkers(t *testing.T) {
	tests := []struct {
		kind   ActivityKind
		marker string
	}{
		{ActivityReading, "⠋"},
		{ActivityNetwork, "⠋"},
		{ActivityMutating, "⠋"},
		{ActivitySuccess, "✓"},
		{ActivityWarning, "!"},
		{ActivityError, "×"},
	}
	for _, mode := range []themeMode{themeDark, themeLight, themeReduced, themeNoColor} {
		for _, tt := range tests {
			activity := Activity{Kind: tt.kind, Label: "状态处理", Frame: 0}
			got := stripANSI(renderActivity(activity, 40, newSemanticTheme(mode)))
			if !strings.HasPrefix(got, tt.marker+" ") || !strings.Contains(got, "状态处理") {
				t.Fatalf("mode=%s kind=%d rendered=%q", mode, tt.kind, got)
			}
			if mode == themeNoColor && strings.Contains(renderActivity(activity, 40, newSemanticTheme(mode)), "\x1b[") {
				t.Fatalf("NO_COLOR emitted ANSI: %q", renderActivity(activity, 40, newSemanticTheme(mode)))
			}
		}
	}
}

func TestActivityTextNeverInventsProgress(t *testing.T) {
	activity := Activity{Kind: ActivityNetwork, Label: "Checking updates"}
	got := stripANSI(renderActivity(activity, 38, newSemanticTheme(themeNoColor)))
	if strings.Contains(got, "%") || strings.Contains(got, "/") || got != "⠋ Checking updates" {
		t.Fatalf("indeterminate activity=%q", got)
	}

	activity.Current, activity.Total = 12, 33
	got = stripANSI(renderActivity(activity, 38, newSemanticTheme(themeNoColor)))
	if got != "⠋ Checking updates · 12/33" {
		t.Fatalf("determinate activity=%q", got)
	}
}

func TestActivityReviewTargetAndItemRemainTyped(t *testing.T) {
	activity := Activity{
		Kind: ActivityWarning, Label: "Completed with issues", Item: "技能👩‍💻",
		Review: ReviewTarget{Kind: ReviewBatchResult, Key: "generation:42"},
	}
	if activity.Review.Kind != ReviewBatchResult || activity.Review.Key != "generation:42" {
		t.Fatalf("review target=%+v", activity.Review)
	}
	got := stripANSI(renderActivity(activity, 80, newSemanticTheme(themeNoColor)))
	if !strings.Contains(got, "技能👩‍💻") || !strings.Contains(got, "Tab or click to review") {
		t.Fatalf("reviewable activity=%q", got)
	}
}

func TestActivityRenderingIsDisplayCellBounded(t *testing.T) {
	activity := Activity{Kind: ActivityReading, Label: "正在扫描技能👩‍💻管理目录", Item: "项目界面"}
	for _, width := range []int{24, 38, 80, 120} {
		got := renderActivity(activity, width, newSemanticTheme(themeNoColor))
		if cells := lipgloss.Width(got); cells > width {
			t.Fatalf("width=%d rendered cells=%d: %q", width, cells, got)
		}
	}
}

func TestActivitySpinnerAndExpiryIgnoreStaleGenerations(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Activity = Activity{Kind: ActivityReading, Label: "Scanning", Generation: 2}
	m.activityGeneration = 2

	next, follow := m.Update(activityTickMsg{Generation: 1})
	m = next.(Model)
	if m.Activity.Frame != 0 || follow != nil {
		t.Fatalf("stale tick changed activity: %+v follow=%v", m.Activity, follow != nil)
	}
	next, _ = m.Update(activityTickMsg{Generation: 2})
	m = next.(Model)
	if m.Activity.Frame != 1 {
		t.Fatalf("current tick frame=%d want=1", m.Activity.Frame)
	}

	m.Activity = Activity{Kind: ActivitySuccess, Label: "Added", Generation: 3}
	m.activityGeneration = 3
	next, _ = m.Update(activityExpireMsg{Generation: 2})
	m = next.(Model)
	if m.Activity.Kind != ActivitySuccess {
		t.Fatalf("stale expiry cleared activity: %+v", m.Activity)
	}
	next, _ = m.Update(activityExpireMsg{Generation: 3})
	m = next.(Model)
	if m.Activity.Kind != ActivityIdle {
		t.Fatalf("current expiry did not clear activity: %+v", m.Activity)
	}
}

func TestActivityCommandEnvelopeDropsStaleTypedResult(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Activity = Activity{Kind: ActivityReading, Label: "Newer work", Generation: 8}
	m.activityGeneration = 8
	wanted := testSnapshot()
	wanted.Config.Library.Skills = wanted.Config.Library.Skills[:1]

	next, cmd := m.Update(activityResultMsg{Generation: 7, Message: snapshotMsg{snapshot: wanted}})
	m = next.(Model)
	if cmd != nil || len(m.Snapshot.Config.Library.Skills) != 0 || m.Activity.Generation != 8 {
		t.Fatalf("stale result mutated model: skills=%d activity=%+v", len(m.Snapshot.Config.Library.Skills), m.Activity)
	}

	next, _ = m.Update(activityResultMsg{Generation: 8, Message: snapshotMsg{snapshot: wanted}})
	m = next.(Model)
	if len(m.Snapshot.Config.Library.Skills) != 1 {
		t.Fatalf("current typed result was not unwrapped: %+v", m.Snapshot.Config.Library.Skills)
	}
}

func TestBeginActivityCommandEmitsTicksUntilTypedResult(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	release := make(chan struct{})
	work := func() tea.Msg {
		<-release
		return configurationValidationMsg{result: app.ConfigurationValidation{Valid: true}}
	}
	cmd := m.beginActivity(ActivityReading, "Validating configuration", "", work)
	if cmd == nil || !m.Busy || m.Activity.Kind != ActivityReading {
		t.Fatalf("activity did not start: busy=%v activity=%+v", m.Busy, m.Activity)
	}

	started := time.Now()
	message := cmd()
	if time.Since(started) < activityTickInterval/2 {
		t.Fatalf("slow command returned before spinner interval: %T", message)
	}
	tick, ok := message.(activityTickMsg)
	if !ok || tick.Generation != m.Activity.Generation || tick.results == nil {
		t.Fatalf("first message=%T %+v", message, message)
	}

	close(release)
	next, follow := m.Update(tick)
	m = next.(Model)
	if follow == nil || m.Activity.Frame != 1 {
		t.Fatalf("tick update frame=%d follow=%v", m.Activity.Frame, follow != nil)
	}
	result, ok := follow().(activityResultMsg)
	if !ok {
		t.Fatalf("follow message=%T", follow())
	}
	if _, ok := result.Message.(configurationValidationMsg); !ok {
		t.Fatalf("typed result=%T", result.Message)
	}
}

func TestMutatingActivityKeepsAnimatingUntilCommandReturns(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewLibrary, ActionNone)
	release := make(chan struct{})
	cmd := m.beginActivity(ActivityMutating, "Adding selected skills", "", func() tea.Msg {
		<-release
		return operationMsg{name: "add"}
	})
	if cmd == nil || !m.MutationBusy {
		t.Fatalf("mutation activity did not start: busy=%v cmd=%v", m.MutationBusy, cmd != nil)
	}

	tick, ok := cmd().(activityTickMsg)
	if !ok {
		t.Fatalf("slow mutation first message=%T, want activityTickMsg", cmd())
	}
	next, follow := m.Update(tick)
	m = next.(Model)
	if m.Activity.Frame != 1 || follow == nil {
		t.Fatalf("mutation did not animate: frame=%d follow=%v", m.Activity.Frame, follow != nil)
	}
	close(release)
	if _, ok := follow().(activityResultMsg); !ok {
		t.Fatalf("mutation follow message=%T", follow())
	}
	if got := stripANSI(renderActivity(m.Activity, 80, newSemanticTheme(themeDark))); !strings.Contains(got, activitySpinnerFrames[1]) {
		t.Fatalf("mutation activity rendered without spinner frame: %q", got)
	}
}

func TestActivityRenderReplacesVisibleBusyState(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot = testSnapshot()
	m.Busy = true
	m.Activity = Activity{Kind: ActivityNetwork, Label: "Checking updates", Current: 12, Total: 33, Generation: 1}
	for _, width := range []int{24, 38, 80, 120} {
		m.Width, m.Height = width, 20
		view := m.ViewString()
		plain := strings.ToLower(stripANSI(view))
		if strings.Contains(plain, "busy") || !strings.Contains(plain, "checking updates") {
			t.Fatalf("width=%d activity not rendered semantically:\n%s", width, stripANSI(view))
		}
		for index, line := range strings.Split(view, "\n") {
			if cells := lipgloss.Width(line); cells > width {
				t.Fatalf("width=%d line=%d cells=%d: %q", width, index, cells, line)
			}
		}
	}
}

func TestActivityFooterHitGeometryMatchesRenderedStatus(t *testing.T) {
	m := NewModel(context.Background(), &fakeService{}, &fakeMigration{}, ViewOverview, ActionNone)
	m.Snapshot, m.Width, m.Height = testSnapshot(), 80, 20
	m.Activity = Activity{
		Kind: ActivityWarning, Label: "Completed with issues", Generation: 9,
		Review: ReviewTarget{Kind: ReviewBatchResult, Key: "9"},
	}
	regions := m.hitRegions()
	if regions.ActivityStatus.Empty() || !regions.Layout.FooterPanel.Body.Contains(regions.ActivityStatus.X, regions.ActivityStatus.Y) {
		t.Fatalf("activity hit rect=%+v footer=%+v", regions.ActivityStatus, regions.Layout.FooterPanel.Body)
	}
	line := stripANSI(strings.Split(m.ViewString(), "\n")[regions.ActivityStatus.Y])
	if !strings.Contains(line, "! Completed with issues") {
		t.Fatalf("activity row mismatch y=%d line=%q", regions.ActivityStatus.Y, line)
	}
}
