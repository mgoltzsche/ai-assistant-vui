package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

var endOfSentenceRegex = regexp.MustCompile(`(\.|\?|!)+(\s+|$)`)

type ResponseChunk = model.Message

type ChatCompletionRequest struct {
	RequestNum int64
}

type Completer struct {
	ServerURL           string
	Model               string
	Temperature         float64
	FrequencyPenalty    float64
	MaxTokens           int
	StripResponsePrefix string
	HTTPClient          HTTPDoer
	Functions           functions.FunctionProvider
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func (c *Completer) ChatCompletion(ctx context.Context, requests <-chan ChatCompletionRequest, conv *model.Conversation, toolCallSink chan<- ToolCallRequest) (<-chan ResponseChunk, error) {
	llm, err := openai.New(
		openai.WithHTTPClient(c.HTTPClient),
		openai.WithBaseURL(c.ServerURL+"/v1"),
		openai.WithToken("fake"),
		openai.WithModel(c.Model),
	)
	if err != nil {
		return nil, err
	}

	ch := make(chan ResponseChunk, 50)

	go func() {
		defer close(ch)

		fns := functions.NewCallLoopPreventingProvider(functions.Noop())

		err := c.createChatCompletion(ctx, llm, conv.RequestCounter(), fns, conv, toolCallSink, ch)
		if err != nil {
			log.Println("ERROR: chat completion:", err)
		}

		reqNum := int64(-1)

		for req := range requests {
			if req.RequestNum > reqNum {
				fns = functions.NewCallLoopPreventingProvider(c.Functions)
				reqNum = req.RequestNum
			}

			err := c.createChatCompletion(ctx, llm, req.RequestNum, fns, conv, toolCallSink, ch)
			if err != nil {
				log.Println("ERROR: chat completion:", err)

				ch <- ResponseChunk{
					RequestNum: req.RequestNum,
					Text:       fmt.Sprintf("ERROR: Chat completion API request failed: %s", err),
				}
			}
		}
	}()

	return ch, nil
}

func (c *Completer) createChatCompletion(ctx context.Context, llm *openai.LLM, reqNum int64, fns *functions.CallLoopPreventingProvider, conv *model.Conversation, toolCallSink chan<- ToolCallRequest, ch chan<- ResponseChunk) error {
	if conv.RequestCounter() > reqNum {
		return nil // skip outdated request (user requested something else)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	conv.SetCancelFunc(cancel)

	functions, err := fns.Functions()
	if err != nil {
		return fmt.Errorf("get available functions: %w", err)
	}

	llmFunctions := make([]llms.FunctionDefinition, len(functions))
	for i, f := range functions {
		llmFunctions[i] = f.Definition()
	}

	messages := conv.Messages()

	printMessages(messages)

	parser := responseParser{
		Cancel:              cancel,
		ReqNum:              reqNum,
		Conversation:        conv,
		StripResponsePrefix: c.StripResponsePrefix,
		Ch:                  ch,
	}

	// TODO: fix streaming when function support is also enabled.
	// Currently LocalAI does not stream the response when function support is enabled.
	// See https://github.com/mudler/LocalAI/issues/1187
	// While this doesn't break the app, it increases the response latency significantly.

	resp, err := llm.GenerateContent(ctx,
		messages,
		llms.WithStreamingFunc(parser.ConsumeChunk),
		//llms.WithTools(tools),
		llms.WithFunctions(llmFunctions),
		llms.WithTemperature(c.Temperature),
		llms.WithFrequencyPenalty(c.FrequencyPenalty),
		llms.WithMaxTokens(c.MaxTokens),
	)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		return err
	}

	for _, choice := range resp.Choices {
		for _, toolCall := range choice.ToolCalls {
			if toolCall.Type != "function" || toolCall.FunctionCall == nil {
				log.Println("WARNING: ignoring unsupported tool type that was requested by the LLM:", toolCall.Type)
				continue
			}

			call, callID, err := parseFunctionCall(parser.buf.String())
			if err != nil {
				return fmt.Errorf("parse function call: %w", err)
			}

			if call == nil {
				call = toolCall.FunctionCall
			}

			var args map[string]any

			if call.Arguments != "" {
				err = json.Unmarshal([]byte(call.Arguments), &args)
				if err != nil {
					return fmt.Errorf("parse function call arguments: %w", err)
				}
			}

			callAllowed, err := fns.IsFunctionCallAllowed(call.Name, args)
			if err != nil {
				return fmt.Errorf("deduplicate function call: %w", err)
			}

			if !callAllowed {
				// Re-request chat completion without the now banned tool
				return c.createChatCompletion(ctx, llm, reqNum, fns, conv, toolCallSink, ch)
			}

			if conv.AddToolCall(reqNum, callID, *call) {
				func() {
					defer func() {
						recover()
					}()
					if rationale, ok := args["rationale"]; ok && rationale != "" {
						infos := splitIntoSentences(fmt.Sprintf("%v", rationale))
						infos = append(infos, fmt.Sprintf("Let me use my %q tool.", call.Name))

						for _, sentence := range infos {
							ch <- ResponseChunk{
								RequestNum: reqNum,
								Text:       sentence,
								UserOnly:   true,
							}
						}
					}
					toolCallSink <- ToolCallRequest{
						RequestNum: reqNum,
						ToolCallID: callID,
						FunctionCall: FunctionCall{
							Name:      call.Name,
							Arguments: args,
						},
					}
				}()
			}

			return nil
		}
	}

	// TODO: make AI response after function calls work

	parser.Complete()

	return nil
}

var whitespaceRegex = regexp.MustCompile(`\s+`)

func printMessages(messages []llms.MessageContent) {
	log.Println("Requesting chat completion for message history:")
	for i, m := range messages {
		content := model.FormatMessage(m)
		if len(content) > 140 {
			content = content[:140] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")
		content = whitespaceRegex.ReplaceAllString(content, " ")
		log.Printf("  %d. %s", i, content)
	}
}

// parseFunctionCall parses a single function call from multiple function call JSON arrays.
// This is because the LLM genereates responses like this:
//
//	[{"id":"108784cf-5325-4fe9-974f-9bbc0210d457","type":"function","function":{"name":"getCurrentWeather","arguments":""}}]
//	[{"id":"108784cf-5325-4fe9-974f-9bbc0210d457","type":"function","function":{"name":"","arguments":"{\"location\":\"Berlin, DE\",\"rationale\":\"Getting the current weather in the specified location.\",\"unit\":\"celsius\"}"}}]
func parseFunctionCall(content string) (*llms.FunctionCall, string, error) {
	call := llms.FunctionCall{}
	id := ""
	dec := json.NewDecoder(strings.NewReader(content))

	for {
		aiRequests := make([]aiRequest, 0, 1)

		err := dec.Decode(&aiRequests)
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, "", err
		}

		for _, req := range aiRequests {
			if req.Type != "function" || req.Function == nil {
				continue
			}

			if id != "" && id != req.ID {
				break
			}

			id = req.ID

			if name := req.Function.Name; name != "" {
				call.Name = name
			}

			call.Arguments = req.Function.Arguments
		}
	}

	if call.Name != "" {
		return &call, id, nil
	}

	return nil, "", nil
}

type aiRequest struct {
	ID       string        `json:"id"`
	Type     string        `type:"type"`
	Function *functionCall `json:"function,omitempty"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `Json:"arguments"`
}
