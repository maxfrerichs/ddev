package mcpserver

import (
	"errors"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var snapshotName = regexp.MustCompile(`^[\w.-]+$`)

type snapshotInput struct {
	Name         string `json:"name" jsonschema:"project name as returned by ddev_list_projects"`
	SnapshotName string `json:"snapshot_name,omitempty" jsonschema:"snapshot name; letters, digits, underscore, dot and dash. Defaults to the project name plus a timestamp when creating"`
}

type lifecycleOutput struct {
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	PrimaryURL  string `json:"primary_url,omitempty"`
}

type snapshotCreateOutput struct {
	SnapshotName string `json:"snapshot_name"`
}

type snapshotRestoreOutput struct {
	Restored string `json:"restored"`
}

func registerWriteTools(s *mcp.Server) {
	lifecycle := func(title string, idempotent bool) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, DestructiveHint: new(false), IdempotentHint: idempotent, OpenWorldHint: new(false)}
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_start",
		Description: "Start a project, as `ddev start`. Can take minutes and may pull images. Runs the project's configured hooks.",
		Annotations: lifecycle("Start project", true),
	}, guarded(startProject))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_stop",
		Description: "Stop a project's containers, as `ddev stop`. The database and project files are kept.",
		Annotations: lifecycle("Stop project", true),
	}, guarded(stopProject))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_restart",
		Description: "Restart a project, as `ddev restart`. Can take minutes. Runs the project's configured hooks.",
		Annotations: lifecycle("Restart project", true),
	}, guarded(restartProject))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_snapshot_create",
		Description: "Write a database snapshot of a running project, as `ddev snapshot`. Useful before risky changes.",
		Annotations: lifecycle("Create snapshot", false),
	}, guarded(createSnapshot))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_snapshot_restore",
		Description: "Replace a project's database with a snapshot from ddev_list_snapshots, as `ddev snapshot restore`. Current database contents are lost unless a snapshot of them exists.",
		Annotations: &mcp.ToolAnnotations{Title: "Restore snapshot", DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)},
	}, guarded(restoreSnapshot))
}

func startProject(in projectInput) (lifecycleOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return lifecycleOutput{}, err
	}
	if err := app.Start(); err != nil {
		return lifecycleOutput{}, errors.New("start failed, the project may be partly started: " + err.Error())
	}
	status, desc := app.SiteStatus()
	return lifecycleOutput{Status: status, Description: desc, PrimaryURL: app.GetPrimaryURL()}, nil
}

func stopProject(in projectInput) (lifecycleOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return lifecycleOutput{}, err
	}
	if err := app.Stop(false, false); err != nil {
		return lifecycleOutput{}, err
	}
	status, desc := app.SiteStatus()
	return lifecycleOutput{Status: status, Description: desc}, nil
}

func restartProject(in projectInput) (lifecycleOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return lifecycleOutput{}, err
	}
	if err := app.Restart(); err != nil {
		return lifecycleOutput{}, errors.New("restart failed, the project may be partly started: " + err.Error())
	}
	status, desc := app.SiteStatus()
	return lifecycleOutput{Status: status, Description: desc, PrimaryURL: app.GetPrimaryURL()}, nil
}

func createSnapshot(in snapshotInput) (snapshotCreateOutput, error) {
	if in.SnapshotName != "" && !snapshotName.MatchString(in.SnapshotName) {
		return snapshotCreateOutput{}, errors.New("invalid snapshot_name")
	}
	app, err := getApp(in.Name)
	if err != nil {
		return snapshotCreateOutput{}, err
	}
	name, err := app.Snapshot(in.SnapshotName, false)
	if err != nil {
		return snapshotCreateOutput{}, err
	}
	return snapshotCreateOutput{SnapshotName: name}, nil
}

func restoreSnapshot(in snapshotInput) (snapshotRestoreOutput, error) {
	if !snapshotName.MatchString(in.SnapshotName) {
		return snapshotRestoreOutput{}, errors.New("snapshot_name is required and may contain only letters, digits, underscore, dot and dash")
	}
	app, err := getApp(in.Name)
	if err != nil {
		return snapshotRestoreOutput{}, err
	}
	if err := app.RestoreSnapshot(in.SnapshotName, false); err != nil {
		return snapshotRestoreOutput{}, err
	}
	return snapshotRestoreOutput{Restored: in.SnapshotName}, nil
}
