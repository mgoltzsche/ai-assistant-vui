package chat

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
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
	Functions functions.FunctionProvider
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

			var functionCallResult string

			fnList, err := r.Functions.Functions()
			if err == nil {
				var fn functions.Function
				fn, err = functions.FindByName(call.FunctionCall.Name, fnList)
				if err == nil {
					err = validateParameters(call.FunctionCall, fn.Definition())
					if err == nil {
						functionCallResult, err = fn.Call(call.FunctionCall.Arguments)
						functionCallResult = strings.TrimSpace(functionCallResult)

						if err == nil && functionCallResult == "" {
							err = fmt.Errorf("function call %q returned empty result", call.FunctionCall.Name)
						}
					}
				}
			}
			if err != nil {
				log.Println("ERROR: Failed to call function:", err)

				functionCallResult = fmt.Sprintf("Failed to call function: %s", err)
			} else {
				for _, line := range strings.Split(functionCallResult, "\n") {
					log.Printf("Function %s result: %s", call.FunctionCall.Name, line)
				}
			}

			conv.AddToolResponse(call.RequestID, llms.ToolCallResponse{
				ToolCallID: call.ToolCallID,
				Name:       call.FunctionCall.Name,
				Content:    functionCallResult,
			})

			completionRequests <- ChatCompletionRequest{
				RequestID: call.RequestID,
			}
		}
	}()

	return completionRequests, toolCalls
}

func validateParameters(call FunctionCall, paramDefinition llms.FunctionDefinition) error {
	if len(call.Arguments) == 0 {
		return fmt.Errorf("function %q called with empty arguments", call.Name)
	}

	// TODO: validate parameters

	return nil
}
