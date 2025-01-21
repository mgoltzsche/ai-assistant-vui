package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/jsonschema"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type ChatCompletionRequest struct {
	RequestID int64
}

type Completer2 struct {
	ServerURL           string
	Model               string
	Temperature         float64
	FrequencyPenalty    float64
	MaxTokens           int
	StripResponsePrefix string
	HTTPClient          HTTPDoer
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func (c *Completer2) RunChatCompletions(ctx context.Context, requests <-chan ChatCompletionRequest, conv *model.Conversation, toolCallSink chan<- ToolCallRequest) (<-chan model.ResponseChunk, error) {
	llm, err := openai.New(
		openai.WithHTTPClient(c.HTTPClient),
		openai.WithBaseURL(c.ServerURL+"/v1"),
		openai.WithToken("fake"),
		openai.WithModel(c.Model),
	)
	if err != nil {
		return nil, err
	}

	ch := make(chan model.ResponseChunk, 50)

	go func() {
		defer close(ch)

		err := c.createChatCompletion(ctx, llm, 0, conv, toolCallSink, ch)
		if err != nil {
			log.Println("ERROR: chat completion:", err)
		}

		for req := range requests {
			if conv.RequestCounter() > req.RequestID {
				continue // skip outdated request (user requested something else)
			}

			err := c.createChatCompletion(ctx, llm, req.RequestID, conv, toolCallSink, ch)
			if err != nil {
				log.Println("ERROR: chat completion:", err)
			}
		}
	}()

	return ch, nil
}

func (c *Completer2) createChatCompletion(ctx context.Context, llm *openai.LLM, reqID int64, conv *model.Conversation, toolCallSink chan<- ToolCallRequest, ch chan<- model.ResponseChunk) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var buf bytes.Buffer

	resp, err := llm.GenerateContent(ctx,
		conv.Messages(),
		llms.WithStreamingFunc(streamFunc(cancel, reqID, conv, &buf, ch)),
		//llms.WithTools(tools),
		llms.WithFunctions(functions),
		//llms.WithFunctionCallBehavior(llms.FunctionCallBehaviorAuto),
		llms.WithTemperature(c.Temperature),
		llms.WithFrequencyPenalty(c.FrequencyPenalty),
		llms.WithN(c.MaxTokens),
	)
	if err != nil {
		return err
	}

	for _, c := range resp.Choices {
		for _, toolCall := range c.ToolCalls {
			if toolCall.Type != "function" || toolCall.FunctionCall == nil {
				log.Println("WARNING: ignoring unsupported tool type that was requested by the LLM:", toolCall.Type)
				continue
			}

			call, callID, err := parseFunctionCall(buf.String())
			if err != nil {
				return fmt.Errorf("parse function call: %w", err)
			}

			if call == nil {
				call = toolCall.FunctionCall
			}

			if call.Arguments == "" {
				return fmt.Errorf("function %q called with empty arguments", call.Name)
			}

			var args map[string]any

			err = json.Unmarshal([]byte(call.Arguments), &args)
			if err != nil {
				return fmt.Errorf("parse function call arguments: %w", err)
			}

			conv.AddMessage(llms.MessageContent{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{llms.ToolCall{
					ID:           callID,
					Type:         toolCall.Type,
					FunctionCall: call,
				}},
			})

			func() {
				defer func() {
					recover()
				}()
				toolCallSink <- ToolCallRequest{
					RequestID: reqID,
					FunctionCall: FunctionCall{
						Name:      call.Name,
						Arguments: args,
					},
				}
			}()

			return nil
		}
	}

	if buf.Len() > 0 {
		ch <- model.ResponseChunk{
			RequestID: reqID,
			Text:      strings.TrimSuffix(buf.String(), "</s>"),
		}
	}

	return nil
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

func streamFunc(cancel context.CancelFunc, reqID int64, conv *model.Conversation, buf *bytes.Buffer, ch chan<- model.ResponseChunk) func(ctx context.Context, chunk []byte) error {
	lastContent := ""
	lastSentence := ""

	return func(ctx context.Context, chunk []byte) error {
		if conv.RequestCounter() > reqID {
			// Cancel response stream if request is outdated (user requested something else)
			cancel()
			return nil
		}

		content := string(chunk)

		buf.WriteString(content)

		// TODO: don't emit separate event for numbered list items, e.g. 3. ?!
		if buf.Len() > len(content)+1 && (content == "\n" || content == " " && (lastContent == "." || lastContent == "!" || lastContent == "?")) {
			sentence := buf.String()

			if strings.TrimSpace(sentence) != "" {
				if sentence == lastSentence {
					// Cancel response stream if last sentence was repeated
					cancel()
					return nil
				}

				lastSentence = sentence

				ch <- model.ResponseChunk{
					RequestID: reqID,
					Text:      sentence,
				}
			}

			buf.Reset()
		}

		lastContent = content

		return nil
	}
}

var functions = []llms.FunctionDefinition{
	{
		Name:        "getCurrentWeather",
		Description: "Get the current weather in a given location",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"rationale": {
					Type:        jsonschema.String,
					Description: "The rationale for choosing this function call with these parameters",
				},
				"location": {
					Type:        jsonschema.String,
					Description: "The city and state, e.g. San Francisco, CA",
				},
				"unit": {
					Type: jsonschema.String,
					Enum: []string{"celsius", "fahrenheit"},
				},
			},
			Required: []string{"rationale", "location"},
		},
	},
}

var tools = []llms.Tool{
	{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "getCurrentWeather",
			Description: "Get the current weather in a given location",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"rationale": {
						Type:        jsonschema.String,
						Description: "The rationale for choosing this function call with these parameters",
					},
					"location": {
						Type:        jsonschema.String,
						Description: "The city and state, e.g. San Francisco, CA",
					},
					"unit": {
						Type: jsonschema.String,
						Enum: []string{"celsius", "fahrenheit"},
					},
				},
				Required: []string{"rationale", "location"},
			},
		},
	},
	{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "getTomorrowWeather",
			Description: "Get the predicted weather in a given location",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"rationale": {
						Type:        jsonschema.String,
						Description: "The rationale for choosing this function call with these parameters",
					},
					"location": {
						Type:        jsonschema.String,
						Description: "The city and state, e.g. San Francisco, CA",
					},
					"unit": {
						Type: jsonschema.String,
						Enum: []string{"celsius", "fahrenheit"},
					},
				},
				Required: []string{"rationale", "location"},
			},
		},
	},
	{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "getSuggestedPrompts",
			Description: "Given the user's input prompt suggest some related prompts",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"rationale": {
						Type:        jsonschema.String,
						Description: "The rationale for choosing this function call with these parameters",
					},
					"suggestions": {
						Type: jsonschema.Array,
						Items: &jsonschema.Definition{
							Type:        jsonschema.String,
							Description: "A suggested prompt",
						},
					},
				},
				Required: []string{"rationale", "suggestions"},
			},
		},
	},
}

func (c *Completer2) AddResponsesToConversation(sentences <-chan model.ResponseChunk, conv *model.Conversation) <-chan struct{} {
	ch := make(chan struct{})

	go func() {
		defer close(ch)

		for sentence := range sentences {
			log.Println("assistant:", strings.TrimSpace(sentence.Text))

			conv.AddMessage(llms.TextParts(llms.ChatMessageTypeAI, strings.TrimSpace(strings.TrimPrefix(sentence.Text, c.StripResponsePrefix))))
		}
	}()

	return ch
}
