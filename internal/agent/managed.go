package agent

import (
	"fmt"
	"os"
	"strings"
)

const (
	managedBegin = "<!-- aikit:managed:begin -->"
	managedEnd   = "<!-- aikit:managed:end -->"
)

// writeManagedSection writes rules into the managed section of a merged file
// (e.g. CLAUDE.md, copilot-instructions.md). Preserves content outside the markers.
func writeManagedSection(path string, content string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	body := string(existing)
	managed := fmt.Sprintf("%s\n%s\n%s", managedBegin, strings.TrimSpace(content), managedEnd)

	beginIdx := strings.Index(body, managedBegin)
	endIdx := strings.Index(body, managedEnd)

	if beginIdx >= 0 && endIdx >= 0 {
		body = body[:beginIdx] + managed + body[endIdx+len(managedEnd):]
	} else if len(body) > 0 {
		body = body + "\n\n" + managed + "\n"
	} else {
		body = managed + "\n"
	}

	return os.WriteFile(path, []byte(body), 0644)
}

// buildManagedContent builds a merged markdown string from multiple rules.
func buildManagedContent(rules []ruleEntry) string {
	var sb strings.Builder
	for i, r := range rules {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("## ")
		sb.WriteString(r.Name)
		sb.WriteString("\n\n")
		if len(r.Globs) > 0 && !r.AlwaysApply {
			sb.WriteString(fmt.Sprintf("_Applies to: %s_\n\n", strings.Join(r.Globs, ", ")))
		}
		sb.WriteString(strings.TrimSpace(r.Content))
	}
	return sb.String()
}

type ruleEntry struct {
	Name        string
	Content     string
	Globs       []string
	AlwaysApply bool
}
