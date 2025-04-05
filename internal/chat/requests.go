package chat

import (
	"context"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/soundgen"
)

type Message = model.Message

type Requester struct {
}

func (r *Requester) AddUserRequestsToConversation(ctx context.Context, requests <-chan Message, notifications chan<- soundgen.Request, conv *model.Conversation) <-chan ChatCompletionRequest {
	ch := make(chan ChatCompletionRequest)

	go func() {
		defer close(ch)
		defer close(notifications)

		for req := range requests {
			reqNum := conv.AddUserRequest(req.Text + " ")

			ch <- ChatCompletionRequest{
				RequestNum: reqNum,
			}
			notifications <- soundgen.Request{
				RequestNum: reqNum,
			}
			// TODO: close channels
		}
	}()

	return ch
}
