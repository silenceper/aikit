package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
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

func (m *Model) beginTransitionActivity(command tea.Cmd) tea.Cmd {
	if command == nil || (!m.Busy && !m.MutationBusy) {
		return command
	}
	kind := ActivityReading
	if m.MutationBusy {
		kind = ActivityMutating
	} else {
		status := strings.ToLower(m.Status)
		if strings.Contains(status, "remote") || strings.Contains(status, "network") || strings.Contains(status, "update check") || strings.Contains(status, "refreshing status") {
			kind = ActivityNetwork
		}
	}
	label := strings.TrimSpace(strings.TrimSuffix(m.Status, "..."))
	if label == "" {
		label = "Working"
	}
	return m.beginActivity(kind, label, "", command)
}

func (m Model) finishActivityResult(message tea.Msg) (ActivityKind, string, ReviewTarget) {
	kind, review := ActivitySuccess, ReviewTarget{}
	label := firstNonEmpty(strings.TrimSpace(strings.TrimSuffix(m.Status, "...")), "Completed")
	err := activityMessageError(message)
	if err != nil {
		kind, review = ActivityError, ReviewTarget{Kind: ReviewFullError, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
		label = firstNonEmpty(strings.TrimSpace(m.Status), "Operation failed")
	}
	switch msg := message.(type) {
	case snapshotMsg:
		failures := 0
		for _, result := range msg.snapshot.Updates.Results {
			if result.State == updatecheck.StateCheckFailed {
				failures++
			}
		}
		if msg.err == nil && len(msg.snapshot.Updates.Warnings)+failures > 0 {
			kind, review = ActivityWarning, ReviewTarget{Kind: ReviewUpdateCheck, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
		}
	case scanMsg:
		if msg.err == nil && (len(msg.result.Issues) > 0 || len(msg.result.Warnings) > 0 || msg.result.Exit == app.ExitPartial) {
			kind = ActivityWarning
			if len(msg.result.Issues) > 0 {
				review = ReviewTarget{Kind: ReviewInventoryIssue, Key: inventoryIssueKey(msg.result.Issues[0])}
			} else {
				review = ReviewTarget{Kind: ReviewFullError, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
			}
		}
	case updateCheckMsg:
		if msg.err != nil || len(msg.result.Updates.Warnings) > 0 || len(m.UpdateFailures) > 0 {
			kind, review = ActivityWarning, ReviewTarget{Kind: ReviewUpdateCheck, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
			if msg.err != nil {
				kind = ActivityError
			}
		}
	case batchOperationMsg:
		if msg.err != nil || len(msg.result.Issues) > 0 || msg.result.Exit == app.ExitPartial {
			kind, review = ActivityWarning, ReviewTarget{Kind: ReviewBatchResult, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
			if msg.err != nil && !msg.result.Changed {
				kind = ActivityError
			}
		}
	case projectOperationMsg:
		if msg.err != nil || msg.result.Exit == app.ExitPartial || len(msg.result.Plan.Issues)+len(msg.result.Link.Failures)+len(msg.result.Link.Issues) > 0 {
			kind, review = ActivityWarning, ReviewTarget{Kind: ReviewOperationResult, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
			if msg.err != nil {
				kind = ActivityError
			}
		}
	case recoveryOperationMsg:
		if msg.err != nil || len(msg.result.Issues) > 0 {
			kind, review = ActivityWarning, ReviewTarget{Kind: ReviewRecoveryResult, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
			if msg.err != nil {
				kind = ActivityError
			}
		}
	case operationMsg:
		if msg.err == nil && (msg.result.Exit == app.ExitPartial || len(msg.result.Warnings)+len(msg.result.Plan.Issues)+len(msg.result.Link.Failures)+len(msg.result.Link.Issues) > 0) {
			kind, review = ActivityWarning, ReviewTarget{Kind: ReviewOperationResult, Key: fmt.Sprintf("generation:%d", m.activityGeneration)}
		}
	}
	return kind, label, review
}

func activityMessageError(message tea.Msg) error {
	switch msg := message.(type) {
	case snapshotMsg:
		return msg.err
	case scanMsg:
		return msg.err
	case skillDetailMsg:
		return msg.err
	case mutationPreviewMsg:
		return msg.err
	case batchPreviewMsg:
		return msg.err
	case syncPreviewMsg:
		return msg.err
	case addPreviewMsg:
		return msg.err
	case compareMsg:
		return msg.err
	case updateCheckMsg:
		return msg.err
	case configurationMsg:
		return msg.err
	case configurationValidationMsg:
		return msg.err
	case configurationReloadMsg:
		return msg.err
	case projectPreviewMsg:
		return msg.err
	case projectRegistrationPreviewMsg:
		return msg.err
	case projectOperationMsg:
		return msg.err
	case recoveryPreviewMsg:
		return msg.err
	case recoveryOperationMsg:
		return msg.err
	case operationMsg:
		return msg.err
	case batchOperationMsg:
		return msg.err
	default:
		return nil
	}
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
