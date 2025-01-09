package tts

import (
	"context"
	"io"
	"log"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type Request = model.Request

type GeneratedSpeech struct {
	RequestID int64
	Text      string
	WaveData  []byte
}

type SpeechGenerator struct {
	Service *Client
}

func (g *SpeechGenerator) GenerateAudio(ctx context.Context, requests <-chan Request) <-chan GeneratedSpeech {
	ch := make(chan GeneratedSpeech, 10)

	go func() {
		defer close(ch)

		for req := range requests {
			body, err := g.Service.GenerateAudio(ctx, req)
			if err != nil {
				log.Println("ERROR: generate speech:", err)
				continue
			}
			b, err := io.ReadAll(body)
			if err != nil {
				log.Println("ERROR: read speech generation response body:", err)
				continue
			}

			ch <- GeneratedSpeech{
				RequestID: req.ID,
				Text:      req.Text,
				WaveData:  b,
			}
		}
	}()

	return ch
}
