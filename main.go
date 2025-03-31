package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/mgoltzsche/ai-assistant-vui/internal/audio"
	"github.com/mgoltzsche/ai-assistant-vui/internal/chat"
	"github.com/mgoltzsche/ai-assistant-vui/internal/functions/docker"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/stt"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tts"
	"github.com/mgoltzsche/ai-assistant-vui/internal/vad"
	"github.com/mgoltzsche/ai-assistant-vui/internal/wakeword"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

// Derived from https://github.com/Xbozon/go-whisper-cpp-server-example/tree/main
// and https://github.com/snakers4/silero-vad/blob/master/examples/go/cmd/main.go

func main() {
	configFile := "/etc/ai-assistant-vui/config.yaml"
	cfg, err := config.FromFile(configFile)
	configFlag := &ConfigFlag{File: configFile, Config: &cfg}

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

	err = runAudioPipeline(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
}

func runAudioPipeline(ctx context.Context, cfg config.Configuration) error {
	systemPrompt := fmt.Sprintf(`You are a helpful assistant.
		Your name is %[1]s.
		Keep your responses short and concise.
		You are interacting with the user via STT and TTS technology, meaning the user cannot see but hear your text output.
		When the user tells you to be quiet, you should answer with "Okay".
		However, next time the user says something, you should engage in the conversation again.
		Initially, start the conversation by asking the user how you can help them and explaining that she must say '%[1]s' in order to address you.
		`, cfg.WakeWord)
	//systemPrompt := "Du bist ein hilfreicher Assistent. Antworte kurz, bündig und auf deutsch!"
	conversation := model.NewConversation(systemPrompt)
	functions := &docker.Functions{
		FunctionDefinitions: cfg.Functions,
	}
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
	wakewordFilter := &wakeword.Filter{
		WakeWord: cfg.WakeWord,
	}
	httpClient := &http.Client{Timeout: 90 * time.Second}
	transcriber := &stt.Transcriber{
		Service: &stt.Client{
			URL:    cfg.ServerURL,
			Model:  cfg.STTModel,
			Client: httpClient,
		},
	}
	requester := &chat.Requester{}
	runner := &chat.FunctionRunner{
		Functions: functions,
	}
	chatCompleter := &chat.Completer{
		ServerURL:           cfg.ServerURL,
		Model:               cfg.ChatModel,
		Temperature:         cfg.Temperature,
		FrequencyPenalty:    1.5,
		MaxTokens:           0,
		StripResponsePrefix: fmt.Sprintf("%s:", wakewordFilter.WakeWord),
		HTTPClient:          httpClient,
		Functions:           functions,
	}
	speechGen := &tts.SpeechGenerator{
		Service: &tts.Client{
			URL:    cfg.ServerURL,
			Model:  cfg.TTSModel,
			Client: httpClient,
		},
	}

	go func() {
		<-ctx.Done()
		log.Println("terminating")
	}()

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

	transcriptions := transcriber.Transcribe(ctx, audioInput)
	userRequests := wakewordFilter.FilterByWakeWord(transcriptions)
	completionRequests := requester.AddUserRequestsToConversation(ctx, userRequests, conversation)
	toolResults, toolCallSink := runner.RunFunctionCalls(ctx, conversation)
	completionRequests = chat.MergeCompletionRequests(completionRequests, toolResults)

	responses, err := chatCompleter.ChatCompletion(ctx, completionRequests, conversation, toolCallSink)
	if err != nil {
		return err
	}

	speeches := speechGen.GenerateAudio(ctx, responses, conversation)

	done, err := audioOutput.PlayAudio(ctx, speeches, conversation)
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

type ConfigFlag struct {
	File   string
	Config *config.Configuration
	IsSet  bool
}

func (f *ConfigFlag) Set(path string) error {
	f.File = path

	cfg, err := config.FromFile(path)
	if err != nil {
		return err
	}

	*f.Config = cfg
	f.IsSet = true

	return nil
}

func (f *ConfigFlag) String() string {
	return f.File
}
