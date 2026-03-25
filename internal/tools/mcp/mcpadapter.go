package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/tmc/langchaingo/llms"
)

func NewMCPTool(tool mcp.Tool, client client.MCPClient) (tools.Tool, error) {
	def, err := mcpToolDefinition(tool)
	if err != nil {
		return nil, fmt.Errorf("create adapter for mcp tool %s: %w", tool.Name, err)
	}

	return &MCPToolAdapter{
		tool:       tool,
		client:     client,
		definition: def,
	}, nil
}

func mcpToolDefinition(tool mcp.Tool) (llms.FunctionDefinition, error) {
	return llms.FunctionDefinition{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  tool.InputSchema,
	}, nil
}

type MCPToolAdapter struct {
	tool       mcp.Tool
	client     client.MCPClient
	definition llms.FunctionDefinition
}

func (t *MCPToolAdapter) Name() string {
	return t.tool.Name
}

func (t *MCPToolAdapter) Definition() llms.FunctionDefinition {
	return t.definition
}

func (t *MCPToolAdapter) Call(ctx context.Context, params map[string]any) (string, error) {
	result, err := t.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      t.tool.Name,
			Arguments: params,
		},
	})
	if err != nil {
		return "", err
	}
	msg, err := textContentToString(result.Content)
	if err != nil {
		return "", fmt.Errorf("read mcp tool call response: %w", err)
	}
	if result.IsError {
		return "", fmt.Errorf("tool returned error: %s", msg)
	}

	return msg, nil
}

func textContentToString(content []mcp.Content) (string, error) {
	msg := make([]string, 0, 1)
	for _, c := range content {
		switch t := c.(type) {
		case mcp.TextContent:
			msg = append(msg, t.Text)
		default:
			// TODO: also support other content types
			return "", fmt.Errorf("unsupported content of type %T", c)
		}
	}
	return strings.Join(msg, ""), nil
}
