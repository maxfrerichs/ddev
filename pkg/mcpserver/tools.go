package mcpserver

import (
	"errors"
	"regexp"
	"slices"
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

type configOutput struct {
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	AppRoot             string   `json:"approot"`
	Docroot             string   `json:"docroot"`
	PHPVersion          string   `json:"php_version"`
	WebserverType       string   `json:"webserver_type"`
	DatabaseType        string   `json:"database_type"`
	DatabaseVersion     string   `json:"database_version"`
	XdebugInConfig      bool     `json:"xdebug_enabled_in_config"`
	ComposerVersion     string   `json:"composer_version"`
	Timezone            string   `json:"timezone,omitempty"`
	PerformanceMode     string   `json:"performance_mode,omitempty"`
	AdditionalHostnames []string `json:"additional_hostnames,omitempty"`
	OmitContainers      []string `json:"omit_containers,omitempty"`
	HookNames           []string `json:"hook_names,omitempty"`
}

type addonInfo struct {
	Name        string `json:"name"`
	Repository  string `json:"repository"`
	Version     string `json:"version"`
	InstallDate string `json:"install_date"`
}

type addonsOutput struct {
	Addons []addonInfo `json:"addons"`
}

type statusOutput struct {
	Status      string `json:"status"`
	Description string `json:"description"`
	PrimaryURL  string `json:"primary_url"`
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
		Name:        "ddev_get_config",
		Description: "Return a project's configuration from its config files: type, docroot, PHP, webserver, database, the Xdebug startup setting and hook names. Hook commands are not included. Xdebug may have been toggled at runtime since; see ddev_xdebug.",
		Annotations: readOnly("Get config"),
	}, guarded(getConfig))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_list_addons",
		Description: "List the add-ons installed in a project.",
		Annotations: readOnly("List add-ons"),
	}, guarded(listAddons))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_get_status",
		Description: "Return a project's current status (running, stopped, unhealthy and so on) and its primary URL, without starting it.",
		Annotations: readOnly("Get status"),
	}, guarded(getStatus))

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

func getConfig(in projectInput) (configOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return configOutput{}, err
	}
	out := configOutput{
		Name:                app.Name,
		Type:                app.Type,
		AppRoot:             app.AppRoot,
		Docroot:             app.Docroot,
		PHPVersion:          app.PHPVersion,
		WebserverType:       app.WebserverType,
		DatabaseType:        app.Database.Type,
		DatabaseVersion:     app.Database.Version,
		XdebugInConfig:      app.XdebugEnabled,
		ComposerVersion:     app.ComposerVersion,
		Timezone:            app.Timezone,
		PerformanceMode:     string(app.PerformanceMode),
		AdditionalHostnames: app.AdditionalHostnames,
		OmitContainers:      app.OmitContainers,
	}
	for name := range app.Hooks {
		out.HookNames = append(out.HookNames, name)
	}
	slices.Sort(out.HookNames)
	return out, nil
}

func listAddons(in projectInput) (addonsOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return addonsOutput{}, err
	}
	manifests := ddevapp.GetInstalledAddons(app)
	out := addonsOutput{Addons: make([]addonInfo, 0, len(manifests))}
	for _, m := range manifests {
		out.Addons = append(out.Addons, addonInfo{Name: m.Name, Repository: m.Repository, Version: m.Version, InstallDate: m.InstallDate})
	}
	return out, nil
}

func getStatus(in projectInput) (statusOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return statusOutput{}, err
	}
	status, desc := app.SiteStatus()
	return statusOutput{Status: status, Description: desc, PrimaryURL: app.GetPrimaryURL()}, nil
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
