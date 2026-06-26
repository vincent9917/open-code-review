package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// MCPToolInfo holds metadata about a discovered MCP tool.
type MCPToolInfo struct {
	ToolName    string         // e.g., "codegraph_explore"
	ServerName  string         // which server provides this tool
	Description string         // human-readable description
	InputSchema map[string]any // JSON Schema for parameters
}

// Manager manages the lifecycle of multiple MCP server clients.
type Manager struct {
	clients map[string]*Client       // server name → client
	tools   map[string]*MCPToolInfo  // tool name → metadata
	mu      sync.RWMutex
}

// NewManager creates a new Manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		tools:   make(map[string]*MCPToolInfo),
	}
}

// StartServers starts all enabled MCP servers in parallel.
// Failures are logged and skipped; they do not block review.
func (m *Manager) StartServers(ctx context.Context, cfg *Config) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(cfg.Servers))

	for _, sc := range cfg.Servers {
		if !sc.Enabled {
			continue
		}
		sc := sc
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := NewClient(sc.Command, sc.Args)
			if err := client.Start(ctx); err != nil {
				errCh <- fmt.Errorf("start MCP server %q: %w", sc.Name, err)
				return
			}
			m.mu.Lock()
			m.clients[sc.Name] = client
			m.mu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)

	// Collect all errors.
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("MCP server startup errors: %v", errs)
	}
	return nil
}

// DiscoverTools calls tools/list on all connected servers and merges
// the discovered tools into the manager's tool map.
func (m *Manager) DiscoverTools(ctx context.Context) error {
	m.mu.RLock()
	clients := make(map[string]*Client, len(m.clients))
	for name, c := range m.clients {
		clients[name] = c
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	errCh := make(chan error, len(clients))

	for srvName, client := range clients {
		srvName := srvName
		client := client
		wg.Add(1)
		go func() {
			defer wg.Done()
			tools, err := client.ListTools(ctx)
			if err != nil {
				errCh <- fmt.Errorf("discover tools from %q: %w", srvName, err)
				return
			}
			m.mu.Lock()
			for _, td := range tools {
				if existing, exists := m.tools[td.Name]; exists {
					fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP tool %q from %q overwrites existing tool from %q\n",
					td.Name, srvName, existing.ServerName)
				}
				m.tools[td.Name] = &MCPToolInfo{
					ToolName:    td.Name,
					ServerName:  srvName,
					Description: td.Description,
					InputSchema: td.InputSchema,
				}
			}
			m.mu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("MCP tool discovery errors: %v", errs)
	}
	return nil
}

// CallTool routes a tool call to the appropriate MCP server.
func (m *Manager) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	m.mu.RLock()
	info, ok := m.tools[name]
	var client *Client
	if ok {
		client = m.clients[info.ServerName]
	}
	m.mu.RUnlock()

	if !ok || client == nil {
		return "", fmt.Errorf("MCP tool %q is not available", name)
	}
	return client.CallTool(ctx, name, args)
}

// Instructions returns the concatenated instructions from all connected servers.
func (m *Manager) Instructions() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var parts []string
	for _, client := range m.clients {
		if ins := client.Instructions(); ins != "" {
			parts = append(parts, ins)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// ListTools returns all discovered MCP tools.
func (m *Manager) ListTools() []*MCPToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*MCPToolInfo, 0, len(m.tools))
	for _, t := range m.tools {
		out = append(out, t)
	}
	return out
}

// Shutdown closes all MCP server subprocesses.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, client := range m.clients {
		_ = client.Close()
	}
	m.clients = nil
	m.tools = nil
}
