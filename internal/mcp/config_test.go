package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FileNotFound(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/mcp_servers.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("expected empty servers, got %d", len(cfg.Servers))
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_servers.json")
	data := `{
		"servers": [
			{
				"name": "test-server",
				"command": "echo",
				"args": ["hello"],
				"enabled": true
			},
			{
				"name": "disabled-server",
				"command": "false",
				"args": [],
				"enabled": false
			}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Name != "test-server" {
		t.Errorf("expected name 'test-server', got %q", cfg.Servers[0].Name)
	}
	if cfg.Servers[0].Command != "echo" {
		t.Errorf("expected command 'echo', got %q", cfg.Servers[0].Command)
	}
	if len(cfg.Servers[0].Args) != 1 || cfg.Servers[0].Args[0] != "hello" {
		t.Errorf("unexpected args: %v", cfg.Servers[0].Args)
	}
	if !cfg.Servers[0].Enabled {
		t.Error("expected first server to be enabled")
	}
	if cfg.Servers[1].Enabled {
		t.Error("expected second server to be disabled")
	}
}

func TestLoadConfig_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed config")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("expected empty servers, got %d", len(cfg.Servers))
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	// Should end with the expected filename.
	if filepath.Base(path) != "mcp_servers.json" {
		t.Errorf("expected mcp_servers.json, got %q", filepath.Base(path))
	}
}
