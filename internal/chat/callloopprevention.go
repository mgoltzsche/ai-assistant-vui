package chat

import (
	"fmt"
	"log"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
)

// preventInfiniteCallLoop excludes functions that have been called with the same arguments repeatedly as part of the response of the last user request.
func preventInfiniteCallLoop(fns []functions.Function, c *model.Conversation) []functions.Function {
	filtered := make([]functions.Function, 0, len(fns))
	callSignatures := make(map[string]struct{}, len(fns))
	blockedFunctions := make(map[string]struct{}, len(fns))

	for _, msg := range c.RequestMessages() {
		if msg.Role == llms.ChatMessageTypeAI {
			for _, p := range msg.Parts {
				if call, ok := p.(llms.ToolCall); ok {
					signature := toolCallSignature(call)

					if _, called := callSignatures[signature]; called {
						blockedFunctions[call.FunctionCall.Name] = struct{}{}
						continue
					}

					callSignatures[signature] = struct{}{}
				}
			}
		}
	}

	blockedNames := make([]string, 0, len(blockedFunctions))

	for _, fn := range fns {
		if _, blocked := blockedFunctions[fn.Definition().Name]; !blocked {
			filtered = append(filtered, fn)
		} else {
			blockedNames = append(blockedNames, fn.Definition().Name)
		}
	}

	if len(blockedNames) > 0 {
		log.Println("WARNING: Detected infinite tool call loop. Disabling tool temporarily:", strings.Join(blockedNames, ", "))
	}

	return filtered
}

func toolCallSignature(call llms.ToolCall) string {
	return fmt.Sprintf("%s(%#v)", call.FunctionCall.Name, call.FunctionCall.Arguments)
}
