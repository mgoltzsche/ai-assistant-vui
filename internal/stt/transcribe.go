package stt

import (
	"context"
	"log"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type Transcription = model.Message
type AudioMessage = model.AudioMessage

type Service interface {
	Transcribe(ctx context.Context, wavData []byte) (Transcription, error)
}

type Transcriber struct {
	Service Service
}

// Transcribe transcribes the provided speech to text.
func (t *Transcriber) Transcribe(ctx context.Context, input <-chan AudioMessage) <-chan Transcription {
	ch := make(chan Transcription, 10)

	go func() {
		defer close(ch)

		for msg := range input {
			result, err := t.Service.Transcribe(ctx, msg.WaveData)
			if err != nil {
				log.Println("ERROR: transcribe:", err)
				continue
			}

			result.Text = strings.TrimSuffix(result.Text, "[BLANK_AUDIO]")

			if strings.TrimSpace(result.Text) != "" {
				ch <- result
			}
		}
	}()

	return ch
}
