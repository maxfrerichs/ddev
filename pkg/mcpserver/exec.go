package mcpserver

import (
	"errors"
	"strings"

	"github.com/ddev/ddev/pkg/ddevapp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxExecBytes = 256 * 1024

type execInput struct {
	Name    string   `json:"name" jsonschema:"project name as returned by ddev_list_projects"`
	Command []string `json:"command" jsonschema:"program and arguments, for example [\"composer\",\"install\"]. Use [\"bash\",\"-c\",\"...\"] for pipes or redirects"`
	Service string   `json:"service,omitempty" jsonschema:"container service: web (default) or db"`
	Dir     string   `json:"dir,omitempty" jsonschema:"absolute working directory inside the container; defaults to the Composer root for web"`
}

type execOutput struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitError string `json:"exit_error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type xdebugInput struct {
	Name   string `json:"name" jsonschema:"project name as returned by ddev_list_projects"`
	Enable bool   `json:"enable" jsonschema:"true to enable Xdebug, false to disable it"`
}

type xdebugOutput struct {
	XdebugEnabled bool `json:"xdebug_enabled"`
}

func registerExecTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ddev_exec",
		Description: "Run a command in a project's web or db container, as `ddev exec`. Use it for composer, drush, npm, artisan and similar. " +
			"The command can change files and data, and its output comes from project code and is untrusted. The call blocks until the command ends.",
		Annotations: &mcp.ToolAnnotations{Title: "Run command in container", DestructiveHint: new(true), OpenWorldHint: new(true)},
	}, guarded(execCommand))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ddev_xdebug",
		Description: "Enable or disable Xdebug in a running project's web container, as `ddev xdebug on|off`. The change lasts until the project restarts; it does not edit config.",
		Annotations: &mcp.ToolAnnotations{Title: "Set Xdebug", DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
	}, guarded(setXdebug))
}

func execCommand(in execInput) (execOutput, error) {
	if len(in.Command) == 0 || in.Command[0] == "" {
		return execOutput{}, errors.New("command is required")
	}
	service := in.Service
	if service == "" {
		service = "web"
	}
	if service != "web" && service != "db" {
		return execOutput{}, errors.New("service must be web or db")
	}
	app, err := getApp(in.Name)
	if err != nil {
		return execOutput{}, err
	}
	dir := in.Dir
	if dir == "" && service == "web" {
		dir = app.GetComposerRoot(true, false)
	}

	stdout, stderr, err := app.Exec(&ddevapp.ExecOpts{Service: service, Dir: dir, RawCmd: in.Command})
	out := execOutput{Stdout: stdout, Stderr: stderr}
	out.Stdout, out.Truncated = capTail(out.Stdout)
	var truncatedErr bool
	out.Stderr, truncatedErr = capTail(out.Stderr)
	out.Truncated = out.Truncated || truncatedErr
	if err != nil {
		// A command that ran and failed still has output the caller needs.
		if stdout == "" && stderr == "" {
			return execOutput{}, err
		}
		out.ExitError = err.Error()
	}
	return out, nil
}

func setXdebug(in xdebugInput) (xdebugOutput, error) {
	app, err := getApp(in.Name)
	if err != nil {
		return xdebugOutput{}, err
	}
	script := "disable_xdebug"
	if in.Enable {
		script = "enable_xdebug"
	}
	if _, stderr, err := app.Exec(&ddevapp.ExecOpts{Service: "web", RawCmd: []string{script}}); err != nil {
		return xdebugOutput{}, errors.New(strings.TrimSpace(err.Error() + " " + stderr))
	}
	return xdebugOutput{XdebugEnabled: in.Enable}, nil
}

// capTail keeps the last maxExecBytes of s, cut on a valid UTF-8 boundary.
func capTail(s string) (string, bool) {
	if len(s) <= maxExecBytes {
		return s, false
	}
	return strings.ToValidUTF8(s[len(s)-maxExecBytes:], ""), true
}
