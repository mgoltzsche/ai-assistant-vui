package chat

import (
	"context"
	"log/slog"
	"time"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/soundgen"
	"github.com/tmc/langchaingo/llms"
)

type Event = model.Event
type Message = model.AudioMessage

type Requester struct {
	NotificationSampleRate int
}

func (r *Requester) AddUserRequestsToConversation(ctx context.Context, requests <-chan Message, conv *model.Conversation) (<-chan ChatCompletionRequest, <-chan Event) {
	ch := make(chan ChatCompletionRequest, 50)
	feedbackMessages := make(chan Event, 10)

	notificationSound, err := soundgen.GenerateSound(r.NotificationSampleRate, 500, 300*time.Millisecond)
	if err != nil {
		slog.Error("failed to generate notification sound", "err", err.Error())
	}

	go func() {
		defer close(ch)
		defer close(feedbackMessages)

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
			req.RequestNum = reqNum

			feedbackMessages <- Event{
				Role:         model.RoleUser,
				AudioMessage: req,
			}
			feedbackMessages <- Event{
				Role: model.RoleAssistant,
				AudioMessage: model.AudioMessage{
					Message: model.Message{
						RequestNum: reqNum,
						Text:       "(acknowledged)",
					},
					WaveData: notificationSound,
				},
			}
			ch <- ChatCompletionRequest{
				RequestNum: reqNum,
			}
		}
	}()

	return ch, feedbackMessages
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
