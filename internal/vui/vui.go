package vui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mgoltzsche/ai-assistant-vui/internal/chat"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/stt"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tools/mcp"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tts"
	"github.com/mgoltzsche/ai-assistant-vui/internal/wakeword"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

type AudioMessage = model.AudioMessage
type Event = model.Event

func AudioPipeline(ctx context.Context, cfg config.Configuration, mcpServers mcp.Servers, input <-chan AudioMessage) (<-chan Event, *model.Conversation, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		cancel()
	}()

	systemPrompt := renderPromptTemplate(strings.Join(cfg.Prompt, "\n"), cfg.WakeWord)
	conversation := model.NewConversation(1)
	tools, err := mcp.ToolProvider(mcpServers, cfg.Tools)
	if err != nil {
		return nil, nil, fmt.Errorf("init main tools: %w", err)
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
	requester := &chat.Requester{
		NotificationSampleRate: 16000,
	}
	llmConfig := chat.LLMConfig{
		ServerURL:  cfg.ServerURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.ChatModel,
		HTTPClient: httpClient,
	}
	llm := chat.LLM{
		LLMConfig:           llmConfig,
		Temperature:         cfg.Temperature,
		FrequencyPenalty:    1.5,
		MaxTokens:           0,
		StripResponsePrefix: fmt.Sprintf("%s:", wakewordFilter.WakeWord),
		MaxTurns:            5,
	}
	agents := make([]chat.Agent, len(cfg.Agents))
	for i, a := range cfg.Agents {
		agentTools, err := mcp.ToolProvider(mcpServers, a.Tools)
		if err != nil {
			return nil, nil, fmt.Errorf("init %s agent tools: %w", a.Name, err)
		}

		agents[i] = chat.Agent{
			Name:         a.Name,
			Description:  a.Description,
			Tools:        agentTools,
			SystemPrompt: renderPromptTemplate(strings.Join(a.Prompt, "\n"), cfg.WakeWord),
			LLM:          llm,
		}
	}

	chatCompleter := &chat.Completer{
		LLM: llm,
		Compaction: &chat.ConversationCompaction{
			LLMConfig: llmConfig,
		},
		Tools:        tools,
		SystemPrompt: systemPrompt,
		IntroPrompt:  renderPromptTemplate(cfg.IntroPrompt, cfg.WakeWord),
		Agents:       agents,
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
			Voice:  cfg.TTSVoice,
			Client: httpClient,
		},
	}

	audioInput, textInput := splitChannel(input, func(msg AudioMessage) bool { return len(msg.WaveData) > 0 })
	transcriptions := transcriber.Transcribe(ctx, audioInput)
	userRequests := wakewordFilter.FilterByWakeWord(transcriptions)
	userRequestsConverted := chat.ToAudioMessageStreamWithoutAudioData(userRequests)
	userRequestsConverted = mergeChannels(userRequestsConverted, textInput)
	completionRequests, feedbackMessages := requester.AddUserRequestsToConversation(ctx, userRequestsConverted, conversation)
	responses, err := chatCompleter.Run(ctx, completionRequests, conversation)
	if err != nil {
		cancel()
		readAll(responses)
		readAll(feedbackMessages)
		return nil, nil, err
	}

	responses = chat.ChunksToSentences(responses)
	speeches := speechGen.GenerateAudio(ctx, responses, conversation)
	resultEvents := assistantEvents(speeches)
	events := mergeChannels(feedbackMessages, resultEvents)

	return events, conversation, nil
}

func assistantEvents(msgs <-chan AudioMessage) <-chan Event {
	ch := make(chan Event)

	go func() {
		defer close(ch)

		for msg := range msgs {
			ch <- Event{
				Role:         model.RoleAssistant,
				AudioMessage: msg,
			}
		}
	}()

	return ch
}

func readAll[T any](ch <-chan T) {
	for _ = range ch {
	}
}
