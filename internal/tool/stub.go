package tool

import "context"

// StubProvider is a no-op tool provider that returns "not available" for all tools.
// Useful as a fallback when users haven't registered real implementations.
type StubProvider struct {
	tool Tool
}

func NewStub(t Tool) *StubProvider { return &StubProvider{tool: t} }

func (s *StubProvider) Tool() Tool { return s.tool }

func (s *StubProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	return NotAvailableMsg, nil
}

// BuiltinToolProvider implements tools that don't require external system access.
type BuiltinToolProvider struct {
	tool Tool
	fn   func(ctx context.Context, args map[string]any) (string, error)
}

func NewBuiltin(t Tool, fn func(ctx context.Context, args map[string]any) (string, error)) *BuiltinToolProvider {
	return &BuiltinToolProvider{tool: t, fn: fn}
}

func (b *BuiltinToolProvider) Tool() Tool { return b.tool }
func (b *BuiltinToolProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return b.fn(ctx, args)
}

// MCPCaller abstracts the MCP manager for tool dispatch, avoiding a
// circular import between the tool and mcp packages.
type MCPCaller interface {
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// MCPToolProvider wraps an MCP-discovered tool as a standard Provider.
type MCPToolProvider struct {
	tool    Tool
	manager MCPCaller
}

// NewMCPProvider creates a Provider that delegates to an MCP server.
func NewMCPProvider(name string, manager MCPCaller) *MCPToolProvider {
	return &MCPToolProvider{tool: OfDynamic(name), manager: manager}
}

func (p *MCPToolProvider) Tool() Tool { return p.tool }

func (p *MCPToolProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return p.manager.CallTool(ctx, p.tool.Name(), args)
}
