package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
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

func (m Model) openActivityReview() (tea.Model, tea.Cmd) {
	target := m.displayedActivity().Review
	if target.Empty() {
		return m, nil
	}
	title, lines := "Activity details", m.activityReviewLines(target)
	switch target.Kind {
	case ReviewFullError:
		title = "Error details"
	case ReviewBatchResult:
		title = "Batch result"
	case ReviewOperationResult:
		title = "Operation result"
	case ReviewRecoveryResult:
		title = "Recovery result"
	case ReviewStatusItem:
		title = "Status issue"
	case ReviewUpdateCheck:
		title = "Update check"
	case ReviewInventoryIssue:
		title = "Inventory issue"
	}
	if len(lines) == 0 {
		lines = []string{firstNonEmpty(m.Err, m.Status, "No additional details")}
	}
	m.enterTextDetail(title, strings.Join(lines, "\n"))
	return m, nil
}

func (m Model) activityReviewLines(target ReviewTarget) []string {
	switch target.Kind {
	case ReviewFullError:
		return []string{firstNonEmpty(m.Err, m.FullError)}
	case ReviewBatchResult:
		lines := make([]string, 0, len(m.BatchResult.Items)+len(m.BatchResult.Issues))
		for _, item := range m.BatchResult.Items {
			state := "Attempted"
			if item.Changed && m.BatchResult.Changed {
				state = "Changed"
			}
			if item.Issue != nil {
				state = "Issue: " + firstNonEmpty(item.Issue.Message, errorText(item.Issue.Err))
			}
			lines = append(lines, firstNonEmpty(item.Item, "item")+": "+state)
		}
		for _, issue := range m.BatchResult.Issues {
			lines = append(lines, strings.Join(nonEmptyStrings(issue.Item, issue.Path, issue.Message, errorText(issue.Err)), " · "))
		}
		return lines
	case ReviewOperationResult:
		return m.detailLines()
	case ReviewRecoveryResult:
		lines := make([]string, 0, len(m.RecoveryResult.Completed)+len(m.RecoveryResult.Issues))
		for _, id := range m.RecoveryResult.Completed {
			lines = append(lines, "Completed: "+id)
		}
		for _, issue := range m.RecoveryResult.Issues {
			lines = append(lines, strings.Join(nonEmptyStrings(issue.Operation, issue.Path, issue.Message, errorText(issue.Err)), " · "))
		}
		return lines
	case ReviewStatusItem:
		for _, item := range m.Snapshot.Status.Items {
			if statusItemKey(item) == target.Key {
				return []string{item.Message}
			}
		}
	case ReviewUpdateCheck:
		for _, item := range m.UpdateFailures {
			if item.SkillID == target.Key {
				return []string{item.SkillID + ": " + item.Error}
			}
		}
		return append([]string(nil), m.UpdateWarnings...)
	case ReviewInventoryIssue:
		for _, issue := range m.Inventory.Issues {
			if inventoryIssueKey(issue) == target.Key {
				return []string{strings.Join(nonEmptyStrings(issue.Origin, issue.Path, issue.Message), " · ")}
			}
		}
	}
	return nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func inventoryIssueKey(issue app.ScanIssue) string {
	return issue.Origin + "\x00" + issue.Path + "\x00" + issue.Message
}
