package soundgen

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/orcaman/writerseeker"
)

type GeneratedSound = model.AudioMessage

func GenerateSound(sampleRate int, frequency float64, duration time.Duration) ([]byte, error) {
	data := make([]int, int(math.Ceil(float64(duration)*float64(sampleRate)/float64(time.Second))))
	for i := range data {
		phase := frequency * float64(i) / float64(sampleRate)

		data[i] = int(math.Sin(2*math.Pi*phase) * 32767)
	}

	buf := &audio.IntBuffer{
		Format:         &audio.Format{SampleRate: sampleRate, NumChannels: 1},
		Data:           data,
		SourceBitDepth: 16,
	}

	wavFile := &writerseeker.WriterSeeker{}
	encoder := wav.NewEncoder(wavFile, buf.Format.SampleRate, 16, 1, 1)

	err := encoder.Write(buf.AsIntBuffer())
	if err != nil {
		return nil, fmt.Errorf("write wav: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close wav encoder: %w", err)
	}

	b, err := io.ReadAll(wavFile.Reader())
	if err != nil {
		return nil, fmt.Errorf("read generated wav: %w", err)
	}

	return b, nil
}
