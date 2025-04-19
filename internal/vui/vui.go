package vui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-audio/audio"
	"github.com/mgoltzsche/ai-assistant-vui/internal/chat"
	"github.com/mgoltzsche/ai-assistant-vui/internal/functions/docker"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/soundgen"
	"github.com/mgoltzsche/ai-assistant-vui/internal/stt"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tts"
	"github.com/mgoltzsche/ai-assistant-vui/internal/wakeword"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

type AudioMessage = model.AudioMessage

func AudioPipeline(ctx context.Context, cfg config.Configuration, input <-chan audio.Buffer) (<-chan AudioMessage, *model.Conversation, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		cancel()
	}()

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
	soundGen := &soundgen.Generator{
		SampleRate: 16000,
	}

	transcriptions := transcriber.Transcribe(ctx, input)
	userRequests := wakewordFilter.FilterByWakeWord(transcriptions)
	notificationSounds, notificationSink, err := soundGen.Notify(conversation)
	if err != nil {
		close(notificationSink)
		return nil, nil, err
	}

	completionRequests := requester.AddUserRequestsToConversation(ctx, userRequests, notificationSink, conversation)
	toolResults, toolCallSink := runner.RunFunctionCalls(ctx, conversation)
	completionRequests = chat.MergeChannels(completionRequests, toolResults)

	responses, err := chatCompleter.ChatCompletion(ctx, completionRequests, conversation, toolCallSink)
	if err != nil {
		close(notificationSink)
		close(toolCallSink)
		cancel()
		for _ = range responses {
		}
		return nil, nil, err
	}

	speeches := speechGen.GenerateAudio(ctx, responses, conversation)
	audioOutput := chat.MergeChannels(speeches, notificationSounds)

	return audioOutput, conversation, nil
}
