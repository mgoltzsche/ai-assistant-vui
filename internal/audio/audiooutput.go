package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/gordonklaus/portaudio"
	"github.com/mgoltzsche/ai-agent-vui/internal/model"
)

type PlayRequest struct {
	RequestID int64
	Text      string
	WaveData  []byte
}

type Played struct {
	Text string
}

type Output struct {
	Device string
}

func (o *Output) PlayAudio(ctx context.Context, input <-chan PlayRequest, conv *model.ConversationContext) (<-chan Played, error) {
	device, err := outputDevice(o.Device)
	if err != nil {
		return nil, err
	}

	ch := make(chan Played, 10)

	go func() {
		defer close(ch)

		for req := range input {
			if conv.RequestCounter() > req.RequestID {
				// Skip playing response if request is outdated (user requested something else)
				continue
			}

			select {
			case <-ctx.Done():
				continue
			default:
			}

			ch <- Played{
				Text: req.Text,
			}

			err := playAudio(ctx, bytes.NewReader(req.WaveData), device)
			if err != nil {
				log.Println("ERROR: play audio:", err)
			}
		}
	}()

	return ch, nil
}

// playAudio opens an audio output device and plays the given audio data.
func playAudio(ctx context.Context, wavFile io.ReadSeeker, device *portaudio.DeviceInfo) error {
	decoder := wav.NewDecoder(wavFile)
	decoder.ReadInfo()
	if err := decoder.Err(); err != nil {
		return fmt.Errorf("read wave file headers: %w", err)
	}

	if decoder.SampleBitDepth() != 16 {
		return fmt.Errorf("wave data with unsupported bit depth of %d provided, expected 16", decoder.SampleBitDepth())
	}

	audioDuration, err := decoder.Duration()
	if err != nil {
		return fmt.Errorf("get audio duration: %w", err)
	}

	inputBufferSize := 512 * 9
	buffer := audio.IntBuffer{
		Format: &audio.Format{
			SampleRate:  int(decoder.SampleRate),
			NumChannels: int(decoder.NumChans),
		},
		SourceBitDepth: int(decoder.SampleBitDepth()),
		Data:           make([]int, inputBufferSize),
	}
	out := make([]int16, inputBufferSize)

	ratio := float64(decoder.SampleRate) / device.DefaultSampleRate
	resampledOutputBufferSize := int(float64(inputBufferSize) / ratio)
	var resampledOut []int16
	stream, err := portaudio.OpenStream(portaudio.StreamParameters{
		Output: portaudio.StreamDeviceParameters{
			Device:   device,
			Channels: int(decoder.NumChans),
			Latency:  device.DefaultLowOutputLatency,
		},
		SampleRate:      device.DefaultSampleRate,
		FramesPerBuffer: resampledOutputBufferSize,
	}, &resampledOut)
	if err != nil {
		return fmt.Errorf("open audio output stream: %w", err)
	}
	defer stream.Close()

	err = stream.Start()
	if err != nil {
		return fmt.Errorf("start audio output stream: %w", err)
	}
	defer stream.Stop()

	startTime := time.Now()

	for {
		n, err := decoder.PCMBuffer(&buffer)
		if n == 0 {
			break // EOF
		}
		for i, sample := range buffer.Data {
			out[i] = int16(sample)
		}
		if n < inputBufferSize { // zero-pad the buffer after short chunk
			for i := n; i < inputBufferSize; i++ {
				out[i] = 0
			}
		}
		resampledOut = resampleInt16(out, int(decoder.SampleRate), int(device.DefaultSampleRate))
		if err != nil {
			return fmt.Errorf("read chunk from audio stream: %w", err)
		}
		err = stream.Write()
		if err != nil {
			// This happens occasionally for some reason.
			// It doesn't impact the audio playback significantly as long as we're not failing here.
			// TODO: get to the bottom of this and fix it!
			log.Println("WARNING: play audio: write chunk:", err)
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}

	// Wait for the audio to complete playing
	time.Sleep(audioDuration - time.Since(startTime))

	return nil
}
