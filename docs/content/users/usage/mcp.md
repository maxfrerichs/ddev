---
search:
  boost: 2
---

# MCP Server

`ddev mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server that lets AI clients inspect your DDEV projects. By default it is read-only: it cannot start, stop, or change a project, and it cannot run commands in containers. Two flags opt in to more, described under [Enabling Actions](#enabling-actions).

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
| `ddev_get_config`       | A project's configuration from its config files: type, docroot, PHP, webserver, database, the Xdebug startup setting and hook names (not hook commands). |
| `ddev_get_status`       | Whether a project is running, stopped or unhealthy, and its primary URL. It never starts the project. |
| `ddev_list_addons`      | Add-ons installed in one project.                                                   |
| `ddev_version`          | The version of the running DDEV binary.                                             |

Tools that act on one project take its name as shown by `ddev_list_projects`. They never fall back to the current directory.

## Enabling Actions

Each flag is independent of the other and applies to the server the client starts, so add it to the client's command:

```bash
claude mcp add ddev -- ddev mcp --allow-write
claude mcp add ddev -- ddev mcp --allow-write --allow-exec
```

Tools from a tier you did not enable are not offered to the client at all.

### `--allow-write`

| Tool                    | What it does                                                                        |
|-------------------------|-------------------------------------------------------------------------------------|
| `ddev_start`            | Starts a project, as `ddev start`. This can take minutes.                           |
| `ddev_stop`             | Stops a project's containers, as `ddev stop`. The database and files are kept.      |
| `ddev_restart`          | Restarts a project, as `ddev restart`.                                              |
| `ddev_snapshot_create`  | Writes a database snapshot, as `ddev snapshot`.                                     |
| `ddev_snapshot_restore` | Replaces the database with a snapshot, as `ddev snapshot restore`. The current database contents are lost unless a snapshot of them exists. |

These tools run the project's configured [hooks](../configuration/hooks.md), such as `pre-snapshot`, as the matching commands do.

### `--allow-exec`

| Tool          | What it does                                                                                       |
|---------------|----------------------------------------------------------------------------------------------------|
| `ddev_exec`   | Runs a command in the `web` or `db` container, as `ddev exec`. Use it for Composer, Drush, npm, Artisan and similar. Takes the command as a list, such as `["composer", "install"]`. Returns stdout and stderr, each cut to the last 256 KiB. |
| `ddev_xdebug` | Enables or disables Xdebug in a running project. The change lasts until the project restarts and does not edit the configuration. |

`ddev_exec` can change files and data in the project, and it can reach the network from inside the container. Enable it only for clients and models you trust with that. A command that reads standard input receives an empty input, never the client's connection. The call waits until the command finishes, and other tool calls wait behind it.

## Things to Know

* `ddev_list_projects` removes registry entries for project directories that no longer exist, exactly as `ddev list` does. For that reason it is not marked read-only in the tool metadata.
* Log and command output is written by your project's code. A client should treat it as data and not as instructions.
* Anything a tool returns is sent to whichever model the client uses. `ddev_describe_project` includes the project's URLs, paths and database settings, so use clients and models you are comfortable sharing that with.
* Tool calls run one at a time, because DDEV's internals are not safe to run concurrently.
* A failed `ddev_start` or `ddev_restart` can leave a project partly started. Check it with `ddev_get_status` before retrying.
* A DDEV error during a tool call is reported to the client as a tool error and does not stop the server.
* DDEV's regular output goes to standard error while the server runs, because standard output carries the protocol.
* Like other commands, `ddev mcp` is counted in [opt-in usage information](diagnostics.md#opt-in-usage-information) by command name only. Tool calls and their arguments are not sent.
