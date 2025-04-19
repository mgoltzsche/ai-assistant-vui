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

type Request struct {
	RequestNum int64
}

type Generator struct {
	SampleRate int
	sound      []byte
}

func (g *Generator) Notify(conv *model.Conversation) (<-chan GeneratedSound, chan<- Request, error) {
	data, err := g.generateSound(500, 300*time.Millisecond)
	if err != nil {
		return nil, nil, fmt.Errorf("generate sound: %w", err)
	}

	requests := make(chan Request, 10)
	ch := make(chan GeneratedSound, 10)

	go func() {
		defer close(ch)

		for req := range requests {
			if conv.RequestCounter() > req.RequestNum {
				// Skip request if outdated (user requested something else)
				continue
			}

			ch <- GeneratedSound{
				Message: model.Message{
					RequestNum: req.RequestNum,
					Text:       "(acknowledged)",
					UserOnly:   true,
				},
				WaveData: data,
			}
		}
	}()

	return ch, requests, nil
}

func (g *Generator) generateSound(frequency float64, duration time.Duration) ([]byte, error) {
	data := make([]int, int(math.Ceil(float64(duration)*float64(g.SampleRate)/float64(time.Second))))
	for i := range data {
		phase := frequency * float64(i) / float64(g.SampleRate)

		data[i] = int(math.Sin(2*math.Pi*phase) * 32767)
	}

	buf := &audio.IntBuffer{
		Format:         &audio.Format{SampleRate: g.SampleRate, NumChannels: 1},
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
