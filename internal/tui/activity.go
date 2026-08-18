package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

const (
	activityTickInterval = 100 * time.Millisecond
	activitySuccessTTL   = 3 * time.Second
)

type activityTickMsg struct {
	Generation uint64
	results    <-chan tea.Msg
}

type activityExpireMsg struct{ Generation uint64 }

type activityResultMsg struct {
	Generation uint64
	Message    tea.Msg
}

func (m *Model) beginActivity(kind ActivityKind, label, item string, command tea.Cmd) tea.Cmd {
	m.activityGeneration++
	m.Activity = Activity{Kind: kind, Label: label, Item: item, Generation: m.activityGeneration}
	switch kind {
	case ActivityMutating:
		m.Busy, m.MutationBusy = true, true
	case ActivityReading, ActivityNetwork:
		m.Busy = true
	}
	if command == nil {
		return nil
	}
	generation := m.activityGeneration
	results := make(chan tea.Msg, 1)
	return func() tea.Msg {
		go func() { results <- command() }()
		return waitActivityResultCmd(generation, results)()
	}
}

func waitActivityResultCmd(generation uint64, results <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		timer := time.NewTimer(activityTickInterval)
		defer timer.Stop()
		select {
		case message := <-results:
			return activityResultMsg{Generation: generation, Message: message}
		case <-timer.C:
			return activityTickMsg{Generation: generation, results: results}
		}
	}
}

func (m *Model) finishActivity(kind ActivityKind, label, item string, review ReviewTarget) tea.Cmd {
	m.activityGeneration++
	m.Activity = Activity{Kind: kind, Label: label, Item: item, Generation: m.activityGeneration, Review: review}
	m.Busy, m.MutationBusy = false, false
	if kind != ActivitySuccess {
		return nil
	}
	generation := m.activityGeneration
	return tea.Tick(activitySuccessTTL, func(time.Time) tea.Msg { return activityExpireMsg{Generation: generation} })
}

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
	plain := strings.Join(parts, " · ")
	if width > 0 {
		plain = clipPlain(plain, width)
	}
	return theme.activity(activity.Kind, plain)
}

func (m Model) displayedActivity() Activity {
	if m.Activity.Kind != ActivityIdle {
		return m.Activity
	}
	if m.Inventory.Loading {
		return Activity{Kind: ActivityReading, Label: "Scanning local inventory", Current: m.Inventory.Completed, Total: m.Inventory.Total}
	}
	return Activity{}
}

func activityShortPhase(activity Activity) string {
	switch activity.Kind {
	case ActivityNetwork:
		return "checking"
	case ActivityMutating:
		return "applying"
	case ActivityReading:
		if strings.Contains(strings.ToLower(activity.Label), "scan") {
			return "scanning"
		}
		return "loading"
	default:
		return ""
	}
}

func (m Model) readingActivityActive() bool {
	return m.Busy && !m.MutationBusy
}

func activityAllowsAction(action uiAction) bool {
	switch action {
	case uiMoveUp, uiMoveDown, uiCancel, uiBack:
		return true
	default:
		return false
	}
}
