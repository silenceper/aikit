package tui

import (
	"fmt"
	"strings"
)

type ActivityKind uint8

const (
	ActivityIdle ActivityKind = iota
	ActivityReading
	ActivityNetwork
	ActivityMutating
	ActivitySuccess
	ActivityWarning
	ActivityError
)

type ReviewKind uint8

const (
	ReviewNone ReviewKind = iota
	ReviewFullError
	ReviewBatchResult
	ReviewOperationResult
	ReviewRecoveryResult
	ReviewStatusItem
	ReviewUpdateCheck
	ReviewInventoryIssue
)

type ReviewTarget struct {
	Kind ReviewKind
	Key  string
}

func (target ReviewTarget) Empty() bool { return target.Kind == ReviewNone }

type Activity struct {
	Kind       ActivityKind
	Label      string
	Item       string
	Current    int
	Total      int
	Frame      int
	Generation uint64
	Review     ReviewTarget
}

var activitySpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func renderActivity(activity Activity, width int, theme semanticTheme) string {
	if width <= 0 || activity.Kind == ActivityIdle || strings.TrimSpace(activity.Label) == "" {
		return ""
	}
	marker := ""
	switch activity.Kind {
	case ActivityReading, ActivityNetwork:
		marker = activitySpinnerFrames[activity.Frame%len(activitySpinnerFrames)]
	case ActivityMutating:
		marker = "●"
	case ActivitySuccess:
		marker = "✓"
	case ActivityWarning:
		marker = "!"
	case ActivityError:
		marker = "×"
	}
	parts := []string{marker + " " + strings.TrimSpace(activity.Label)}
	if item := strings.TrimSpace(activity.Item); item != "" {
		parts = append(parts, item)
	}
	if activity.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", max(0, activity.Current), activity.Total))
	}
	if !activity.Review.Empty() {
		parts = append(parts, "Tab or click to review")
	}
	return clip(theme.activity(activity.Kind, strings.Join(parts, " · ")), width)
}
