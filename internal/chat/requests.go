package chat

import (
	"context"
	"log"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
)

type Request = model.Request

type Requester struct {
}

func (r *Requester) AddUserRequestsToConversation(ctx context.Context, requests <-chan Request, conv *model.Conversation) <-chan ChatCompletionRequest {
	ch := make(chan ChatCompletionRequest)

	go func() {
		defer close(ch)

		for request := range requests {
			if conv.RequestCounter() > request.ID {
				continue // skip outdated request (user requested something else)
			}

			log.Println("user request:", request.Text)

			conv.AddMessage(llms.TextParts(llms.ChatMessageTypeHuman, request.Text))

			ch <- ChatCompletionRequest{
				RequestID: request.ID,
			}
		}
	}()

	return ch
}
