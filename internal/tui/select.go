package tui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/silenceper/aikit/internal/skill"
)

// SelectSkills shows a multi-select prompt and returns the selected skill names.
func SelectSkills(skills []skill.Info) ([]string, error) {
	if len(skills) == 0 {
		return nil, fmt.Errorf("no skills found")
	}

	var options []huh.Option[string]
	for _, s := range skills {
		label := s.Name
		if s.Desc != "" {
			desc := s.Desc
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
			label += " — " + desc
		}
		options = append(options, huh.NewOption(label, s.Name))
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select skills to add").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}

// AgentOption represents an agent with its detected status.
type AgentOption struct {
	Name     string
	Detected bool
}

// SelectAgents shows a multi-select prompt for choosing target agents.
// Detected agents are pre-selected.
func SelectAgents(agents []AgentOption) ([]string, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available")
	}

	var options []huh.Option[string]
	var preselected []string
	for _, a := range agents {
		label := a.Name
		if a.Detected {
			label += " (detected)"
			preselected = append(preselected, a.Name)
		}
		options = append(options, huh.NewOption(label, a.Name))
	}

	selected := preselected
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select target IDEs/Agents to sync").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}

// CatalogItem represents a catalog entry for interactive selection.
type CatalogItem struct {
	Kind      string // "skill", "rule", "mcp", "command"
	Name      string
	Source    string
	Desc      string
	Group     string
	InCatalog bool
}

// SelectCatalogItems shows a multi-select prompt for choosing assets from the catalog.
// Items with InCatalog=true are pre-selected and labeled accordingly.
func SelectCatalogItems(items []CatalogItem) ([]CatalogItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("catalog is empty; run 'aikit catalog add <source>' first")
	}

	var options []huh.Option[int]
	var preselected []int
	for i, item := range items {
		label := fmt.Sprintf("[%s] %s", item.Kind, item.Name)
		if item.Group != "" {
			label += fmt.Sprintf(" (%s)", item.Group)
		}
		if item.Desc != "" {
			desc := item.Desc
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
			label += " — " + desc
		}
		if item.InCatalog {
			label += " (in catalog)"
			preselected = append(preselected, i)
		}
		options = append(options, huh.NewOption(label, i))
	}

	selected := preselected
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select assets to add to the project").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}

	var out []CatalogItem
	for _, idx := range selected {
		out = append(out, items[idx])
	}
	return out, nil
}

// SelectCatalogItemsToRemove shows a multi-select prompt for choosing assets to remove.
func SelectCatalogItemsToRemove(items []CatalogItem) ([]CatalogItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no assets to remove")
	}

	var options []huh.Option[int]
	for i, item := range items {
		label := fmt.Sprintf("[%s] %s", item.Kind, item.Name)
		if item.Group != "" {
			label += fmt.Sprintf(" (%s)", item.Group)
		}
		if item.Source != "" {
			label += fmt.Sprintf(" (source: %s)", item.Source)
		}
		options = append(options, huh.NewOption(label, i))
	}

	var selected []int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Select assets to remove").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}

	var out []CatalogItem
	for _, idx := range selected {
		out = append(out, items[idx])
	}
	return out, nil
}

// SelectSyncStrategy prompts the user to choose a catalog sync strategy
// when local and remote content differ.
func SelectSyncStrategy() (string, error) {
	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Remote repository already has content that differs from local.\nHow would you like to sync?").
				Options(
					huh.NewOption("Merge        — merge local and remote (conflicts need manual resolution)", "merge"),
					huh.NewOption("Local wins   — overwrite remote with local content", "local"),
					huh.NewOption("Remote wins  — overwrite local with remote content", "remote"),
					huh.NewOption("Cancel       — abort sync", "cancel"),
				).
				Value(&choice),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return choice, nil
}

// InputGroup prompts the user for a group name.
func InputGroup(defaultValue string) (string, error) {
	var group string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Group name (leave empty for 'Ungrouped')").
				Value(&group).
				Placeholder(defaultValue),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	if group == "" {
		group = defaultValue
	}
	return group, nil
}
