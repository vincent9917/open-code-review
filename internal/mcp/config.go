package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerConfig defines a single MCP server to connect to.
type ServerConfig struct {
	Name    string   `json:"name"`              // e.g., "codegraph"
	Command string   `json:"command"`            // e.g., "codegraph"
	Args    []string `json:"args,omitempty"`     // e.g., ["serve", "--mcp"]
	Enabled bool     `json:"enabled"`            // whether to start this server
}

// Config holds the list of configured MCP servers.
type Config struct {
	Servers []ServerConfig `json:"servers"`
}

// LoadConfig reads and parses the MCP server configuration file.
// Returns an empty config when the file does not exist.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read MCP config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal MCP config file %s: %w", path, err)
	}

	// Validate required fields.
	for i, s := range cfg.Servers {
		if s.Name == "" {
			return nil, fmt.Errorf("MCP config: server at index %d has empty name", i)
		}
		if s.Command == "" {
			return nil, fmt.Errorf("MCP config: server %q has empty command", s.Name)
		}
	}

	return &cfg, nil
}

// DefaultConfigPath returns the path to the MCP server configuration file.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "mcp_servers.json"), nil
}
