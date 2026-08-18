package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestActivityKindsKeepDistinctSemanticMarkers(t *testing.T) {
	tests := []struct {
		kind   ActivityKind
		marker string
	}{
		{ActivityReading, "⠋"},
		{ActivityNetwork, "⠋"},
		{ActivityMutating, "●"},
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
