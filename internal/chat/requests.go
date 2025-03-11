package chat

import (
	"context"
	"log"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
)

type Message = model.Message

type Requester struct {
}

func (r *Requester) AddUserRequestsToConversation(ctx context.Context, requests <-chan Message, conv *model.Conversation) <-chan ChatCompletionRequest {
	ch := make(chan ChatCompletionRequest)

	go func() {
		defer close(ch)

		for request := range requests {
			reqID := conv.NewRequestID()

			log.Println("user request:", request.Text)
			conv.AddMessage(llms.TextParts(llms.ChatMessageTypeHuman, request.Text))

			ch <- ChatCompletionRequest{
				RequestID: reqID,
			}
		}
	}()

	return ch
}
