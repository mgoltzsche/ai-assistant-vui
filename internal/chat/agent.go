package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/tmc/langchaingo/jsonschema"
	"github.com/tmc/langchaingo/llms"
)

type Agent struct {
	Name         string
	Description  string
	SystemPrompt string
	Tools        tools.ToolProvider
	LLM          LLM
}

func (a *Agent) Definition() llms.FunctionDefinition {
	return llms.FunctionDefinition{
		Name:        a.Name,
		Description: a.Description,
		Strict:      true,
		Parameters: jsonschema.Definition{
			Type: "object",
			Properties: map[string]jsonschema.Definition{
				"prompt": {
					Type:        "string",
					Description: "The prompt providing the user request.",
				},
			},
			Required: []string{"prompt"},
		},
	}
}

func (a *Agent) AsTool(reqNum int64, ch chan<- ResponseChunk) *AgentTool {
	return &AgentTool{
		Agent:      a,
		RequestNum: reqNum,
		Ch:         ch,
	}
}

func (a *Agent) invoke(ctx context.Context, prompt string, reqNum int64, ch chan<- ResponseChunk) error {
	tools, err := a.Tools.Tools(ctx)
	if err != nil {
		return err
	}

	conv := model.NewConversation(a.SystemPrompt, reqNum-1)

	conv.AddUserRequest(llms.TextPart(prompt))

	err = a.LLM.ChatCompletion(ctx, reqNum, tools, conv, ch)
	if err != nil {
		return err
	}

	// TODO: ensure the agent called a function, otherwise retry.

	return nil
}

type AgentTool struct {
	*Agent
	RequestNum int64
	Ch         chan<- ResponseChunk
}

func (a *AgentTool) Definition() llms.FunctionDefinition {
	return llms.FunctionDefinition{
		Name:        a.Name,
		Description: a.Description,
		Strict:      true,
		Parameters: jsonschema.Definition{
			Type: "object",
			Properties: map[string]jsonschema.Definition{
				"prompt": {
					Type:        "string",
					Description: "The prompt providing the user request along with the relevant context.",
				},
			},
			Required: []string{"prompt"},
		},
	}
}

func (a *AgentTool) Call(ctx context.Context, arguments string) (string, error) {
	args := map[string]any{}
	if arguments != "" {
		err := json.Unmarshal([]byte(arguments), &args)
		if err != nil {
			return "", fmt.Errorf("parse agent call arguments: %w", err)
		}
	}

	prompt, ok := args["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("no prompt provided for agent %s", a.Name)
	}

	err := a.invoke(ctx, prompt, a.RequestNum, a.Ch)
	if err != nil {
		return "", fmt.Errorf("run %s agent: %w", a.Name, err)
	}

	return "", &ResponseDelegated{errors.New("response delegated")}
}
