package audio

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/go-audio/audio"
	"github.com/gordonklaus/portaudio"
)

type Input struct {
	Device      string
	SampleRate  int
	Channels    int
	MinVolume   int
	MinDelay    time.Duration
	MaxDuration time.Duration
}

// RecordAudio opens an audio input device and emits samples into the returned channel.
func (o *Input) RecordAudio(ctx context.Context) (<-chan audio.Buffer, error) {
	device, err := inputDevice(o.Device)
	if err != nil {
		return nil, err
	}

	in := make([]int16, 512*9) // Use int16 to capture 16-bit samples.
	audioStream, err := portaudio.OpenStream(portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   device,
			Channels: o.Channels,
			Latency:  device.DefaultLowInputLatency,
		},
		SampleRate:      device.DefaultSampleRate,
		FramesPerBuffer: len(in),
	}, &in)
	if err != nil {
		return nil, fmt.Errorf("opening audio input stream: %w", err)
	}

	err = audioStream.Start()
	if err != nil {
		return nil, fmt.Errorf("starting audio input stream: %w", err)
	}

	ch := make(chan audio.Buffer, 5)

	go func() {
		defer close(ch)

		var lastHeard time.Time
		buffer := make([]int16, 512*9)

		for {
			select {
			case <-ctx.Done():
				if err := audioStream.Stop(); err != nil {
					slog.Warn("failed to stop input audio stream", "err", err)
				}
				if err := audioStream.Close(); err != nil {
					slog.Warn("failed to close input audio stream", "err", err)
				}
				return
			default:
				if err := audioStream.Read(); err != nil {
					if err == portaudio.InputOverflowed {
						slog.Warn("audio input overflowed - dropped samples")
					} else {
						slog.Warn("failed to read audio stream", "err", err)
					}
					continue
				}

				volume := calculateRMS16(in)
				if int(volume) > o.MinVolume {
					lastHeard = time.Now()
				}

				if time.Since(lastHeard) < o.MinDelay && time.Duration(int64(math.Ceil(float64(len(buffer)+len(in))/16000)))*time.Second < o.MaxDuration {
					buffer = append(buffer, in...)

					//slog.Debug(fmt.Sprintf("listening (volume: %d)...\n", int(volume)))
				} else if len(buffer) > 0 {
					buffer = resampleInt16(buffer, int(device.DefaultSampleRate), o.SampleRate)
					ch <- &audio.IntBuffer{
						Format:         &audio.Format{SampleRate: o.SampleRate, NumChannels: o.Channels},
						Data:           int16ToInt(buffer),
						SourceBitDepth: 16,
					}

					buffer = buffer[:0]
				}
			}
		}
	}()

	return ch, nil
}

// calculateRMS16 calculates the root mean square of the audio buffer for int16 samples.
func calculateRMS16(buffer []int16) float64 {
	var sumSquares float64
	for _, sample := range buffer {
		val := float64(sample)
		sumSquares += val * val
	}
	meanSquares := sumSquares / float64(len(buffer))
	return math.Sqrt(meanSquares)
}

func int16ToInt(input []int16) []int {
	output := make([]int, len(input))
	for i, value := range input {
		output[i] = int(value)
	}
	return output
}
