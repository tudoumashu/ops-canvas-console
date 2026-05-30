package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/basketikun/infinite-canvas/internal/localworkspace"
)

type cliOptions struct {
	Workspace string
	JSON      bool
	ShowPaths bool
	Name      string
	RunID     string
	Follow    bool
	Host      string
	Port      int
	Origins   []string
	Command   []string
}

type successEnvelope struct {
	OK       bool     `json:"ok"`
	Data     any      `json:"data"`
	Warnings []string `json:"warnings"`
}

type doctorEnvelope struct {
	OK       bool                         `json:"ok"`
	Data     *localworkspace.DoctorReport `json:"data"`
	Warnings []string                     `json:"warnings"`
}

type errorEnvelope struct {
	OK    bool                  `json:"ok"`
	Error *localworkspace.Error `json:"error"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runWithContext(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	return runWithContext(context.Background(), args, stdout, stderr)
}

func runWithContext(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if len(opts.Command) == 0 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing command", 1, nil))
	}
	switch opts.Command[0] {
	case "workspace":
		return runWorkspaceCommand(opts, stdout, stderr)
	case "template":
		return runTemplateCommand(opts, stdout, stderr)
	case "run":
		return runRunCommand(ctx, opts, stdout, stderr)
	case "artifact":
		return runArtifactCommand(opts, stdout, stderr)
	case "profile":
		return runProfileCommand(opts, stdout, stderr)
	case "project":
		return runProjectCommand(opts, stdout, stderr)
	case "asset":
		return runAssetCommand(opts, stdout, stderr)
	case "prompt":
		return runPromptCommand(opts, stdout, stderr)
	case "serve":
		return runServeCommand(ctx, opts, stdout, stderr)
	case "executor":
		return runExecutorCommand(ctx, opts, stdout, stderr)
	case "mcp":
		return runMCPCommand(ctx, opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown command: "+opts.Command[0], 1, nil))
	}
}

func runServeCommand(ctx context.Context, opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) != 1 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "serve does not accept subcommands", 1, nil))
	}
	ready := false
	err := localworkspace.Serve(ctx, localworkspace.ServeOptions{
		WorkspacePath:  opts.Workspace,
		Host:           opts.Host,
		Port:           opts.Port,
		AllowedOrigins: opts.Origins,
		Ready: func(runtime localworkspace.ServeRuntimeInfo) error {
			ready = true
			if opts.JSON {
				writeSuccess(stdout, runtime, nil)
				return nil
			}
			fmt.Fprintf(stderr, "opsc serve listening on %s\n", runtime.BaseURL)
			fmt.Fprintf(stderr, "token file: %s\n", runtime.TokenFile)
			fmt.Fprintf(stderr, "launch secret file: %s\n", runtime.LaunchSecretFile)
			return nil
		},
	})
	if err != nil {
		if ready && !opts.JSON {
			fmt.Fprintf(stderr, "opsc serve stopped: %v\n", err)
			return asCLIError(err).ExitCode
		}
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	return 0
}

func runExecutorCommand(ctx context.Context, opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) != 1 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "executor does not accept subcommands", 1, nil))
	}
	result, err := localworkspace.RunExecutorOnce(ctx, localworkspace.ExecutorOptions{
		WorkspacePath: opts.Workspace,
		RunID:         opts.RunID,
	})
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, result, result.Warnings)
	}
	fmt.Fprintf(stdout, "Executor processed %d run(s)\n", result.Processed)
	for _, run := range result.Runs {
		line := fmt.Sprintf("- %s\t%s\texecuted=%d\tskipped=%d\tartifacts=%d", run.RunID, run.Status, run.Executed, run.Skipped, run.ArtifactRefs)
		if run.Error != "" {
			line += "\terror=" + run.Error
		}
		fmt.Fprintln(stdout, line)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", warning)
	}
	return 0
}

func runWorkspaceCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing workspace subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "init":
		return runWorkspaceInit(opts, stdout, stderr)
	case "info":
		return runWorkspaceInfo(opts, stdout, stderr)
	case "doctor":
		return runWorkspaceDoctor(opts, stdout, stderr)
	case "index":
		return runWorkspaceIndexCommand(opts, stdout, stderr)
	case "export":
		return runWorkspaceExportCommand(opts, stdout, stderr)
	case "gc":
		return runWorkspaceGCCommand(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown workspace subcommand: "+opts.Command[1], 1, nil))
	}
}

func runWorkspaceInit(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	result, err := localworkspace.Init(localworkspace.InitOptions{Path: opts.Workspace, Name: opts.Name})
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	info := result.Workspace.Info(opts.ShowPaths)
	data := map[string]any{
		"created":   result.Created,
		"workspace": info,
	}
	if opts.JSON {
		return writeSuccess(stdout, data, result.Warnings)
	}
	if result.Created {
		fmt.Fprintf(stdout, "Initialized workspace %s\n", info.ID)
	} else {
		fmt.Fprintf(stdout, "Workspace already initialized %s\n", info.ID)
	}
	if opts.ShowPaths {
		fmt.Fprintf(stdout, "Path: %s\n", info.Path)
	}
	return 0
}

func runWorkspaceInfo(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	info := workspace.Info(opts.ShowPaths)
	if opts.JSON {
		return writeSuccess(stdout, info, nil)
	}
	fmt.Fprintf(stdout, "Workspace: %s\n", info.ID)
	fmt.Fprintf(stdout, "Name: %s\n", info.Name)
	fmt.Fprintf(stdout, "Schema: %s\n", info.SchemaVersion)
	if opts.ShowPaths {
		fmt.Fprintf(stdout, "Path: %s\n", info.Path)
	}
	return 0
}

func runWorkspaceDoctor(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	report, err := localworkspace.Doctor(localworkspace.DoctorOptions{Path: opts.Workspace, ShowPath: opts.ShowPaths, CheckLock: true})
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	writeDoctorReport(stderr, report)
	if !report.OK {
		if opts.JSON {
			writeDoctorJSON(stdout, report)
		}
		return 2
	}
	if opts.JSON {
		writeDoctorJSON(stdout, report)
	}
	return 0
}

func runWorkspaceIndexCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 3 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing workspace index subcommand", 1, nil))
	}
	switch opts.Command[2] {
	case "rebuild":
		return runWorkspaceIndexRebuild(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown workspace index subcommand: "+opts.Command[2], 1, nil))
	}
}

func runWorkspaceIndexRebuild(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	scan, err := localworkspace.RebuildIndex(context.Background(), *workspace, localworkspace.SQLiteIndexRebuilder{}, localworkspace.ScanOptions{})
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	data := map[string]any{
		"workspaceId": workspace.Document.ID,
		"entries":     len(scan.Entries),
	}
	if opts.JSON {
		return writeSuccess(stdout, data, scan.Warnings)
	}
	fmt.Fprintf(stdout, "Rebuilt index for %s (%d entries)\n", workspace.Document.ID, len(scan.Entries))
	for _, warning := range scan.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", warning)
	}
	return 0
}

func runWorkspaceExportCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 3 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing workspace export subcommand", 1, nil))
	}
	switch opts.Command[2] {
	case "plan":
		return runWorkspaceExportPlan(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown workspace export subcommand: "+opts.Command[2], 1, nil))
	}
}

func runWorkspaceExportPlan(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	plan, err := localworkspace.BuildExportPlan(*workspace, localworkspace.ExportPlanOptions{})
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, plan, plan.Warnings)
	}
	fmt.Fprintf(stdout, "Export plan for %s\n", workspace.Document.ID)
	fmt.Fprintf(stdout, "Include: %d\n", len(plan.IncludePaths))
	fmt.Fprintf(stdout, "Exclude: %d\n", len(plan.ExcludePaths))
	for _, warning := range plan.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", warning)
	}
	return 0
}

func runWorkspaceGCCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 3 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing workspace gc subcommand", 1, nil))
	}
	switch opts.Command[2] {
	case "plan":
		return runWorkspaceGCPlan(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown workspace gc subcommand: "+opts.Command[2], 1, nil))
	}
}

func runWorkspaceGCPlan(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	plan, err := localworkspace.BuildGCPlan(*workspace, localworkspace.GCPlanOptions{})
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, plan, plan.Warnings)
	}
	fmt.Fprintf(stdout, "GC dry-run plan for %s\n", workspace.Document.ID)
	fmt.Fprintf(stdout, "Candidates: %d\n", len(plan.Candidates))
	for _, warning := range plan.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", warning)
	}
	return 0
}

func runTemplateCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing template subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runTemplateList(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown template subcommand: "+opts.Command[1], 1, nil))
	}
}

func runTemplateList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	templates, err := localworkspace.ListTemplateSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"templates": templates}, nil)
	}
	for _, template := range templates {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%d\n", template.ID, template.Title, template.WorkflowType, template.Version)
	}
	return 0
}

func runRunCommand(ctx context.Context, opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing run subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runRunList(opts, stdout, stderr)
	case "status":
		return runRunStatus(opts, stdout, stderr)
	case "events":
		return runRunEvents(ctx, opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown run subcommand: "+opts.Command[1], 1, nil))
	}
}

func runRunList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	runs, err := localworkspace.ListRunSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"runs": runs}, nil)
	}
	for _, run := range runs {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%d\n", run.ID, run.Status, run.TemplateID, run.ArtifactCount)
	}
	return 0
}

func runRunStatus(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 3 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "run status requires a run id", 1, nil))
	}
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	snapshot, err := localworkspace.GetRunStatus(*workspace, opts.Command[2])
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, snapshot, nil)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%d\n", snapshot.Run.ID, snapshot.Run.Status, snapshot.Run.TemplateID, snapshot.Run.ArtifactCount)
	return 0
}

func runRunEvents(ctx context.Context, opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 3 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "run events requires a run id", 1, nil))
	}
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	runID := opts.Command[2]
	emit := func(event localworkspace.RunEventEnvelope) error {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(encoded))
		return err
	}
	if opts.Follow {
		if err := localworkspace.FollowRunEvents(ctx, *workspace, runID, 0, 500*time.Millisecond, emit); err != nil {
			return writeError(stderr, opts.JSON, asCLIError(err))
		}
		return 0
	}
	events, err := localworkspace.ReadRunEvents(*workspace, runID, 0)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	for _, event := range events {
		if err := emit(event); err != nil {
			return writeError(stderr, opts.JSON, asCLIError(err))
		}
	}
	return 0
}

func runArtifactCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing artifact subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runArtifactList(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown artifact subcommand: "+opts.Command[1], 1, nil))
	}
}

func runArtifactList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if strings.TrimSpace(opts.RunID) != "" {
		artifacts, err := localworkspace.ListRunArtifactSummaries(*workspace, opts.RunID)
		if err != nil {
			return writeError(stderr, opts.JSON, asCLIError(err))
		}
		if opts.JSON {
			return writeSuccess(stdout, map[string]any{"runId": opts.RunID, "artifacts": artifacts}, nil)
		}
		for _, item := range artifacts {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%d\n", item.Artifact.ID, item.Artifact.Type, item.Artifact.Title, item.Ref.Role, item.Ref.Order)
		}
		return 0
	}
	artifacts, err := localworkspace.ListArtifactSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"artifacts": artifacts}, nil)
	}
	for _, artifact := range artifacts {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%d\n", artifact.ID, artifact.Type, artifact.Title, artifact.Bytes)
	}
	return 0
}

func runProfileCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing profile subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runProfileList(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown profile subcommand: "+opts.Command[1], 1, nil))
	}
}

func runProfileList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	profiles, err := localworkspace.ListProfileSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"profiles": profiles}, nil)
	}
	for _, profile := range profiles {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%d\n", profile.ID, profile.Name, profile.Mode, profile.ChannelCount)
	}
	return 0
}

func runProjectCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing project subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runProjectList(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown project subcommand: "+opts.Command[1], 1, nil))
	}
}

func runProjectList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	projects, err := localworkspace.ListProjectSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"projects": projects}, nil)
	}
	for _, project := range projects {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%t\n", project.ID, project.Name, project.Kind, project.Adapter, project.HasRootPath)
	}
	return 0
}

func runAssetCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing asset subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runAssetList(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown asset subcommand: "+opts.Command[1], 1, nil))
	}
}

func runAssetList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	assets, err := localworkspace.ListAssetSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"assets": assets}, nil)
	}
	for _, asset := range assets {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", asset.ID, asset.Type, asset.Title, asset.Privacy)
	}
	return 0
}

func runPromptCommand(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) < 2 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "missing prompt subcommand", 1, nil))
	}
	switch opts.Command[1] {
	case "list":
		return runPromptList(opts, stdout, stderr)
	default:
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown prompt subcommand: "+opts.Command[1], 1, nil))
	}
}

func runPromptList(opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	workspace, err := localworkspace.Open(opts.Workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	prompts, err := localworkspace.ListPromptSummaries(*workspace)
	if err != nil {
		return writeError(stderr, opts.JSON, asCLIError(err))
	}
	if opts.JSON {
		return writeSuccess(stdout, map[string]any{"prompts": prompts}, nil)
	}
	for _, prompt := range prompts {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%t\n", prompt.ID, prompt.Title, prompt.Kind, prompt.HasContent)
	}
	return 0
}

func writeDoctorReport(writer io.Writer, report *localworkspace.DoctorReport) {
	if report.OK {
		fmt.Fprintln(writer, "Workspace OK")
	} else {
		fmt.Fprintln(writer, "Workspace has problems")
	}
	for _, check := range report.Checks {
		status := "ok"
		if !check.OK {
			status = check.Severity
		}
		fmt.Fprintf(writer, "- [%s] %s: %s\n", status, check.Name, check.Message)
	}
}

func parseArgs(args []string) (cliOptions, error) {
	opts := cliOptions{Port: -1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--follow":
			opts.Follow = true
		case arg == "--show-paths":
			opts.ShowPaths = true
		case arg == "--workspace":
			i++
			if i >= len(args) {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--workspace requires a value", 1, nil)
			}
			opts.Workspace = args[i]
		case strings.HasPrefix(arg, "--workspace="):
			opts.Workspace = strings.TrimPrefix(arg, "--workspace=")
		case arg == "--name":
			i++
			if i >= len(args) {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--name requires a value", 1, nil)
			}
			opts.Name = args[i]
		case strings.HasPrefix(arg, "--name="):
			opts.Name = strings.TrimPrefix(arg, "--name=")
		case arg == "--host":
			i++
			if i >= len(args) {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--host requires a value", 1, nil)
			}
			opts.Host = args[i]
		case strings.HasPrefix(arg, "--host="):
			opts.Host = strings.TrimPrefix(arg, "--host=")
		case arg == "--port":
			i++
			if i >= len(args) {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--port requires a value", 1, nil)
			}
			port, err := strconv.Atoi(args[i])
			if err != nil || port < 0 || port > 65535 {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--port must be 0-65535", 1, nil)
			}
			opts.Port = port
		case strings.HasPrefix(arg, "--port="):
			port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port="))
			if err != nil || port < 0 || port > 65535 {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--port must be 0-65535", 1, nil)
			}
			opts.Port = port
		case arg == "--origin":
			i++
			if i >= len(args) {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--origin requires a value", 1, nil)
			}
			opts.Origins = append(opts.Origins, args[i])
		case strings.HasPrefix(arg, "--origin="):
			opts.Origins = append(opts.Origins, strings.TrimPrefix(arg, "--origin="))
		case arg == "--run":
			i++
			if i >= len(args) {
				return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "--run requires a value", 1, nil)
			}
			opts.RunID = args[i]
		case strings.HasPrefix(arg, "--run="):
			opts.RunID = strings.TrimPrefix(arg, "--run=")
		case strings.HasPrefix(arg, "-"):
			return opts, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "unknown flag: "+arg, 1, nil)
		default:
			opts.Command = append(opts.Command, arg)
		}
	}
	return opts, nil
}

func writeSuccess(stdout io.Writer, data any, warnings []string) int {
	if warnings == nil {
		warnings = []string{}
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(successEnvelope{OK: true, Data: data, Warnings: warnings})
	return 0
}

func writeDoctorJSON(stdout io.Writer, report *localworkspace.DoctorReport) {
	warnings := report.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(doctorEnvelope{OK: report.OK, Data: report, Warnings: warnings})
}

func writeError(stderr io.Writer, jsonOutput bool, err *localworkspace.Error) int {
	if err == nil {
		err = localworkspace.NewError(localworkspace.ErrorInternal, "unknown error", 5, nil)
	}
	if jsonOutput {
		encoder := json.NewEncoder(stderr)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(errorEnvelope{OK: false, Error: err})
	} else {
		fmt.Fprintln(stderr, err.Message)
	}
	if err.ExitCode == 0 {
		return 1
	}
	return err.ExitCode
}

func asCLIError(err error) *localworkspace.Error {
	if err == nil {
		return nil
	}
	var workspaceErr *localworkspace.Error
	if errors.As(err, &workspaceErr) {
		return workspaceErr
	}
	return localworkspace.WrapError(localworkspace.ErrorInternal, "unexpected error", 5, err)
}
