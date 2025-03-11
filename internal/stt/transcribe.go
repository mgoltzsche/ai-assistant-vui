package stt

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/orcaman/writerseeker"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type Transcription = model.Message

type Service interface {
	Transcribe(ctx context.Context, wavData []byte) (Transcription, error)
}

type Transcriber struct {
	Service Service
}

// Transcribe transcribes the provided speech to text.
func (t *Transcriber) Transcribe(ctx context.Context, input <-chan audio.Buffer) <-chan Transcription {
	ch := make(chan Transcription, 10)

	go func() {
		defer close(ch)

		for audioBuffer := range input {
			wavFile := &writerseeker.WriterSeeker{}
			encoder := wav.NewEncoder(wavFile, 16000, 16, 1, 1)

			if err := encoder.Write(audioBuffer.AsIntBuffer()); err != nil {
				log.Println(fmt.Errorf("encoder write buffer: %w", err))
				return
			}

			if err := encoder.Close(); err != nil {
				log.Println(fmt.Errorf("encoder close: %w", err))
				return
			}

			wavData, err := io.ReadAll(wavFile.Reader())
			if err != nil {
				log.Println(fmt.Errorf("reading file into memory: %w", err))
				return
			}

			result, err := t.Service.Transcribe(ctx, wavData)
			if err != nil {
				log.Println(fmt.Errorf("failed to transcribe: %w", err))
				return
			}

			result.Text = strings.TrimSuffix(result.Text, "[BLANK_AUDIO]")

			if strings.TrimSpace(result.Text) != "" {
				ch <- result
			}
		}
	}()

	return ch
}
