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
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/stt"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tts"
	"github.com/mgoltzsche/ai-assistant-vui/internal/vad"
	"github.com/mgoltzsche/ai-assistant-vui/internal/wakeword"
)

// Derived from https://github.com/Xbozon/go-whisper-cpp-server-example/tree/main
// and https://github.com/snakers4/silero-vad/blob/master/examples/go/cmd/main.go

type Options struct {
	ServerURL    string
	InputDevice  string
	OutputDevice string
	MinVolume    int
	VADEnabled   bool
	VADModelPath string
	STTModel     string
	TTSModel     string
	ChatModel    string
	Temperature  float64
	WakeWord     string
}

func main() {
	opts := Options{
		ServerURL:    "http://localhost:8080",
		MinVolume:    450,
		VADEnabled:   true,
		VADModelPath: "/models/silero_vad.onnx",
		STTModel:     "whisper-1",
		//ChatModel:    "llama-3-sauerkrautlm-8b-instruct",
		ChatModel: "LocalAI-llama3-8b-function-call-v0.2",
		//ChatModel:    "llama-3-8b-lexifun-uncensored-v1",
		//ChatModel: "mistral-7b-instruct-v0.3",
		TTSModel: "voice-en-us-amy-low",
		//TTSModel:    "voice-de-kerstin-low",
		Temperature: 0.7,
		WakeWord:    "Computer",
	}

	flag.StringVar(&opts.ServerURL, "server-url", opts.ServerURL, "URL pointing to the OpenAI API server that runs the LLM")
	flag.StringVar(&opts.InputDevice, "input-device", opts.InputDevice, "name or ID or the audio input device")
	flag.StringVar(&opts.OutputDevice, "output-device", opts.OutputDevice, "name or ID or the audio output device")
	flag.IntVar(&opts.MinVolume, "min-volume", opts.MinVolume, "min input volume threshold")
	flag.BoolVar(&opts.VADEnabled, "vad", opts.VADEnabled, "enable voice activity detection (VAD)")
	flag.StringVar(&opts.VADModelPath, "vad-model", opts.VADModelPath, "path to the VAD model")
	flag.StringVar(&opts.STTModel, "stt-model", opts.STTModel, "name of the STT model to use")
	flag.StringVar(&opts.TTSModel, "tts-model", opts.TTSModel, "name of the TTS model to use")
	flag.StringVar(&opts.ChatModel, "chat-model", opts.ChatModel, "name of the chat model to use")
	flag.Float64Var(&opts.Temperature, "temperature", opts.Temperature, "temperature parameter for the chat LLM")
	flag.StringVar(&opts.WakeWord, "wake-word", opts.WakeWord, "word used to address the assistent (needs to be recognized by whisper)")
	flag.Parse()

	portaudio.Initialize()
	defer portaudio.Terminate()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := runAudioPipeline(ctx, opts)
	if err != nil {
		log.Fatal(err)
	}
}

func runAudioPipeline(ctx context.Context, opts Options) error {
	audioDevice := &audio.Input{
		Device:      opts.InputDevice,
		SampleRate:  16000,
		Channels:    1,
		MinVolume:   opts.MinVolume,
		MinDelay:    time.Second,
		MaxDuration: time.Second * 25,
	}
	audioOutput := &audio.Output{
		Device: opts.OutputDevice,
	}
	detector := &vad.Detector{
		ModelPath: opts.VADModelPath,
	}
	wakewordFilter := &wakeword.Filter{
		WakeWord: opts.WakeWord,
		SystemPrompt: fmt.Sprintf(`You are a helpful assistant.
		Your name is %[1]s.
		Keep your responses short and concise.
		You are interacting with the user via STT and TTS technology, meaning the user cannot see but hear your text output.
		When the user tells you to be quiet, you should answer with "Okay".
		However, next time the user says something, you should engage in the conversation again.
		Initially, start the conversation by asking the user how you can help them and explaining that she must say '%[1]s' in order to address you.
		`, opts.WakeWord),
		//SystemPrompt: "Du bist ein hilfreicher Assistent. Antworte kurz, bündig und auf deutsch!",
	}
	httpClient := &http.Client{Timeout: 45 * time.Second}
	transcriber := &stt.Transcriber{
		Service: &stt.Client{
			URL:    opts.ServerURL,
			Model:  opts.STTModel,
			Client: httpClient,
		},
	}
	requester := &chat.Requester{}
	runner := &chat.FunctionRunner{}
	chatCompleter := &chat.Completer{
		ServerURL:           opts.ServerURL,
		Model:               opts.ChatModel,
		Temperature:         opts.Temperature,
		FrequencyPenalty:    1.5,
		MaxTokens:           0,
		StripResponsePrefix: fmt.Sprintf("%s:", wakewordFilter.WakeWord),
		HTTPClient:          httpClient,
	}
	speechGen := &tts.SpeechGenerator{
		Service: &tts.Client{
			URL:    opts.ServerURL,
			Model:  opts.TTSModel,
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

	if opts.VADEnabled {
		audioInput, err = detector.DetectVoiceActivity(audioInput)
		if err != nil {
			return err
		}
	}

	transcriptions := transcriber.Transcribe(ctx, audioInput)

	userRequests := transcriptions2requests(transcriptions)
	userRequests, conversation := wakewordFilter.FilterByWakeWord(userRequests)

	completionRequests := requester.AddUserRequestsToConversation(ctx, userRequests, conversation)
	toolResults, toolCallSink := runner.RunFunctionCalls(ctx, conversation)
	completionRequests = chat.MergeCompletionRequests(completionRequests, toolResults)

	responses, err := chatCompleter.RunChatCompletions(ctx, completionRequests, conversation, toolCallSink)
	if err != nil {
		return err
	}

	speeches := speechGen.GenerateAudio(ctx, completions2ttsrequests(responses))

	played, err := audioOutput.PlayAudio(ctx, speeches2playrequests(speeches), conversation)
	if err != nil {
		return err
	}

	<-chatCompleter.AddResponsesToConversation(played, conversation)

	return nil
}

func transcriptions2requests(transcriptions <-chan stt.Transcription) <-chan model.Request {
	ch := make(chan model.Request, 10)

	go func() {
		defer close(ch)

		for transcription := range transcriptions {
			ch <- model.Request{
				Text: transcription.Text,
			}
		}
	}()

	return ch
}

func completions2ttsrequests(responses <-chan model.ResponseChunk) <-chan tts.Request {
	ch := make(chan tts.Request, 10)

	go func() {
		defer close(ch)

		for resp := range responses {
			ch <- tts.Request{
				ID:   resp.RequestID,
				Text: resp.Text,
			}
		}
	}()

	return ch
}

func speeches2playrequests(speeches <-chan tts.GeneratedSpeech) <-chan audio.PlayRequest {
	ch := make(chan audio.PlayRequest, 5)

	go func() {
		defer close(ch)

		for speech := range speeches {
			ch <- audio.PlayRequest{
				RequestID: speech.RequestID,
				Text:      speech.Text,
				WaveData:  speech.WaveData,
			}
		}
	}()

	return ch
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
