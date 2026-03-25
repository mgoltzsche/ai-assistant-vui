package chat

import (
	"context"
	"errors"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/jsonschema"
	"github.com/tmc/langchaingo/llms"
)

type answerTool struct {
	RequestNum int64
	Ch         chan<- ResponseChunk
}

func (f *answerTool) Definition() llms.FunctionDefinition {
	return llms.FunctionDefinition{
		Name:        "answer",
		Description: "Call this function to answer the user request finally, once you have all information.",
		Strict:      true,
		Parameters: jsonschema.Definition{
			Type: "object",
			Properties: map[string]jsonschema.Definition{
				"message": {
					Type:        "string",
					Description: "Your answer to the user.",
				},
			},
			Required: []string{"message"},
		},
	}
}

func (f *answerTool) Call(ctx context.Context, params map[string]any) (string, error) {
	msg, ok := params["message"].(string)
	if !ok || msg == "" {
		return "", errors.New("no message provided")
	}

	f.Ch <- ResponseChunk{
		Type:       model.MessageTypeChunk,
		RequestNum: f.RequestNum,
		Text:       msg,
	}

	return "", &ResponseDelegated{errors.New("response delegated")}
}
