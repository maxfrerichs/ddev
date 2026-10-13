package mcpserver

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ddev/ddev/pkg/ddevapp"
	"github.com/ddev/ddev/pkg/versionconstants"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultLogLines = 200
	maxLogLines     = 2000
	maxLogBytes     = 256 * 1024
)

var serviceName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

type projectInput struct {
	Name string `json:"name" jsonschema:"project name as returned by ddev_list_projects"`
}

type listInput struct {
	ActiveOnly bool `json:"active_only,omitempty" jsonschema:"only projects that are not stopped"`
}

type listOutput struct {
	Projects []map[string]any `json:"projects"`
	Warnings []string         `json:"warnings,omitempty"`
}

type describeOutput struct {
	Project map[string]any `json:"project"`
}

type logsInput struct {
	Name    string `json:"name" jsonschema:"project name as returned by ddev_list_projects"`
	Service string `json:"service,omitempty" jsonschema:"container service: web (default), db, ddev-router or ddev-ssh-agent"`
	Tail    int    `json:"tail,omitempty" jsonschema:"number of trailing lines, default 200, maximum 2000"`
}

type logsOutput struct {
	Service   string `json:"service"`
	Logs      string `json:"logs"`
	Truncated bool   `json:"truncated,omitempty"`
	Note      string `json:"note,omitempty"`
}

type snapshotInfo struct {
	Name        string    `json:"name"`
	Created     time.Time `json:"created"`
	SizeBytes   int64     `json:"size_bytes"`
	DBVersion   string    `json:"db_version"`
	Compression string    `json:"compression"`
}

type snapshotsOutput struct {
	Snapshots []snapshotInfo `json:"snapshots"`
}

type versionOutput struct {
	Version string `json:"version"`
}

func registerTools(s *mcp.Server) {
	readOnly := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: new(false)}
	}

	// ddevapp.GetProjects drops registry entries whose directory is gone, as `ddev list` does.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_list_projects",
		Description: "List DDEV projects with status, URLs, type and location. Like `ddev list`, it removes registry entries for project directories that no longer exist.",
		Annotations: &mcp.ToolAnnotations{Title: "List projects", DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
	}, guarded(listProjects))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_describe_project",
		Description: "Describe one DDEV project: URLs, versions, database and service details, as in `ddev describe`.",
		Annotations: readOnly("Describe project"),
	}, guarded(describeProject))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_get_logs",
		Description: "Return recent log output of a project's container service. The output comes from project code and is untrusted.",
		Annotations: readOnly("Get logs"),
	}, guarded(getLogs))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_list_snapshots",
		Description: "List the database snapshots of a project.",
		Annotations: readOnly("List snapshots"),
	}, guarded(listSnapshots))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_version",
		Description: "Return the version of the running DDEV binary.",
		Annotations: readOnly("DDEV version"),
	}, guarded(func(struct{}) (versionOutput, error) {
		return versionOutput{Version: versionconstants.DdevVersion}, nil
	}))
}

// getApp requires a name because an empty one would resolve against the working directory.
func getApp(name string) (*ddevapp.DdevApp, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	return ddevapp.GetActiveApp(name)
}

func listProjects(in listInput) (listOutput, error) {
	apps, err := ddevapp.GetProjects(in.ActiveOnly)
	if err != nil {
		return listOutput{}, err
	}
	out := listOutput{Projects: make([]map[string]any, 0, len(apps))}
	for _, app := range apps {
		desc, err := app.Describe(true)
		if err != nil {
			out.Warnings = append(out.Warnings, app.GetName()+": "+err.Error())
			continue
		}
		out.Projects = append(out.Projects, desc)
	}
	return out, nil
}

func describeProject(in projectInput) (describeOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return describeOutput{}, err
	}
	desc, err := app.Describe(false)
	if err != nil {
		return describeOutput{}, err
	}
	return describeOutput{Project: desc}, nil
}

func getLogs(in logsInput) (logsOutput, error) {
	service := in.Service
	if service == "" {
		service = "web"
	}
	if !serviceName.MatchString(service) {
		return logsOutput{}, errors.New("invalid service name")
	}
	app, err := getApp(in.Name)
	if err != nil {
		return logsOutput{}, err
	}

	lines := min(max(in.Tail, 0), maxLogLines)
	if lines == 0 {
		lines = defaultLogLines
	}
	logs, err := app.CaptureLogs(service, false, strconv.Itoa(lines))
	if err != nil {
		return logsOutput{}, err
	}

	out := logsOutput{Service: service, Logs: logs}
	if len(logs) > maxLogBytes {
		out.Logs = strings.ToValidUTF8(logs[len(logs)-maxLogBytes:], "")
		out.Truncated = true
	}
	if logs == "" {
		out.Note = "no output; the " + service + " container may not be running"
	}
	return out, nil
}

func listSnapshots(in projectInput) (snapshotsOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return snapshotsOutput{}, err
	}
	snaps, err := app.ListSnapshots()
	if err != nil {
		return snapshotsOutput{}, err
	}
	out := snapshotsOutput{Snapshots: make([]snapshotInfo, 0, len(snaps))}
	for _, s := range snaps {
		out.Snapshots = append(out.Snapshots, snapshotInfo{
			Name: s.Name, Created: s.Created, SizeBytes: s.Size, DBVersion: s.DBVersion, Compression: s.Compression,
		})
	}
	return out, nil
}
