package chat

import (
	"context"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type Message = model.Message

type Requester struct {
}

func (r *Requester) AddUserRequestsToConversation(ctx context.Context, requests <-chan Message, conv *model.Conversation) <-chan ChatCompletionRequest {
	ch := make(chan ChatCompletionRequest)

	go func() {
		defer close(ch)

		for req := range requests {
			ch <- ChatCompletionRequest{
				RequestID: conv.AddUserRequest(req.Text + " "),
			}
		}
	}()

	return ch
}
