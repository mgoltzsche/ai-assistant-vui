package mcp

import (
	"context"

	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

func ToolProvider(servers Servers, serverRefs []config.MCPToolsReference) (tools.ToolProvider, error) {
	filtered := make([]tools.ToolProvider, len(serverRefs))

	for i, ref := range serverRefs {
		p, err := servers.Get(ref.MCPServer)
		if err != nil {
			return nil, err
		}

		if len(ref.AllowTools) == 0 {
			filtered[i] = p
		} else {
			allowed := make(map[string]struct{}, len(ref.AllowTools))
			for _, toolName := range ref.AllowTools {
				allowed[toolName] = struct{}{}
			}

			filtered[i] = &filteredToolProvider{
				delegate:         p,
				allowedToolNames: allowed,
			}
		}
	}

	return toolProviderList(filtered), nil
}

type filteredToolProvider struct {
	delegate         tools.ToolProvider
	allowedToolNames map[string]struct{}
}

func (p filteredToolProvider) Tools(ctx context.Context) ([]tools.Tool, error) {
	result, err := p.delegate.Tools(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]tools.Tool, 0, 10)

	for _, tool := range result {
		if _, allowed := p.allowedToolNames[tool.Definition().Name]; allowed {
			filtered = append(filtered, tool)
		}
	}

	return filtered, nil
}

type toolProviderList []tools.ToolProvider

func (p toolProviderList) Tools(ctx context.Context) ([]tools.Tool, error) {
	tools := make([]tools.Tool, 0, 10)

	for _, provider := range p {
		t, err := provider.Tools(ctx)
		if err != nil {
			return nil, err
		}

		tools = append(tools, t...)
	}

	return tools, nil
}
