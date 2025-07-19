package audio

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/orcaman/writerseeker"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

func AudioBuffersToRiffWavs(input <-chan audio.Buffer) <-chan model.AudioMessage {
	ch := make(chan model.AudioMessage, 5)

	go func() {
		defer close(ch)

		for buf := range input {
			b, err := audioBufferToRiffWav(buf)
			if err != nil {
				slog.Error(fmt.Sprintf("convert audio buffer to riff wave: %s", err))
				continue
			}

			ch <- model.AudioMessage{
				WaveData: b,
			}
		}
	}()

	return ch
}

func audioBufferToRiffWav(buffer audio.Buffer) ([]byte, error) {
	wavFile := &writerseeker.WriterSeeker{}
	f := buffer.PCMFormat()
	encoder := wav.NewEncoder(wavFile, f.SampleRate, 16, f.NumChannels, 1)

	if err := encoder.Write(buffer.AsIntBuffer()); err != nil {
		return nil, fmt.Errorf("encoder write buffer: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encoder close: %w", err)
	}

	riffWav, err := io.ReadAll(wavFile.Reader())
	if err != nil {
		return nil, fmt.Errorf("reading wav into memory: %w", err)
	}

	return riffWav, nil
}
