package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/mgoltzsche/ai-assistant-vui/internal/audio"
	"github.com/mgoltzsche/ai-assistant-vui/internal/vad"
	"github.com/mgoltzsche/ai-assistant-vui/internal/vui"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

// Derived from https://github.com/Xbozon/go-whisper-cpp-server-example/tree/main
// and https://github.com/snakers4/silero-vad/blob/master/examples/go/cmd/main.go

func main() {
	configFile := "/etc/ai-assistant-vui/config.yaml"
	cfg, err := config.FromFile(configFile)
	configFlag := &config.Flag{File: configFile, Config: &cfg}

	flag.Var(configFlag, "config", "Path to the configuration file")
	flag.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "URL pointing to the OpenAI API server that runs the LLM")
	flag.StringVar(&cfg.InputDevice, "input-device", cfg.InputDevice, "name or ID or the audio input device")
	flag.StringVar(&cfg.OutputDevice, "output-device", cfg.OutputDevice, "name or ID or the audio output device")
	flag.IntVar(&cfg.MinVolume, "min-volume", cfg.MinVolume, "min input volume threshold")
	flag.BoolVar(&cfg.VADEnabled, "vad", cfg.VADEnabled, "enable voice activity detection (VAD)")
	flag.StringVar(&cfg.VADModelPath, "vad-model", cfg.VADModelPath, "path to the VAD model")
	flag.StringVar(&cfg.STTModel, "stt-model", cfg.STTModel, "name of the STT model to use")
	flag.StringVar(&cfg.TTSModel, "tts-model", cfg.TTSModel, "name of the TTS model to use")
	flag.StringVar(&cfg.ChatModel, "chat-model", cfg.ChatModel, "name of the chat model to use")
	flag.Float64Var(&cfg.Temperature, "temperature", cfg.Temperature, "temperature parameter for the chat LLM")
	flag.StringVar(&cfg.WakeWord, "wake-word", cfg.WakeWord, "word used to address the assistent (needs to be recognized by whisper)")
	flag.Parse()

	if !configFlag.IsSet && err != nil {
		log.Fatal(err)
	}

	portaudio.Initialize()
	defer portaudio.Terminate()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("terminating")
	}()

	err = runAudioPipeline(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
}

func runAudioPipeline(ctx context.Context, cfg config.Configuration) error {
	audioDevice := &audio.Input{
		Device:      cfg.InputDevice,
		SampleRate:  16000,
		Channels:    1,
		MinVolume:   cfg.MinVolume,
		MinDelay:    time.Second,
		MaxDuration: time.Second * 25,
	}
	audioOutput := &audio.Output{
		Device: cfg.OutputDevice,
	}
	detector := &vad.Detector{
		ModelPath: cfg.VADModelPath,
	}

	audioInput, err := audioDevice.RecordAudio(ctx)
	if err != nil {
		return err
	}

	if cfg.VADEnabled {
		audioInput, err = detector.DetectVoiceActivity(audioInput)
		if err != nil {
			return err
		}
	}

	wavAudioInput := audio.AudioBuffersToRiffWavs(audioInput)

	playbackRequests, conversation, err := vui.AudioPipeline(ctx, cfg, wavAudioInput)
	if err != nil {
		return err
	}

	done, err := audioOutput.PlayAudio(ctx, playbackRequests, conversation)
	if err != nil {
		return err
	}

	<-done

	return nil
}

/*func writeWavFile(buffer []int16, sampleRate, channels int) error {
	file, err := os.OpenFile("/output/record.wav", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("ERROR: open target wav file: %s\n", err)
		return fmt.Errorf("open target wav file: %w", err)
	}
	defer file.Close()
	soundIntBuffer := &audio.IntBuffer{
		Format: &audio.Format{SampleRate: sampleRate, NumChannels: channels},
		Data:   convert.Int16ToInt(buffer),
	}
	encoder := wav.NewEncoder(file, 16000, 16, 1, 1)
	if err := encoder.Write(soundIntBuffer); err != nil {
		return fmt.Errorf("write buffer to wav file: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("encoder close: %w", err)
	}
	return nil
}*/
