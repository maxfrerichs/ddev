package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMcpServesOnlyProtocolOnStdout starts the real binary because stdout
// purity depends on process-level state that an in-process test cannot cover.
func TestMcpServesOnlyProtocolOnStdout(t *testing.T) {
	c := exec.Command(DdevBin, "mcp")
	var stderr bytes.Buffer
	c.Stderr = &stderr
	stdin, err := c.StdinPipe()
	require.NoError(t, err)
	stdout, err := c.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, c.Start())
	t.Cleanup(func() { _ = c.Process.Kill() })

	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(nil, 16*1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	for _, req := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ddev_describe_project","arguments":{"name":"no-such-project-for-mcp-test"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ddev_version","arguments":{}}}`,
	} {
		_, err = io.WriteString(stdin, req+"\n")
		require.NoError(t, err)
	}

	type response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			IsError bool `json:"isError"`
			Tools   []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	responses := map[int]response{}
	timeout := time.After(60 * time.Second)
	for len(responses) < 4 {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "stdout closed early; stderr: %s", stderr.String())
			var r response
			require.NoError(t, json.Unmarshal([]byte(line), &r), "stdout line is not JSON: %q", line)
			require.Equal(t, "2.0", r.JSONRPC, "stdout line is not JSON-RPC: %q", line)
			responses[r.ID] = r
		case <-timeout:
			t.Fatalf("no answer to all requests; got %d; stderr: %s", len(responses), stderr.String())
		}
	}

	require.Len(t, responses[2].Result.Tools, 5)
	require.True(t, responses[3].Result.IsError, "an unknown project must be a tool error, not a dead server")
	require.False(t, responses[4].Result.IsError, "the server must still answer after a failing call")

	require.NoError(t, stdin.Close())
	require.NoError(t, c.Wait(), "stderr: %s", stderr.String())
}
