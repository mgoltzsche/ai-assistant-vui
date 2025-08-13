package vui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mgoltzsche/ai-assistant-vui/internal/chat"
	//"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
	"github.com/mgoltzsche/ai-assistant-vui/internal/functions/docker"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/soundgen"
	"github.com/mgoltzsche/ai-assistant-vui/internal/stt"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tts"
	"github.com/mgoltzsche/ai-assistant-vui/internal/wakeword"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

type AudioMessage = model.AudioMessage

func AudioPipeline(ctx context.Context, cfg config.Configuration, input <-chan AudioMessage) (<-chan AudioMessage, *model.Conversation, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		cancel()
	}()

	systemPrompt := renderPromptTemplate(strings.Join(cfg.Prompt, "\n"), cfg.WakeWord)
	conversation := model.NewConversation(systemPrompt, 1)
	tools := &docker.Functions{FunctionDefinitions: cfg.Functions}
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
	llm := chat.LLM{
		ServerURL:           cfg.ServerURL,
		APIKey:              cfg.APIKey,
		Model:               cfg.ChatModel,
		Temperature:         cfg.Temperature,
		FrequencyPenalty:    1.5,
		MaxTokens:           0,
		StripResponsePrefix: fmt.Sprintf("%s:", wakewordFilter.WakeWord),
		MaxTurns:            5,
		HTTPClient:          httpClient,
	}
	agents := make([]chat.Agent, len(cfg.Agents))
	for i, a := range cfg.Agents {
		agents[i] = chat.Agent{
			Name:         a.Name,
			Description:  a.Description,
			Tools:        &docker.Functions{FunctionDefinitions: a.Functions},
			SystemPrompt: renderPromptTemplate(strings.Join(a.Prompt, "\n"), cfg.WakeWord),
			LLM:          llm,
		}
	}

	chatCompleter := &chat.Completer{
		LLM:         llm,
		Tools:       tools,
		IntroPrompt: renderPromptTemplate(cfg.IntroPrompt, cfg.WakeWord),
		Agents:      agents,
	}
	/*conversationAgent := &chat.ConversationAgent{
		Completer: chatCompleter,
		//DelegationKeyword: delegationKeyword,
		ToolAgent: chatCompleter,
	}
	conversationAgent.Completer.Functions = functions.Noop()*/
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
	userRequestsConverted := chat.ToAudioMessageStreamWithoutAudioData(userRequests)
	completionRequests, notifications := requester.AddUserRequestsToConversation(ctx, userRequestsConverted, conversation)

	responses, err := chatCompleter.Run(ctx, completionRequests, conversation)
	if err != nil {
		cancel()
		for _ = range responses {
		}
		return nil, nil, err
	}

	notificationSounds, err := soundGen.Notify(notifications, conversation)
	if err != nil {
		cancel()
		for _ = range responses {
		}
		for _ = range notificationSounds {
		}
		return nil, nil, err
	}

	responses = chat.ChunksToSentences(responses)
	speeches := speechGen.GenerateAudio(ctx, responses, conversation)
	audioOutput := chat.MergeChannels(speeches, notificationSounds)

	return audioOutput, conversation, nil
}
