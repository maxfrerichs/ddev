package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/ddev/ddev/pkg/util"
	"github.com/ddev/ddev/pkg/versionconstants"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverSide, clientSide := mcp.NewInMemoryTransports()
	_, err := srv.Connect(ctx, serverSide, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientSide, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected text content, got %T", res.Content[0])
	return text.Text
}

var (
	readTools  = []string{"ddev_describe_project", "ddev_get_config", "ddev_get_logs", "ddev_get_status", "ddev_list_addons", "ddev_list_snapshots", "ddev_version"}
	writeTools = []string{"ddev_restart", "ddev_snapshot_create", "ddev_snapshot_restore", "ddev_start", "ddev_stop"}
	execTools  = []string{"ddev_exec", "ddev_xdebug"}
)

func listTools(t *testing.T, opts Options) map[string]*mcp.ToolAnnotations {
	t.Helper()
	res, err := connect(t, New("vtest", opts)).ListTools(context.Background(), nil)
	require.NoError(t, err)
	got := map[string]*mcp.ToolAnnotations{}
	for _, tool := range res.Tools {
		got[tool.Name] = tool.Annotations
	}
	return got
}

func TestToolsAreListedWithAnnotations(t *testing.T) {
	got := listTools(t, Options{})

	require.Len(t, got, len(readTools)+1)
	for _, name := range readTools {
		require.Contains(t, got, name)
		require.True(t, got[name].ReadOnlyHint, "%s must be read-only", name)
	}
	// ddev_list_projects prunes the project registry, so it must not claim to be read-only.
	require.Contains(t, got, "ddev_list_projects")
	require.False(t, got["ddev_list_projects"].ReadOnlyHint)
}

func TestToolTiersAreIndependent(t *testing.T) {
	base := len(readTools) + 1
	for _, tc := range []struct {
		opts Options
		have []string
		not  []string
	}{
		{Options{AllowWrite: true}, writeTools, execTools},
		{Options{AllowExec: true}, execTools, writeTools},
		{Options{AllowWrite: true, AllowExec: true}, append(slices.Clone(writeTools), execTools...), nil},
	} {
		got := listTools(t, tc.opts)
		require.Len(t, got, base+len(tc.have), "%+v", tc.opts)
		for _, name := range tc.have {
			require.Contains(t, got, name, "%+v", tc.opts)
			require.False(t, got[name].ReadOnlyHint, "%s must not claim to be read-only", name)
		}
		for _, name := range tc.not {
			require.NotContains(t, got, name, "%+v", tc.opts)
		}
	}
	require.True(t, *listTools(t, Options{AllowWrite: true})["ddev_snapshot_restore"].DestructiveHint)
}

func TestActionInputsAreValidatedBeforeAnyProjectIsLoaded(t *testing.T) {
	session := connect(t, New("vtest", Options{AllowWrite: true, AllowExec: true}))

	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"ddev_start", map[string]any{"name": " "}, "name is required"},
		{"ddev_stop", map[string]any{"name": ""}, "name is required"},
		{"ddev_exec", map[string]any{"name": "x", "command": []string{}}, "command is required"},
		{"ddev_exec", map[string]any{"name": "x", "command": []string{"ls"}, "service": "ddev-router"}, "service must be web or db"},
		{"ddev_exec", map[string]any{"name": " ", "command": []string{"ls"}}, "name is required"},
		{"ddev_snapshot_create", map[string]any{"name": "x", "snapshot_name": "a/../b"}, "invalid snapshot_name"},
		{"ddev_snapshot_restore", map[string]any{"name": "x"}, "snapshot_name is required"},
		{"ddev_xdebug", map[string]any{"name": " ", "enable": true}, "name is required"},
	} {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
		require.NoError(t, err, tc.tool)
		require.True(t, res.IsError, tc.tool)
		require.Contains(t, resultText(t, res), tc.want, tc.tool)
	}
}

func TestInstructionsNameTheEnabledTiers(t *testing.T) {
	require.Contains(t, Options{}.instructions(), "read-only")
	require.NotContains(t, Options{AllowWrite: true}.instructions(), "read-only")
	require.Contains(t, Options{AllowExec: true}.instructions(), "cannot start or stop")
	require.Contains(t, Options{AllowWrite: true, AllowExec: true}.instructions(), "run commands")
}

func TestVersionTool(t *testing.T) {
	session := connect(t, New("vtest", Options{}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "ddev_version", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, resultText(t, res), versionconstants.DdevVersion)
}

func TestProjectNameIsRequired(t *testing.T) {
	session := connect(t, New("vtest", Options{}))

	for _, tool := range []string{"ddev_describe_project", "ddev_get_logs", "ddev_list_snapshots", "ddev_get_config", "ddev_list_addons", "ddev_get_status"} {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"name": " "}})
		require.NoError(t, err, tool)
		require.True(t, res.IsError, tool)
		require.Contains(t, resultText(t, res), "name is required", tool)
	}
}

func TestInvalidServiceIsRejected(t *testing.T) {
	session := connect(t, New("vtest", Options{}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "ddev_get_logs",
		Arguments: map[string]any{"name": "anything", "service": "web; rm -rf /"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "invalid service name")
}

func TestUtilFailedBecomesToolError(t *testing.T) {
	defer guardExit()()

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "fail"}, guarded(func(struct{}) (versionOutput, error) {
		util.Failed("boom %d", 42)
		return versionOutput{}, nil
	}))
	mcp.AddTool(srv, &mcp.Tool{Name: "crash"}, guarded(func(struct{}) (versionOutput, error) {
		panic("unexpected state")
	}))
	session := connect(t, srv)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fail", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "boom 42")
	require.NotContains(t, resultText(t, res), "\x1b[", "terminal colors must be stripped")

	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "crash", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "internal error")

	// The server must still answer after both failures.
	_, err = session.ListTools(context.Background(), nil)
	require.NoError(t, err)
}

func TestServeWritesOnlyJSONRPCToItsWriter(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, clientToServerR, serverToClientW, "vtest", Options{}) }()

	_, err := io.WriteString(clientToServerW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`+"\n")
	require.NoError(t, err)

	line, err := bufio.NewReader(serverToClientR).ReadBytes('\n')
	require.NoError(t, err)
	var msg struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(line, &msg), "line was %q", line)
	require.Equal(t, "2.0", msg.JSONRPC)
	require.Equal(t, 1, msg.ID)
	require.Equal(t, "ddev", msg.Result.ServerInfo.Name)

	cancel()
	require.NoError(t, clientToServerW.Close())
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after its context was canceled")
	}
}
