package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"slices"

	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServers(ctx context.Context, mcpServers map[string]config.MCPServer) (Servers, error) {
	providers := Servers(make(map[string]tools.ToolProvider, 1))

	for _, k := range slices.Sorted(maps.Keys(mcpServers)) {
		s := mcpServers[k]

		mcpClient := mcp.NewClient(&mcp.Implementation{Name: "mcp-client", Version: "v1.0.0"}, nil)

		// Connect to a server over stdin/stdout.
		transport := &mcp.CommandTransport{Command: exec.Command(s.Command, s.Args...)}
		session, err := mcpClient.Connect(ctx, transport, nil)
		if err != nil {
			_ = providers.Close()
			return nil, err
		}
		if session.InitializeResult().Capabilities.Tools == nil {
			_ = session.Close()
			_ = providers.Close()
			return nil, errors.New("MCP server does not support tools")
		}

		providers[k] = &mcpToolProvider{
			Name:    k,
			Session: session,
		}
	}

	return providers, nil
}

type Servers map[string]tools.ToolProvider

func (s Servers) Close() error {
	errs := make([]error, 0, len(s))

	for _, provider := range s {
		if closer, ok := provider.(io.Closer); ok {
			err := closer.Close()
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close tool providers: %w", errors.Join(errs...))
	}

	return nil
}

func (s Servers) Get(name string) (tools.ToolProvider, error) {
	srv, ok := s[name]
	if !ok {
		return nil, fmt.Errorf("mcp server %q does not exist", name)
	}
	return srv, nil
}

type mcpToolProvider struct {
	Name    string
	Session *mcp.ClientSession
}

func (p *mcpToolProvider) Close() error {
	return p.Session.Close()
}

func (p *mcpToolProvider) Tools(ctx context.Context) ([]tools.Tool, error) {
	// TODO: handle pagination?
	toolAdapter := make([]tools.Tool, 0, 10)
	for mcpTool, err := range p.Session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("mcp server %s: %w", p.Name, err)
		}
		ta, err := NewMCPTool(*mcpTool, p.Session)
		if err != nil {
			return nil, err
		}
		toolAdapter = append(toolAdapter, ta)
	}

	return toolAdapter, nil
}
