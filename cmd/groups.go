package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/silenceper/aikit/internal/app"
	"github.com/spf13/cobra"
)

func newPresetCommand(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "preset", Short: "Manage presets", RunE: func(cmd *cobra.Command, _ []string) error { return requireOrTUI(cmd, deps, false, "presets") }}
	var skills []string
	create := &cobra.Command{Use: "create <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Service.PutPreset(cmd.Context(), app.PresetRequest{Name: args[0], Skills: skills, Create: true})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, "Preset created"); err != nil {
			return err
		}
		return resultError(result)
	}}
	create.Flags().StringSliceVar(&skills, "skill", nil, "skill id (repeatable)")
	var addSkills []string
	add := &cobra.Command{Use: "add <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(addSkills) == 0 {
			return requireOrTUI(cmd, deps, false, "presets")
		}
		result, err := deps.Service.PutPreset(cmd.Context(), app.PresetRequest{Name: args[0], Skills: addSkills})
		if err != nil {
			return err
		}
		return resultError(result)
	}}
	add.Flags().StringSliceVar(&addSkills, "skill", nil, "skill id (repeatable)")
	var removeSkills []string
	var force bool
	remove := &cobra.Command{Use: "remove <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(removeSkills) == 0 {
			result, err := deps.Service.RemovePreset(cmd.Context(), app.PresetRemoveRequest{Name: args[0], Force: force})
			if err != nil {
				return err
			}
			return resultError(result)
		}
		result, err := deps.Service.PutPreset(cmd.Context(), app.PresetRequest{Name: args[0], Skills: removeSkills, Remove: true})
		if err != nil {
			return err
		}
		return resultError(result)
	}}
	remove.Flags().StringSliceVar(&removeSkills, "skill", nil, "remove skill id (repeatable)")
	remove.Flags().BoolVar(&force, "force", false, "remove referenced preset")
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		snapshot, err := deps.Service.Snapshot(cmd.Context(), app.StatusRequest{Offline: true})
		if err != nil {
			return err
		}
		return writeValue(cmd, snapshot.Config.Presets, fmt.Sprintf("%d preset(s)", len(snapshot.Config.Presets)))
	}}
	root.AddCommand(create, add, remove, list)
	return root
}

func newProjectCommand(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "project", Short: "Manage projects", RunE: func(cmd *cobra.Command, _ []string) error { return requireOrTUI(cmd, deps, false, "projects") }}
	var name string
	var agents []string
	add := &cobra.Command{Use: "add [path]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) == 1 {
			path = args[0]
		}
		absolute, err := absoluteCWD(path)
		if err != nil {
			return err
		}
		if name == "" {
			name = filepath.Base(filepath.Clean(absolute))
		}
		if len(agents) == 0 {
			return requireOrTUI(cmd, deps, false, "projects")
		}
		result, err := deps.Service.EditProject(cmd.Context(), app.ProjectEditRequest{Name: name, Path: absolute, AddAgents: agents, Confirmed: true})
		if err != nil {
			return err
		}
		return resultError(result)
	}}
	add.Flags().StringVar(&name, "name", "", "project name")
	add.Flags().StringSliceVar(&agents, "agent", nil, "project agent (repeatable)")
	var newName, path string
	var addAgents, removeAgents []string
	var yes bool
	edit := &cobra.Command{Use: "edit <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if path != "" && !yes {
			return fmt.Errorf("project path changes require --yes; this command context cannot be transferred to the TUI")
		}
		result, err := deps.Service.EditProject(cmd.Context(), app.ProjectEditRequest{Project: args[0], Name: newName, Path: path, AddAgents: addAgents, RemoveAgents: removeAgents, Confirmed: yes})
		if err != nil {
			return err
		}
		return resultError(result)
	}}
	edit.Flags().StringVar(&newName, "name", "", "new project name")
	edit.Flags().StringVar(&path, "path", "", "new project path")
	edit.Flags().StringSliceVar(&addAgents, "add-agent", nil, "add agent")
	edit.Flags().StringSliceVar(&removeAgents, "remove-agent", nil, "remove agent")
	edit.Flags().BoolVar(&yes, "yes", false, "confirm path rebind")
	var removeYes bool
	remove := &cobra.Command{Use: "remove <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !removeYes && !deps.IsTTY() {
			return fmt.Errorf("project remove requires --yes outside a TTY")
		}
		result, err := deps.Service.RemoveProject(cmd.Context(), app.ProjectRemoveRequest{Project: args[0], Confirmed: removeYes})
		if err != nil {
			return err
		}
		return resultError(result)
	}}
	remove.Flags().BoolVar(&removeYes, "yes", false, "confirm removal")
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		snapshot, err := deps.Service.Snapshot(cmd.Context(), app.StatusRequest{Offline: true})
		if err != nil {
			return err
		}
		return writeValue(cmd, snapshot.Config.Projects, fmt.Sprintf("%d project(s)", len(snapshot.Config.Projects)))
	}}
	root.AddCommand(add, edit, remove, list)
	return root
}

func newScanCommand(deps Dependencies) *cobra.Command {
	var agentName, projectName string
	var skills []string
	var all, adopt bool
	cmd := &cobra.Command{Use: "scan", Short: "Discover unmanaged local skills", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.Migration == nil {
			return fmt.Errorf("scan service is unavailable")
		}
		if projectName == "" {
			inferred, err := inferProject(cmd.Context(), deps)
			if err != nil {
				return err
			}
			projectName = inferred
		}
		if adopt && len(skills) == 0 && !all && !deps.IsTTY() {
			return fmt.Errorf("non-TTY --adopt requires --skill or --all")
		}
		if adopt && len(skills) == 0 && !all {
			return deps.LaunchTUI(cmd.Context(), "scan")
		}
		result, err := deps.Migration.Scan(cmd.Context(), app.ScanRequest{Agent: agentName, Project: projectName, Skills: skills, All: all, Adopt: adopt})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, fmt.Sprintf("Scanned %d item(s)", len(result.Items))); err != nil {
			return err
		}
		if err := writeScanIssues(cmd, result); err != nil {
			return err
		}
		return resultError(app.Result{Exit: result.Exit})
	}}
	cmd.Flags().StringVar(&agentName, "agent", "", "agent scope")
	cmd.Flags().StringVar(&projectName, "project", "", "project scope")
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "discovered skill name or id")
	cmd.Flags().BoolVar(&all, "all", false, "select all discovered skills")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "replace authorized source locations with managed links")
	return cmd
}

func newMigrateCommand(deps Dependencies) *cobra.Command {
	var projects []string
	var adopt, dryRun bool
	cmd := &cobra.Command{Use: "migrate", Short: "Migrate legacy aikit data", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.Migration == nil {
			return fmt.Errorf("migration service is unavailable")
		}
		for index, project := range projects {
			absolute, err := absoluteCWD(project)
			if err != nil {
				return err
			}
			projects[index] = absolute
		}
		result, err := deps.Migration.Migrate(cmd.Context(), app.MigrateRequest{ProjectPaths: projects, Adopt: adopt, DryRun: dryRun})
		if err != nil {
			return err
		}
		if err := writeValue(cmd, result, fmt.Sprintf("Imported %d, pending adopt %d, skipped %d, failed %d", result.Imported, result.PendingAdopt, result.Skipped, result.Failed)); err != nil {
			return err
		}
		if err := writeTextWarnings(cmd, result.Warnings); err != nil {
			return err
		}
		return resultError(app.Result{Exit: result.Exit})
	}}
	cmd.Flags().StringSliceVar(&projects, "project", nil, "legacy project path (repeatable)")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "adopt legacy skill directories")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show migration without writing")
	return cmd
}

func writeScanIssues(cmd *cobra.Command, result app.ScanResult) error {
	if jsonOutput(cmd) {
		return nil
	}
	for _, item := range result.Items {
		if item.Error == "" {
			continue
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", item.Origin, item.Skill.ID, item.Error); err != nil {
			return err
		}
	}
	return writeTextWarnings(cmd, result.Warnings)
}

func writeTextWarnings(cmd *cobra.Command, warnings []string) error {
	if jsonOutput(cmd) {
		return nil
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), warning); err != nil {
			return err
		}
	}
	return nil
}

func jsonOutput(cmd *cobra.Command) bool {
	value, _ := cmd.Flags().GetBool("json")
	if !value {
		value, _ = cmd.Root().PersistentFlags().GetBool("json")
	}
	return value
}
