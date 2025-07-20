package tts

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type Request = model.Message
type GeneratedSpeech = model.AudioMessage

type SpeechGenerator struct {
	Service *Client
}

func (g *SpeechGenerator) GenerateAudio(ctx context.Context, requests <-chan Request, conv *model.Conversation) <-chan GeneratedSpeech {
	ch := make(chan GeneratedSpeech, 10)

	go func() {
		defer close(ch)

		for req := range requests {
			if conv.RequestCounter() > req.RequestNum {
				// Skip request if outdated (user requested something else)
				continue
			}

			body, err := g.Service.GenerateAudio(ctx, req)
			if err != nil {
				slog.Error(fmt.Sprintf("generate speech: %s", err))
				continue
			}
			b, err := io.ReadAll(body)
			if err != nil {
				slog.Error(fmt.Sprintf("read speech generation response body: %s", err))
				continue
			}

			ch <- GeneratedSpeech{
				Message:  req,
				WaveData: b,
			}
		}
	}()

	return ch
}
