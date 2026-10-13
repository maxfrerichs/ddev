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

// McpCmd serves read-only DDEV project information to MCP clients.
var McpCmd = &cobra.Command{
	Use:         "mcp",
	Annotations: map[string]string{NoDockerCommand: "true"},
	Short:       "Run a read-only MCP server on stdin and stdout",
	Long: `Run a Model Context Protocol (MCP) server that lets AI clients list and
describe DDEV projects, read container logs, and list database snapshots.
The server is read-only and talks to the client over stdin and stdout, so it
is normally started by the client rather than by hand.`,
	Example: `ddev mcp
claude mcp add ddev -- ddev mcp`,
	Args: cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := mcpserver.Run(ctx); err != nil {
			util.Failed("MCP server failed: %v", err)
		}
	},
}

func registerMcpCmd() {
	RootCmd.AddCommand(McpCmd)
}
