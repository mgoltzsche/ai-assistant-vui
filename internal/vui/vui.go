package vui

import (
	"context"
	"fmt"
	"net/http"
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

	//delegationKeyword := "D3LEG4TE"
	systemPrompt := fmt.Sprintf(`You are a helpful assistant.
		Your name is %[1]s.
		Keep your responses short and concise.
		You are interacting with the user via STT and TTS technology, meaning the user cannot see but hear your text output.
		When the user tells you to be quiet, you should answer with "Okay".
		However, next time the user says something, you should engage in the conversation again.
		Initially, start the conversation by asking the user how you can help them and explaining that she must say '%[1]s' in order to address you.
		`, cfg.WakeWord /*, delegationKeyword*/)
	//You must speak to the user using the 'say' function. Before and after calling any other function, call the 'say' function to tell the user what you're doing!
	//In case the user asks you to use an external tool, confirm the action by saying e.g. 'Okay.'.
	//In case the user asks you to use an external tool, provide a short confirming response such as 'Okay.' followed by '%[2]s' on a new line followed by a prompt for another AI with tool access to finish the response.
	//You can access external tools as well as the internet by returning a prompt after an intermediate response as quick user feedback, separated by '%[2]s' on a new line.
	//To use an external tool or access the internet, end your response with a new line followed by '%[2]s' followed by the prompt that will fed into another AI with tool access to answer the user request on your behalf.
	//systemPrompt := "Du bist ein hilfreicher Assistent. Antworte kurz, bündig und auf deutsch!"
	conversation := model.NewConversation(systemPrompt)
	tools := &docker.Functions{
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
	chatCompleter := chat.Completer{
		ServerURL:              cfg.ServerURL,
		APIKey:                 cfg.APIKey,
		Model:                  cfg.ChatModel,
		Temperature:            cfg.Temperature,
		FrequencyPenalty:       1.5,
		MaxTokens:              0,
		StripResponsePrefix:    fmt.Sprintf("%s:", wakewordFilter.WakeWord),
		MaxTurns:               5,
		MaxConcurrentToolCalls: 5,
		HTTPClient:             httpClient,
		Functions:              tools,
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
