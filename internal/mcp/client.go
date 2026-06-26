package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// Default timeouts for MCP client operations.
const (
	defaultStartupTimeout = 5 * time.Second
	defaultToolTimeout    = 30 * time.Second
)

// Client manages a single MCP server connection over stdio.
type Client struct {
	command string
	args    []string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	mu      sync.Mutex       // serialises writes to stdin
	nextID  atomic.Int64     // monotonically increasing request ID
	pending map[int64]chan *JSONRPCResponse
	closed  chan struct{}

	serverInfo    ServerInfo
	instructions  string
	startupTimeout time.Duration
	toolTimeout    time.Duration
}

// NewClient creates a Client for the given command and arguments.
func NewClient(command string, args []string) *Client {
	return &Client{
		command:        command,
		args:           args,
		pending:        make(map[int64]chan *JSONRPCResponse),
		closed:         make(chan struct{}),
		startupTimeout: defaultStartupTimeout,
		toolTimeout:    defaultToolTimeout,
	}
}

// Start launches the MCP server subprocess and performs the initialization
// handshake (initialize + initialized notification).
func (c *Client) Start(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.startupTimeout)
	defer cancel()

	c.cmd = exec.CommandContext(ctx, c.command, c.args...)

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	c.stdout = bufio.NewScanner(stdoutPipe)
	// Increase buffer for large responses.
	c.stdout.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start MCP server %q: %w", c.command, err)
	}

	// Start the stdout reader goroutine.
	go c.readLoop()

	// Send initialize request.
	initParams := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    Capabilities{},
		ClientInfo: ClientInfo{
			Name:    "open-code-review",
			Version: "1.0.0",
		},
	}
	resp, err := c.sendRequest(ctx, "initialize", initParams)
	if err != nil {
		c.Close()
		return fmt.Errorf("initialize MCP server %q: %w", c.command, err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		c.Close()
		return fmt.Errorf("parse initialize result from %q: %w", c.command, err)
	}
	c.serverInfo = initResult.ServerInfo
	c.instructions = initResult.Instructions

	// Send initialized notification (no response expected).
	notif := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	c.mu.Lock()
	if err := c.writeLocked(notif); err != nil {
		c.mu.Unlock()
		c.Close()
		return fmt.Errorf("send initialized notification to %q: %w", c.command, err)
	}
	c.mu.Unlock()

	return nil
}

// ListTools sends tools/list and returns the discovered tool definitions.
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	ctx, cancel := context.WithTimeout(ctx, c.toolTimeout)
	defer cancel()

	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("list tools from %q: %w", c.command, err)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list from %q: %w", c.command, err)
	}
	return result.Tools, nil
}

// CallTool sends tools/call and returns the aggregated text content.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.toolTimeout)
	defer cancel()

	params := ToolsCallParams{
		Name:      name,
		Arguments: args,
	}
	resp, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return "", fmt.Errorf("MCP tool %q call failed: %w", name, err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("MCP server %q returned error: [%d] %s",
			c.command, resp.Error.Code, resp.Error.Message)
	}

	var result ToolsCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse MCP tool result: %w", err)
	}

	if result.IsError {
		text := ""
		for _, b := range result.Content {
			text += b.Text
		}
		return "", fmt.Errorf("MCP tool %q returned error: %s", name, text)
	}

	// Aggregate all text content blocks.
	var text string
	for _, b := range result.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	if text == "" && len(result.Content) == 0 {
		return "Tool returned empty result.", nil
	}
	return text, nil
}

// ServerInfo returns the server metadata received during initialization.
func (c *Client) ServerInfo() ServerInfo { return c.serverInfo }

// Instructions returns the server-provided usage instructions (may be empty).
func (c *Client) Instructions() string { return c.instructions }

// IsHealthy reports whether the underlying subprocess is still running.
func (c *Client) IsHealthy() bool {
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	select {
	case <-c.closed:
		return false
	default:
		return true
	}
}

// Close shuts down the MCP server subprocess.
func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil // already closed
	default:
		close(c.closed)
	}

	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		// Give the process a moment to exit gracefully, then kill.
		done := make(chan struct{})
		go func() {
			_ = c.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
		}
	}
	return nil
}

// sendRequest writes a JSON-RPC request to stdin and waits for the response.
func (c *Client) sendRequest(ctx context.Context, method string, params any) (*JSONRPCResponse, error) {
	if c.stdin == nil {
		return nil, fmt.Errorf("MCP client not started")
	}
	id := c.nextID.Add(1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *JSONRPCResponse, 1)

	c.mu.Lock()
	if c.pending == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP server closed")
	}
	if _, ok := c.pending[id]; ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("duplicate request id %d", id)
	}
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.pending != nil {
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()

	if err := c.writeLocked(req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("MCP server closed")
	case resp := <-ch:
		if resp == nil {
			return nil, fmt.Errorf("MCP server closed")
		}
		return resp, nil
	}
}

// writeLocked writes a JSON-RPC message to stdin. Caller must hold c.mu.
func (c *Client) writeLocked(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

// readLoop reads JSON-RPC responses from stdout and routes them to pending request channels.
func (c *Client) readLoop() {
	for c.stdout.Scan() {
		line := c.stdout.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Skip unparseable lines (e.g., server stderr accidentally on stdout).
			continue
		}

		// Notifications have no id — skip them.
		if resp.ID == 0 {
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()
		if ok {
			ch <- &resp
		}
	}

	// Scanner stopped — drain all pending channels so senders don't block.
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = nil
}
