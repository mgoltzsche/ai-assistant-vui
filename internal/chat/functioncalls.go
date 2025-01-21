package chat

import (
	"context"
	"log"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
)

type ToolCallRequest struct {
	RequestID    int64
	ToolCallID   string
	FunctionCall FunctionCall
}

type FunctionCall struct {
	Name      string
	Arguments map[string]any
}

type FunctionRunner struct {
}

func (r *FunctionRunner) RunFunctionCalls(ctx context.Context, conv *model.Conversation) (<-chan ChatCompletionRequest, chan<- ToolCallRequest) {
	completionRequests := make(chan ChatCompletionRequest, 10)
	toolCalls := make(chan ToolCallRequest, 10)

	go func() {
		defer close(completionRequests)

		go func() {
			<-ctx.Done()
			// TODO: ideally clean this up: close the channel in the goroutine that writes it!
			close(toolCalls)
		}()

		for call := range toolCalls {
			// TODO: skip or not? We cannot skip, unless the initial tool call request AI message is also added to the conversation by the runner.
			//if conv.RequestCounter() > call.RequestID {
			//  conf.AddMessage(skipped)
			//	continue // skip outdated request (user requested something else)
			//}

			log.Printf("Calling function %q with args %#v", call.FunctionCall.Name, call.FunctionCall.Arguments)

			// TODO: implement actual call

			fakeFunctionCallResult := "sunny day, 27°C"

			conv.AddMessage(llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: call.ToolCallID,
						Name:       call.FunctionCall.Name,
						Content:    fakeFunctionCallResult,
					},
				},
			})

			completionRequests <- ChatCompletionRequest{
				RequestID: call.RequestID,
			}
		}
	}()

	return completionRequests, toolCalls
}
