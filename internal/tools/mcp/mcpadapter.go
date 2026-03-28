package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/langchaingo/llms"
)

func NewMCPTool(tool mcp.Tool, session *mcp.ClientSession) (tools.Tool, error) {
	def, err := mcpToolDefinition(tool)
	if err != nil {
		return nil, fmt.Errorf("create adapter for mcp tool %s: %w", tool.Name, err)
	}

	return &MCPToolAdapter{
		tool:       tool,
		session:    session,
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
	session    *mcp.ClientSession
	definition llms.FunctionDefinition
}

func (t *MCPToolAdapter) Name() string {
	return t.tool.Name
}

func (t *MCPToolAdapter) Definition() llms.FunctionDefinition {
	return t.definition
}

func (t *MCPToolAdapter) Call(ctx context.Context, args string) (string, error) {
	argObj, err := t.convertArgs(args)
	if err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	result, err := t.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      t.tool.Name,
		Arguments: argObj,
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

func (t *MCPToolAdapter) convertArgs(args string) (any, error) {
	argsSchema, ok := t.definition.Parameters.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected paramter schema of type %T", t.definition.Parameters)
	}
	typeName := argsSchema["type"]
	switch typeName {
	case "object":
		var m map[string]any
		err := json.Unmarshal([]byte(args), &m)
		if err != nil {
			return nil, err
		}
		return m, nil
	case "string":
		return args, nil
	case "boolean":
		return strconv.ParseBool(args)
	case "number":
		return strconv.ParseFloat(args, 64)
	default:
		return nil, fmt.Errorf("unsupported argument type %q", typeName)
	}
}

func textContentToString(content []mcp.Content) (string, error) {
	msg := make([]string, 0, 1)
	for _, c := range content {
		switch t := c.(type) {
		case *mcp.TextContent:
			msg = append(msg, t.Text)
		default:
			// TODO: also support other content types
			return "", fmt.Errorf("unsupported content of type %T", c)
		}
	}
	return strings.Join(msg, ""), nil
}
