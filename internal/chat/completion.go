package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	openai "github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/jsonschema"
	"github.com/tmc/langchaingo/llms"
	//"github.com/tmc/langchaingo/llms/openai"
)

type ChatCompletionRequest struct {
	RequestID int64
}

type Completer struct {
	ServerURL           string
	Model               string
	Temperature         float64
	FrequencyPenalty    float64
	MaxTokens           int
	StripResponsePrefix string
	HTTPClient          HTTPDoer
	client              *openai.Client
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

/*func (c *Completer2) RunChatCompletions(ctx context.Context, requests <-chan ChatCompletionRequest, conv *model.Conversation, toolCallSink chan<- ToolCallRequest) (<-chan model.ResponseChunk, error) {
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

func (c *Completer) createChatCompletion(ctx context.Context, llm *openai.LLM, reqID int64, conv *model.Conversation, toolCallSink chan<- ToolCallRequest, ch chan<- model.ResponseChunk) error {
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
}*/

func (c *Completer) RunChatCompletions(ctx context.Context, requests <-chan ChatCompletionRequest, conv *model.Conversation, toolCallSink chan<- ToolCallRequest) (<-chan model.ResponseChunk, error) {
	c.client = openai.NewClientWithConfig(openai.ClientConfig{
		BaseURL:            c.ServerURL + "/v1",
		HTTPClient:         c.HTTPClient,
		EmptyMessagesLimit: 10,
	})

	ch := make(chan model.ResponseChunk, 50)

	go func() {
		defer close(ch)

		err := c.createOpenAIChatCompletion(ctx, convertMessages(conv.Messages()), conv.RequestCounter(), conv, toolCallSink, ch)
		if err != nil {
			log.Println("ERROR: chat completion:", err)
		}

		for req := range requests {
			if conv.RequestCounter() > req.RequestID {
				continue // skip outdated request (user requested something else)
			}

			err := c.createOpenAIChatCompletion(ctx, convertMessages(conv.Messages()), req.RequestID, conv, toolCallSink, ch)
			if err != nil {
				log.Println("ERROR: chat completion:", err)
			}
		}
	}()

	return ch, nil
}

func convertMessages(msgs []llms.MessageContent) []openai.ChatCompletionMessage {
	r := make([]openai.ChatCompletionMessage, len(msgs))

	for i, m := range msgs {
		content := make([]string, len(m.Parts))
		for j, p := range m.Parts {
			content[j] = fmt.Sprintf("%s", p)
		}

		r[i] = openai.ChatCompletionMessage{
			Role:    string(m.Role),
			Content: strings.Join(content, ""),
		}
	}

	return r
}

func (c *Completer) createOpenAIChatCompletion(ctx context.Context, msgs []openai.ChatCompletionMessage, reqID int64, conv *model.Conversation, toolCallSink chan<- ToolCallRequest, ch chan<- model.ResponseChunk) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:            c.Model,
		Temperature:      float32(c.Temperature),
		FrequencyPenalty: float32(c.FrequencyPenalty),
		MaxTokens:        c.MaxTokens,
		Messages:         msgs,
		Functions: []openai.FunctionDefinition{
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
		},
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return err
	}
	defer stream.Close()

	var buf bytes.Buffer
	lastContent := ""
	lastSentence := ""

	for {
		response, e := stream.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			if !errors.Is(e, context.Canceled) {
				err = fmt.Errorf("streaming chat completion response: %w", e)
			}
			break
		}

		if conv.RequestCounter() > reqID {
			// Cancel response stream if request is outdated (user requested something else)
			cancel()
			continue
		}

		choice := response.Choices[0]
		content := response.Choices[0].Delta.Content

		buf.WriteString(content)

		if choice.FinishReason == openai.FinishReasonToolCalls {
			// Dispatch function call
			call, callID, err := parseFunctionCall(buf.String())
			if err != nil {
				return fmt.Errorf("parse function call: %w", err)
			}

			if call == nil {
				fmt.Println("WARNING: failed to parse function call:", buf.String())
				continue
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
					Type:         "function",
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

		// TODO: don't emit separate event for numbered list items, e.g. 3. ?!
		if buf.Len() > len(content)+1 && (content == "\n" || content == " " && (lastContent == "." || lastContent == "!" || lastContent == "?")) {
			sentence := buf.String()

			if strings.TrimSpace(sentence) != "" {
				if sentence == lastSentence {
					// Cancel response stream if last sentence was repeated
					cancel()
					continue
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
	}

	if buf.Len() > 0 {
		ch <- model.ResponseChunk{
			RequestID: reqID,
			Text:      strings.TrimSuffix(buf.String(), "</s>"),
		}
	}

	return err
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

		// TODO: fix streaming
		fmt.Println("## chunk:", content)

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

func (c *Completer) AddResponsesToConversation(sentences <-chan model.ResponseChunk, conv *model.Conversation) <-chan struct{} {
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
