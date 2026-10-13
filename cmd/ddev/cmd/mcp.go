package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ddev/ddev/pkg/mcpserver"
	"github.com/ddev/ddev/pkg/util"
	"github.com/spf13/cobra"
)

var mcpOptions mcpserver.Options

// McpCmd serves DDEV project information and, when enabled, actions to MCP clients.
var McpCmd = &cobra.Command{
	Use:         "mcp",
	Annotations: map[string]string{NoDockerCommand: "true"},
	Short:       "Run an MCP server on stdin and stdout",
	Long: `Run a Model Context Protocol (MCP) server that lets AI clients inspect DDEV
projects: list and describe them, read configuration, status, add-ons and
container logs, and list database snapshots.

By default the server is read-only. --allow-write adds start, stop, restart and
database snapshots. --allow-exec adds running commands in project containers.
The two flags are independent. The server talks to the client over stdin and
stdout, so it is normally started by the client rather than by hand.`,
	Example: `ddev mcp
claude mcp add ddev -- ddev mcp
claude mcp add ddev -- ddev mcp --allow-write --allow-exec`,
	Args: cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := mcpserver.Run(ctx, mcpOptions); err != nil {
			util.Failed("MCP server failed: %v", err)
		}
	},
}

func registerMcpCmd() {
	McpCmd.Flags().BoolVar(&mcpOptions.AllowWrite, "allow-write", false, "Offer tools that start, stop and snapshot projects")
	McpCmd.Flags().BoolVar(&mcpOptions.AllowExec, "allow-exec", false, "Offer a tool that runs commands in project containers")
	RootCmd.AddCommand(McpCmd)
}
