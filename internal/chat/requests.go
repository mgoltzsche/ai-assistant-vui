package chat

import (
	"context"
	"log/slog"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/soundgen"
	"github.com/tmc/langchaingo/llms"
)

type Message = model.AudioMessage

type Requester struct {
}

func (r *Requester) AddUserRequestsToConversation(ctx context.Context, requests <-chan Message, notifications chan<- soundgen.Request, conv *model.Conversation) <-chan ChatCompletionRequest {
	ch := make(chan ChatCompletionRequest)

	go func() {
		defer close(ch)
		defer close(notifications)

		for req := range requests {
			var msg llms.ContentPart
			if len(req.Text) > 0 {
				msg = llms.TextPart(req.Text + " ")
			} else if len(req.WaveData) > 0 {
				msg = llms.BinaryPart("audio/wav", req.WaveData)
			} else {
				slog.Warn("skipping request since it doesn't contain any content")

				continue
			}

			reqNum := conv.AddUserRequest(msg)

			ch <- ChatCompletionRequest{
				RequestNum: reqNum,
			}
			notifications <- soundgen.Request{
				RequestNum: reqNum,
			}
		}
	}()

	return ch
}

func ToAudioMessageStreamWithoutAudioData(requests <-chan model.Message) <-chan model.AudioMessage {
	ch := make(chan model.AudioMessage)

	go func() {
		defer close(ch)

		for req := range requests {
			ch <- model.AudioMessage{Message: req}
		}
	}()

	return ch
}
