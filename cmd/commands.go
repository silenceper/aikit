package cmd

import (
	"fmt"
	"strings"

	"github.com/silenceper/aikit/internal/app"
	"github.com/silenceper/aikit/internal/updatecheck"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func newAddCommand(deps Dependencies) *cobra.Command {
	var skills []string
	var sourcePath, refValue, agentName, projectName string
	var force bool
	cmd := &cobra.Command{Use: "add <source-or-path>", Short: "Add skills to the central library", Args: cobra.MaximumNArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return requireOrTUI(cmd, deps, false, "add")
		}
		ref, err := parseRef(refValue)
		if err != nil {
			return err
		}
		if len(skills) == 0 {
			preview, previewErr := deps.Service.PreviewAdd(cmd.Context(), app.AddPreviewRequest{Source: args[0], SourcePath: sourcePath, Ref: ref})
			if previewErr != nil {
				return previewErr
			}
			if len(preview.Candidates) > 1 {
				return fmt.Errorf("source contains multiple skills; use --skill to preserve this source selection")
			}
			if len(preview.Candidates) == 1 {
				selection := preview.Candidates[0].RelativePath
				if selection == "" {
					selection = preview.Candidates[0].Name
				}
				skills = []string{selection}
			}
		}
		result, err := deps.Service.Add(cmd.Context(), app.AddRequest{Source: args[0], Skills: skills, SourcePath: sourcePath, Ref: ref, Force: force, Agent: agentName, Project: projectName})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, fmt.Sprintf("Added %d skill(s)", len(result.Skills))); err != nil {
			return err
		}
		return resultError(result)
	}
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "skill name or path (repeatable)")
	cmd.Flags().StringVar(&sourcePath, "source-path", "", "skill path within a repository")
	cmd.Flags().StringVar(&refValue, "ref", "", "branch:<name>, tag:<name>, or commit:<object-id>")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent scope")
	cmd.Flags().StringVar(&projectName, "project", "", "project scope")
	cmd.Flags().BoolVar(&force, "force", false, "replace an occupied ledger skill")
	return cmd
}

func newListCommand(deps Dependencies) *cobra.Command {
	var offline bool
	var agentName, projectName, presetName string
	cmd := &cobra.Command{Use: "list", Short: "List managed skills", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		snapshot, err := deps.Service.Snapshot(cmd.Context(), app.StatusRequest{Offline: offline})
		if err != nil {
			return err
		}
		skills, err := filterSkills(snapshot.Config, agentName, projectName, presetName)
		if err != nil {
			return err
		}
		if jsonOutput, _ := cmd.Root().PersistentFlags().GetBool("json"); jsonOutput {
			return writeValue(cmd, skills, "")
		}
		for _, skill := range skills {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", skill.ID, skill.Name)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&offline, "offline", false, "do not access remotes")
	cmd.Flags().StringVar(&agentName, "agent", "", "filter by agent")
	cmd.Flags().StringVar(&projectName, "project", "", "filter by project")
	cmd.Flags().StringVar(&presetName, "preset", "", "filter by preset")
	return cmd
}

func newRemoveCommand(deps Dependencies) *cobra.Command {
	var force, yes bool
	cmd := &cobra.Command{Use: "remove <id>", Short: "Remove a skill from the library", Args: cobra.MaximumNArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return requireOrTUI(cmd, deps, false, "library")
		}
		result, err := deps.Service.Remove(cmd.Context(), app.RemoveRequest{SkillID: args[0], Force: force})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, "Removed "+args[0]); err != nil {
			return err
		}
		return resultError(result)
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove references before removal")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm destructive action")
	return cmd
}

func newBindingCommand(deps Dependencies, enable bool) *cobra.Command {
	name := "disable"
	if enable {
		name = "enable"
	}
	var preset, agentName, projectName string
	cmd := &cobra.Command{Use: name + " [id]", Short: strings.Title(name) + " a skill or preset", Args: cobra.MaximumNArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		complete := (len(args) == 1 || preset != "") && (agentName != "" || projectName != "")
		if complete && projectName == "" && agentName != "" {
			// Explicit --agent always means the global scope; cwd must not narrow it.
		} else if (len(args) == 1 || preset != "") && agentName == "" && projectName == "" {
			inferred, err := inferProject(cmd.Context(), deps)
			if err != nil {
				return err
			}
			projectName = inferred
			complete = projectName != ""
		}
		if !complete {
			view := "agents"
			if projectName != "" {
				view = "projects"
			}
			return requireOrTUI(cmd, deps, false, view)
		}
		request := app.BindingRequest{Preset: preset, Agent: agentName, Project: projectName}
		if len(args) == 1 {
			request.SkillID = args[0]
		}
		var result app.Result
		var err error
		if enable {
			result, err = deps.Service.Enable(cmd.Context(), request)
		} else {
			result, err = deps.Service.Disable(cmd.Context(), request)
		}
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, name+"d binding"); err != nil {
			return err
		}
		return resultError(result)
	}
	cmd.Flags().StringVar(&preset, "preset", "", "preset name")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent scope")
	cmd.Flags().StringVar(&projectName, "project", "", "project scope")
	return cmd
}

func newSyncCommand(deps Dependencies) *cobra.Command {
	var agentName, projectName string
	var dryRun bool
	cmd := &cobra.Command{Use: "sync", Short: "Reconcile configured skill links", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := deps.Service.Sync(cmd.Context(), app.SyncRequest{Agent: agentName, Project: projectName, DryRun: dryRun})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, "Sync complete"); err != nil {
			return err
		}
		return resultError(result)
	}}
	cmd.Flags().StringVar(&agentName, "agent", "", "agent scope")
	cmd.Flags().StringVar(&projectName, "project", "", "project scope")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show changes without writing")
	return cmd
}

func newStatusCommand(deps Dependencies) *cobra.Command {
	var offline, refresh bool
	cmd := &cobra.Command{Use: "status", Short: "Show ledger and filesystem status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		snapshot, err := deps.Service.Snapshot(cmd.Context(), app.StatusRequest{Offline: offline, ForceRefresh: refresh})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, snapshot, fmt.Sprintf("%d skills, %d issue(s)", snapshot.Status.LibrarySkills, len(snapshot.Status.Items))); err != nil {
			return err
		}
		if err := writeStatusDetails(cmd, snapshot); err != nil {
			return err
		}
		return resultError(app.Result{Exit: snapshot.Exit})
	}}
	cmd.Flags().BoolVar(&offline, "offline", false, "do not access remotes")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh update data")
	return cmd
}

func newUpdateCommand(deps Dependencies) *cobra.Command {
	var skills []string
	var refValue string
	var check, offline, refresh, yes, force bool
	cmd := &cobra.Command{Use: "update [id]", Short: "Check for or apply skill updates", Args: cobra.MaximumNArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			skills = append(skills, args[0])
		}
		ref, err := parseRef(refValue)
		if err != nil {
			return err
		}
		if !check && !yes {
			if deps.IsTTY() && len(skills) == 0 && ref == nil && !force {
				return deps.LaunchTUI(cmd.Context(), "updates")
			}
			if deps.IsTTY() {
				return fmt.Errorf("this update selection cannot be transferred to the TUI; rerun with --yes")
			}
		}
		checkRequest := app.UpdateRequest{SkillIDs: skills, Ref: ref, Force: force, CheckOnly: true, Offline: offline, Refresh: true}
		checked, err := deps.Service.Update(cmd.Context(), checkRequest)
		if err != nil {
			return err
		}
		if check || !yes || offline {
			if err := writeValue(cmd, checked, "Update check complete"); err != nil {
				return err
			}
			if err := writeUpdateReport(cmd, checked.Updates); err != nil {
				return err
			}
			return resultError(checked)
		}
		snapshot, err := deps.Service.Snapshot(cmd.Context(), app.StatusRequest{Offline: true})
		if err != nil {
			return err
		}
		expected := confirmationTokens(snapshot.Config, checked.Updates, ref != nil)
		result, err := deps.Service.Update(cmd.Context(), app.UpdateRequest{SkillIDs: skills, Ref: ref, Expected: expected, Force: force, Offline: false, Refresh: true, Confirmed: true})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, "Update check complete"); err != nil {
			return err
		}
		if err := writeUpdateReport(cmd, result.Updates); err != nil {
			return err
		}
		return resultError(result)
	}
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "skill id (repeatable)")
	cmd.Flags().StringVar(&refValue, "ref", "", "new structured ref")
	cmd.Flags().BoolVar(&check, "check", false, "check only")
	cmd.Flags().BoolVar(&offline, "offline", false, "do not access remotes")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "ignore update cache")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply without prompting")
	cmd.Flags().BoolVar(&force, "force", false, "allow explicit ref change")
	return cmd
}

func confirmationTokens(cfg config.Config, report updatecheck.CheckReport, explicitRef bool) map[string]app.ExpectedUpdate {
	remote := make(map[string]string, len(report.Results))
	if !explicitRef {
		for _, item := range report.Results {
			remote[item.SkillID] = item.Remote
		}
	}
	expected := make(map[string]app.ExpectedUpdate, len(cfg.Library.Skills))
	for _, skill := range cfg.Library.Skills {
		if skill.Source == "" || skill.Ref == nil {
			continue
		}
		expected[skill.ID] = app.ExpectedUpdate{Ref: skill.Ref, Resolved: skill.Resolved, Remote: remote[skill.ID]}
	}
	return expected
}

func filterSkills(cfg config.Config, agentName, projectName, presetName string) ([]config.Skill, error) {
	var selected map[string]struct{}
	if projectName != "" {
		var project *config.Project
		for index := range cfg.Projects {
			if cfg.Projects[index].Name == projectName {
				project = &cfg.Projects[index]
				break
			}
		}
		if project == nil {
			return nil, fmt.Errorf("project %q not found", projectName)
		}
		selected = make(map[string]struct{})
		if err := addBindingIDs(selected, cfg, project.Binding); err != nil {
			return nil, err
		}
		if agentName != "" {
			if err := addBindingIDs(selected, cfg, project.AgentBindings[agentName]); err != nil {
				return nil, err
			}
		} else {
			for _, binding := range project.AgentBindings {
				if err := addBindingIDs(selected, cfg, binding); err != nil {
					return nil, err
				}
			}
		}
	} else if agentName != "" {
		selected = make(map[string]struct{})
		if err := addBindingIDs(selected, cfg, cfg.Agents[agentName]); err != nil {
			return nil, err
		}
	}
	if presetName != "" {
		presetIDs, err := namedPresetIDs(cfg, presetName)
		if err != nil {
			return nil, err
		}
		if selected == nil {
			selected = presetIDs
		} else {
			for id := range selected {
				if _, ok := presetIDs[id]; !ok {
					delete(selected, id)
				}
			}
		}
	}
	if selected == nil {
		return append([]config.Skill(nil), cfg.Library.Skills...), nil
	}
	result := make([]config.Skill, 0, len(selected))
	for _, skill := range cfg.Library.Skills {
		if _, ok := selected[skill.ID]; ok {
			result = append(result, skill)
		}
	}
	return result, nil
}

func addBindingIDs(ids map[string]struct{}, cfg config.Config, binding config.Binding) error {
	for _, id := range binding.Skills {
		ids[id] = struct{}{}
	}
	for _, name := range binding.Presets {
		presetIDs, err := namedPresetIDs(cfg, name)
		if err != nil {
			return err
		}
		for id := range presetIDs {
			ids[id] = struct{}{}
		}
	}
	return nil
}

func namedPresetIDs(cfg config.Config, name string) (map[string]struct{}, error) {
	for _, preset := range cfg.Presets {
		if preset.Name == name {
			ids := make(map[string]struct{}, len(preset.Skills))
			for _, id := range preset.Skills {
				ids[id] = struct{}{}
			}
			return ids, nil
		}
	}
	return nil, fmt.Errorf("preset %q not found", name)
}

func writeStatusDetails(cmd *cobra.Command, snapshot app.Snapshot) error {
	if jsonOutput(cmd) {
		return nil
	}
	for _, item := range snapshot.Status.Items {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", item.Kind, item.SkillID, item.Path, item.Message); err != nil {
			return err
		}
	}
	for _, item := range snapshot.Status.Warnings {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning:%s\t%s\t%s\t%s\n", item.Kind, item.SkillID, item.Path, item.Message); err != nil {
			return err
		}
	}
	return writeUpdateReport(cmd, snapshot.Updates)
}

func writeUpdateReport(cmd *cobra.Command, report updatecheck.CheckReport) error {
	if jsonOutput(cmd) {
		return nil
	}
	for _, item := range report.Results {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s -> %s", item.State, item.SkillID, shortObjectID(item.Current), shortObjectID(item.Remote)); err != nil {
			return err
		}
		if item.Error != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\t%s", item.Error); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
			return err
		}
	}
	for _, warning := range report.Warnings {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning\t%s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func shortObjectID(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
