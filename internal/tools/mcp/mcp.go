package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

func NewServers(ctx context.Context, mcpServers map[string]config.MCPServer) (Servers, error) {
	providers := Servers(make(map[string]tools.ToolProvider, 1))

	for _, k := range slices.Sorted(maps.Keys(mcpServers)) {
		s := mcpServers[k]

		mcpClient, err := client.NewStdioMCPClient(s.Command, nil, s.Args...)
		if err != nil {
			_ = providers.Close()
			return nil, fmt.Errorf("create mcp client for server %s: %w", k, err)
		}

		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "ai-assistant-vui",
			Version: "0.0.1",
		}

		_, err = mcpClient.Initialize(ctx, initRequest)
		if err != nil {
			return nil, fmt.Errorf("initialize mcp client %s: %w", k, err)
		}

		providers[k] = &mcpToolProvider{
			Client: mcpClient,
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
	Client *client.Client
}

func (p *mcpToolProvider) Close() error {
	return p.Client.Close()
}

func (p *mcpToolProvider) Tools(ctx context.Context) ([]tools.Tool, error) {
	result, err := p.Client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list mcp tools: %w", err)
	}
	// TODO: handle pagination
	toolAdapter := make([]tools.Tool, len(result.Tools))
	for i, mcpTool := range result.Tools {
		ta, err := NewMCPTool(mcpTool, p.Client)
		if err != nil {
			return nil, err
		}
		toolAdapter[i] = ta
	}

	return toolAdapter, nil
}
