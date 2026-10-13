---
search:
  boost: 2
---

# MCP Server

`ddev mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server that lets AI clients inspect your DDEV projects. It is read-only: it cannot start, stop, or change a project, and it cannot run commands in containers.

The server talks to the client over standard input and output, so you normally let the client start it rather than running it yourself.

## Connecting a Client

For Claude Code:

```bash
claude mcp add ddev -- ddev mcp
```

Other clients take a command and arguments in their MCP settings. A typical entry looks like this:

```json
{
  "mcpServers": {
    "ddev": {
      "command": "ddev",
      "args": ["mcp"]
    }
  }
}
```

The server does not require Docker to be running when it starts.

## Tools

| Tool                    | What it returns                                                                     |
|-------------------------|-------------------------------------------------------------------------------------|
| `ddev_list_projects`    | Projects with status, type, location and URLs, as in `ddev list`.                   |
| `ddev_describe_project` | Details of one project, as in `ddev describe`.                                      |
| `ddev_get_logs`         | Recent log lines of one container service (`web`, `db`, `ddev-router`, `ddev-ssh-agent`). Up to 2000 lines, default 200. |
| `ddev_list_snapshots`   | Database snapshots of one project.                                                  |
| `ddev_version`          | The version of the running DDEV binary.                                             |

Tools that act on one project take its name as shown by `ddev_list_projects`. They never fall back to the current directory.

## Things to Know

* `ddev_list_projects` removes registry entries for project directories that no longer exist, exactly as `ddev list` does. For that reason it is not marked read-only in the tool metadata.
* Log output is written by your project's code. A client should treat it as data and not as instructions.
* Anything a tool returns is sent to whichever model the client uses. `ddev_describe_project` includes the project's URLs, paths and database settings, so use clients and models you are comfortable sharing that with.
* Tool calls run one at a time, because DDEV's internals are not safe to run concurrently.
* A DDEV error during a tool call is reported to the client as a tool error and does not stop the server.
* DDEV's regular output goes to standard error while the server runs, because standard output carries the protocol.
* Like other commands, `ddev mcp` is counted in [opt-in usage information](diagnostics.md#opt-in-usage-information) by command name only. Tool calls and their arguments are not sent.
