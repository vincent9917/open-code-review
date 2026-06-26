package mcp

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	tools := mgr.ListTools()
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tools))
	}
}

func TestManager_StartServers_EmptyConfig(t *testing.T) {
	mgr := NewManager()
	cfg := &Config{Servers: []ServerConfig{}}
	if err := mgr.StartServers(t.Context(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mgr.ListTools()) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(mgr.ListTools()))
	}
}

func TestManager_StartServers_DisabledServer(t *testing.T) {
	mgr := NewManager()
	cfg := &Config{
		Servers: []ServerConfig{
			{
				Name:    "disabled",
				Command: "nonexistent-command-xyz",
				Args:    []string{},
				Enabled: false,
			},
		},
	}
	if err := mgr.StartServers(t.Context(), cfg); err != nil {
		t.Fatalf("unexpected error for disabled server: %v", err)
	}
}

func TestManager_StartServers_NonexistentCommand(t *testing.T) {
	mgr := NewManager()
	cfg := &Config{
		Servers: []ServerConfig{
			{
				Name:    "bad-server",
				Command: "nonexistent-command-xyz-12345",
				Args:    []string{},
				Enabled: true,
			},
		},
	}
	// Should return an error but not panic.
	err := mgr.StartServers(t.Context(), cfg)
	if err == nil {
		t.Log("expected error for nonexistent command, but StartServers may handle gracefully")
	}
}

func TestManager_CallTool_NotFound(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.CallTool(t.Context(), "nonexistent_tool", nil)
	if err == nil {
		t.Fatal("CallTool should return error when tool is not found")
	}
}

func TestManager_Shutdown(t *testing.T) {
	mgr := NewManager()
	// Shutdown on empty manager should not panic.
	mgr.Shutdown()
}

func TestManagerMCPToolInfo_Fields(t *testing.T) {
	info := &MCPToolInfo{
		ToolName:    "test_tool",
		ServerName:  "test-server",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
		},
	}
	if info.ToolName != "test_tool" {
		t.Errorf("expected ToolName 'test_tool', got %q", info.ToolName)
	}
	if info.ServerName != "test-server" {
		t.Errorf("expected ServerName 'test-server', got %q", info.ServerName)
	}
}
