package vad

import (
	"fmt"
	"log"
	"time"

	"github.com/go-audio/audio"
	"github.com/streamer45/silero-vad-go/speech"
)

type Detector struct {
	ModelPath string
}

// DetectVoiceActivity detects voice activity within the given audio input data channel.
func (d *Detector) DetectVoiceActivity(input <-chan audio.Buffer) (<-chan audio.Buffer, error) {
	ch := make(chan audio.Buffer, 10)

	sileroVAD, err := speech.NewDetector(speech.DetectorConfig{
		ModelPath:            d.ModelPath,
		SampleRate:           16000,
		Threshold:            0.5,
		MinSilenceDurationMs: 0,
		SpeechPadMs:          0,
	})
	if err != nil {
		return input, fmt.Errorf("create silero vad: %w", err)
	}

	go func() {
		defer func() {
			if err := sileroVAD.Destroy(); err != nil {
				log.Printf("WARNING: destroy silero vad: %v\n", err)
			}
			close(ch)
		}()

		for audioBuffer := range input {
			start := time.Now()
			segments, err := sileroVAD.Detect(audioBuffer.AsFloat32Buffer().Data)
			if err != nil {
				log.Println("WARNING: detect voice:", err)
				continue
			}
			detected := len(segments) > 0
			log.Printf("voice activity detected: %v (took %s)\n", detected, time.Since(start))

			if detected {
				ch <- audioBuffer
			}
		}
	}()

	return ch, nil
}
