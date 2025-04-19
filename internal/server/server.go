package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/mgoltzsche/ai-assistant-vui/internal/channel"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
	"github.com/orcaman/writerseeker"
)

func AddRoutes(ctx context.Context, cfg config.Configuration, webDir string, mux *http.ServeMux) {
	channels := channel.NewChannels(ctx, cfg)

	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	mux.HandleFunc("/channels/{channelId}/audio", func(w http.ResponseWriter, req *http.Request) {
		channelId := req.PathValue("channelId")

		c, err := channels.GetOrCreate(channelId)
		if err != nil {
			log.Println("ERROR:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		switch req.Method {
		case http.MethodPost:
			defer req.Body.Close()

			buf, err := readAudio(req.Context(), req.Body)
			if err != nil {
				err = fmt.Errorf("failed to read PCM audio from request body: %w", err)
				log.Println("WARNING:", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			c.Publish(buf)
		case http.MethodGet:
			h := w.Header()

			//h.Set("Content-Type", "audio/pcm;rate=16000;bits=16;channels=1;encoding=signed-int;big-endian=false")
			h.Set("Content-Type", "audio/wav")
			h.Set("Transfer-Encoding", "chunked")
			h.Set("Cache-Control", "no-cache, no-store, must-revalidate")
			h.Set("Pragma", "no-cache")
			h.Set("Expires", "0")
			w.WriteHeader(http.StatusOK)

			err = streamAudio(req.Context(), c, w)
			if err != nil {
				log.Println("WARNING: failed to stream audio:", err)
			}
		default:
			http.Error(w, "Unsupported HTTP request method. Supported methods: GET, POST", http.StatusBadRequest)
		}
	})
}

func readAudio(ctx context.Context, reader io.Reader) (audio.Buffer, error) {
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	decoder := wav.NewDecoder(bytes.NewReader(b))

	decoder.ReadInfo()
	if err := decoder.Err(); err != nil {
		return nil, fmt.Errorf("read wave file headers: %w", err)
	}

	if decoder.SampleBitDepth() != 16 {
		return nil, fmt.Errorf("wave data with unsupported bit depth of %d provided, expected 16", decoder.SampleBitDepth())
	}

	// TODO: don't read full audio into memory but stream chunk-wise into transcription API
	buffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, fmt.Errorf("read full pcm buffer: %w", err)
	}

	return buffer, nil
}

func streamAudio(ctx context.Context, c channel.Subscriber, w io.Writer) error {
	s := c.Subscribe(ctx)
	defer s.Stop()

	err := sendWavStreamHeader(w)
	if err != nil {
		return fmt.Errorf("writing wav header: %w", err)
	}

	ch := s.ResultChan()
	flushed := false

	for {
		if flushed {
			msg, ok := <-ch
			if !ok {
				break
			}

			err = copyAudio(ctx, w, bytes.NewReader(msg.WaveData))
			if err != nil {
				return fmt.Errorf("copy audio into stream: %w", err)
			}

			flushed = false

			continue
		}

		select {
		case msg, ok := <-ch:
			if !ok {
				break
			}

			err = copyAudio(ctx, w, bytes.NewReader(msg.WaveData))
			if err != nil {
				return fmt.Errorf("copy audio into stream: %w", err)
			}
		case <-time.After(50 * time.Millisecond):
			err = forceFlush(w)
			if err != nil {
				return fmt.Errorf("force flush audio stream: %w", err)
			}

			flushed = true
		}
	}

	if ctx.Err() != nil {
		log.Println("WARNING: request was cancelled")
	}

	return nil
}

func sendWavStreamHeader(w io.Writer) error {
	wavHeader, err := generateWavHeader(16000, 16, 1)
	if err != nil {
		return fmt.Errorf("read PCM audio from request body: %w", err)
	}

	_, err = io.Copy(w, bytes.NewReader(wavHeader))

	return err
}

func copyAudio(ctx context.Context, w io.Writer, reader io.ReadSeeker) error {
	decoder := wav.NewDecoder(reader)

	decoder.ReadInfo()
	if err := decoder.Err(); err != nil {
		return fmt.Errorf("read wave file headers: %w", err)
	}

	if decoder.SampleBitDepth() != 16 {
		return fmt.Errorf("wave data with unsupported bit depth of %d provided, expected 16", decoder.SampleBitDepth())
	}

	inputBufferSize := 512 * 9
	data := make([]int, inputBufferSize)
	buffer := audio.IntBuffer{
		Format: &audio.Format{
			SampleRate:  int(decoder.SampleRate),
			NumChannels: int(decoder.NumChans),
		},
		SourceBitDepth: int(decoder.SampleBitDepth()),
		Data:           data,
	}

	totalSize := 0

	for {
		n, err := decoder.PCMBuffer(&buffer)
		if n == 0 {
			break // EOF
		}

		totalSize += n

		if err != nil {
			return fmt.Errorf("read pcm buffer: %w", err)
		}

		if n < inputBufferSize {
			buffer.Data = buffer.Data[:n]
		}

		err = writePCMStream(&buffer, w)
		if err != nil {
			return fmt.Errorf("copy pcm data: %w", err)
		}

		buffer.Data = data

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	// Add zeros to fill client buffer to force it to play immediately
	/*if pad := inputBufferSize-totalSize%inputBufferSize; pad < inputBufferSize {
		buffer.Data = make([]int, 3*inputBufferSize+pad)
		err := writePCMStream(&buffer, w)
		if err != nil {
			return fmt.Errorf("write padding pcm data: %w", err)
		}
	}*/

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	return nil
}

func forceFlush(w io.Writer) error {
	buffer := audio.IntBuffer{
		Format: &audio.Format{
			SampleRate:  16000,
			NumChannels: 1,
		},
		SourceBitDepth: 16,
	}

	// Add padding to fill the client buffer to make it play immediately
	padMs := 1200
	buffer.Data = make([]int, buffer.Format.SampleRate*padMs/1000)
	//inputBufferSize := 512 * 9
	//buffer.Data = make([]int, 4*inputBufferSize)
	err := writePCMStream(&buffer, w)
	if err != nil {
		return fmt.Errorf("write padding pcm data: %w", err)
	}

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	return nil
}

func writePCMStream(src *audio.IntBuffer, w io.Writer) error {
	var buf = bytes.NewBuffer(make([]byte, 0, len(src.Data)*binary.MaxVarintLen16))
	var err error

	if src.SourceBitDepth != 16 {
		return fmt.Errorf("supports 16 bit input audio but was %d", src.SourceBitDepth)
	}

	for _, sample := range src.Data {
		if err = binary.Write(buf, binary.LittleEndian, int16(sample)); err != nil {
			return err
		}
	}

	b := buf.Bytes()

	n, err := w.Write(b)
	if err != nil {
		return err
	}

	if n != len(b) {
		return io.ErrShortWrite
	}

	return nil
}

func generateWavHeader(sampleRate, bitDepth, channels int) ([]byte, error) {
	wavFile := &writerseeker.WriterSeeker{}
	encoder := wav.NewEncoder(wavFile, sampleRate, bitDepth, channels, 1)

	buffer := &audio.IntBuffer{
		Format:         &audio.Format{SampleRate: sampleRate, NumChannels: channels},
		SourceBitDepth: bitDepth,
		Data:           []int{}, // To generate headers with length 0 to make streaming work
	}

	if err := encoder.Write(buffer); err != nil {
		return nil, fmt.Errorf("encoder write buffer: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encoder close: %w", err)
	}

	riffWav, err := io.ReadAll(wavFile.Reader())
	if err != nil {
		return nil, fmt.Errorf("reading wav into memory: %w", err)
	}

	// Set WAVE data size to the maximum value in order to make streaming work
	headerBuf := bytes.NewBuffer(riffWav[:40])
	err = binary.Write(headerBuf, binary.LittleEndian, uint32(0xFFFFFFFF))
	if err != nil {
		return nil, fmt.Errorf("write wav data length: %w", err)
	}

	return headerBuf.Bytes(), nil
}
