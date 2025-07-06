package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
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

			buf, err := readWaveAudio(req.Context(), req.Body)
			if err != nil {
				err = fmt.Errorf("failed to read PCM audio from request body: %w", err)
				log.Println("WARNING:", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			c.Publish(buf)
			return
		}

		sendHTTPAudioStream(c, w, req)
	})
}

func readWaveAudio(ctx context.Context, reader io.Reader) (audio.Buffer, error) {
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

type pubsubChannel interface {
	channel.Publisher
	channel.Subscriber
}

func sendHTTPAudioStream(c pubsubChannel, w http.ResponseWriter, req *http.Request) {
	var err error
	var writer io.Writer = w

	raw := req.Header.Get("Accept") == "audio/x-raw"

	// The client-side buffer duration is used to pad the stream with zeros after a speech.
	// This is to make the client play the speech immediately
	// instead of delaying playback until there is more data.
	bufferDurationMs := uint64(1250)
	bufferDurationMsStr := req.URL.Query().Get("buffer-ms")
	if bufferDurationMsStr != "" {
		bufferDurationMs, err = strconv.ParseUint(bufferDurationMsStr, 10, 32)
		if err != nil {
			err = fmt.Errorf("invalid X-Buffer-Duration-Ms header value provided: %w", err)
			log.Println("WARNING:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if strings.Contains(req.Header.Get("Connection"), "Upgrade") {
		log.Println("Accepting websocket connection")
		conn, err := websocket.Accept(w, req, nil)
		if err != nil {
			err = fmt.Errorf("accept websocket connection: %w", err)
			log.Println("WARNING:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.CloseNow()

		go func() {
			err := readAudioFromWebsocket(req.Context(), conn, c)
			if err != nil {
				log.Println("ERROR: read websocket audio:", err)
				return
			}
		}()

		writer = &websocketWriter{
			Ctx:       req.Context(),
			Websocket: conn,
		}

		raw = true
	} else {
		h := w.Header()

		if raw {
			h.Set("Content-Type", "audio/x-raw;rate=16000;bits=16;channels=1;encoding=signed-int;big-endian=false")
		} else {
			h.Set("Content-Type", "audio/wav")
		}

		h.Set("Content-Type", "audio/wav")
		h.Set("Transfer-Encoding", "chunked")
		h.Set("X-Accel-Buffering", "no") // tell reverse proxy not to buffer

		// Set headers to prevent the client from caching
		h.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		h.Set("Pragma", "no-cache")
		h.Set("Expires", "0")

		w.WriteHeader(http.StatusOK)
	}

	err = streamAudio(req.Context(), c, raw, bufferDurationMs, writer)
	if err != nil {
		log.Println("WARNING: failed to stream audio:", err)
	}
}

func readAudioFromWebsocket(ctx context.Context, conn *websocket.Conn, out channel.Publisher) error {
	for {
		conn.SetReadLimit(-1)
		msgType, reader, err := conn.Reader(ctx)
		if err != nil {
			return fmt.Errorf("read websocket message: %w", err)
		}

		if msgType != websocket.MessageBinary {
			return fmt.Errorf("invalid message type %q received", msgType)
		}

		//buf, err := readRawPCMStream(ctx, reader)
		buf, err := readWaveAudio(ctx, reader)
		if err != nil {
			return fmt.Errorf("read websocket audio message: %w", err)
		}

		out.Publish(buf)
	}
}

/*
func writeWavFile(buf audio.Buffer) error {
	file, err := os.OpenFile("/output/record.wav", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("ERROR: open target wav file: %s\n", err)
		return fmt.Errorf("open target wav file: %w", err)
	}
	defer file.Close()
	//encoder := wav.NewEncoder(file, buf.Format.SampleRate, buf.SourceBitDepth, buf.Format.NumChannels, 1)
	encoder := wav.NewEncoder(file, 16000, 16, 1, 1)
	if err := encoder.Write(buf.AsIntBuffer()); err != nil {
		return fmt.Errorf("write buffer to wav file: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("encoder close: %w", err)
	}
	return nil
}

func readRawPCMStream(ctx context.Context, reader io.Reader) (*audio.IntBuffer, error) {
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read audio: %w", err)
	}

	os.WriteFile("/output/record.pcm", b, 0644)

	samples := make([]int, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(b[i : i+2]))
		//_, err := binary.Decode([]byte{b[i]}, binary.LittleEndian, &v)
		//if err != nil {
		//	return nil, fmt.Errorf("decode audio: %w", err)
		//}
		samples = append(samples, int(sample))
	}

	return &audio.IntBuffer{
		Format: &audio.Format{
			SampleRate:  16000,
			NumChannels: 1,
		},
		SourceBitDepth: 16,
		Data:           samples,
	}, nil
}*/

func streamAudio(ctx context.Context, c channel.Subscriber, raw bool, bufferDurationMs uint64, w io.Writer) error {
	s := c.Subscribe(ctx)
	defer s.Stop()

	if !raw {
		err := sendWavStreamHeader(w)
		if err != nil {
			return fmt.Errorf("writing wav header: %w", err)
		}
	}

	ch := s.ResultChan()
	flushed := false
	var err error

	for {
		if flushed {
			select {
			case msg, ok := <-ch:
				if !ok {
					break
				}

				err = copyAudio(ctx, w, bytes.NewReader(msg.WaveData))
				if err != nil {
					return fmt.Errorf("copy audio into stream: %w", err)
				}
			case <-time.After(50 * time.Second):
				err = sendKeepAliveSample(w)
				if err != nil {
					return fmt.Errorf("send keep-alive sample: %w", err)
				}

				continue
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
			err = forceFlush(w, bufferDurationMs)
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

func sendKeepAliveSample(w io.Writer) error {
	n, err := w.Write([]byte{0, 0})
	if err != nil {
		return err
	}

	if n != 2 {
		return io.ErrShortWrite
	}

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	return nil
}

func forceFlush(w io.Writer, clientBufferDurationMs uint64) error {
	buffer := audio.IntBuffer{
		Format: &audio.Format{
			SampleRate:  16000,
			NumChannels: 1,
		},
		SourceBitDepth: 16,
	}

	// Add padding to fill the client buffer to make it play immediately
	buffer.Data = make([]int, buffer.Format.SampleRate*int(clientBufferDurationMs)/1000)
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
		Data:           []int{},
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
