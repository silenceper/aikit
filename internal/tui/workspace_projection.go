package tui

import (
	"fmt"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	domainscope "github.com/silenceper/aikit/internal/scope"
	"github.com/silenceper/aikit/pkg/config"
)

func globalWorkspaceRows(cfg config.Config) []row {
	agents := agent.Names()
	rows := make([]row, 0, len(cfg.Library.Skills))
	for _, skill := range cfg.Library.Skills {
		enabled := make([]string, 0, len(agents))
		for _, agentName := range agents {
			view := domainscope.Global(&cfg, agentName)
			for _, effective := range view.Skills {
				if effective.ID == skill.ID {
					enabled = append(enabled, agentName)
					break
				}
			}
		}
		detail := "Not enabled"
		if len(enabled) > 0 {
			detail = "Enabled: " + strings.Join(enabled, ", ")
		}
		rows = append(rows, row{
			Key:      "workspace-global:" + skill.ID,
			ID:       skill.ID,
			Name:     skill.Name,
			State:    fmt.Sprintf("%d/%d agents", len(enabled), len(agents)),
			Detail:   detail,
			Enabled:  len(enabled) > 0,
			Severity: enabledSeverity(len(enabled) > 0),
		})
	}
	return rows
}
