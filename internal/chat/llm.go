package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type ResponseChunk = model.Message

type ResponseDelegated struct {
	error
}

func IsResponseDelegated(err error) bool {
	var e *ResponseDelegated
	return errors.As(err, &e)
}

type LLM struct {
	ServerURL           string
	APIKey              string
	Model               string
	Temperature         float64
	FrequencyPenalty    float64
	MaxTokens           int
	StripResponsePrefix string
	MaxTurns            int
	HTTPClient          HTTPDoer

	llm *openai.LLM
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func (c *LLM) ChatCompletion(ctx context.Context, reqNum int64, fn []functions.Function, conv *model.Conversation, ch chan<- ResponseChunk) error {
	if c.llm == nil {
		llm, err := openai.New(
			openai.WithHTTPClient(c.HTTPClient),
			openai.WithBaseURL(c.ServerURL+"/v1"),
			openai.WithToken(c.APIKey),
			openai.WithModel(c.Model),
		)
		if err != nil {
			return err
		}

		c.llm = llm
	}

	// TODO: align [SAY]
	fns := functions.NewCallLoopPreventingProvider(fn)
	// TODO: add function to let the LLM say something to the user while using tools?
	// On the the one hand this might provide good UX when the LLM provides dynamic feedback (alternative to a reasoning argument).
	// On the other hand the client could require the LLM to always respond with a function call - no special cases, no accidental reading of function call JSON aloud and function call JSON would always be parsed via StreamingFunc.
	// => turned out not to be great since latency was increased the often the AI said that it would look something up without actually doing it.
	/*fns := functions.NewCallLoopPreventingProvider(&ResponseFunctionProvider{
		Delegate:   c.Functions,
		RequestNum: reqNum,
		Ch:         ch,
	})*/
	turn := 0

	for {
		turn++

		if c.MaxTurns > 0 && turn > c.MaxTurns {
			slog.Warn(fmt.Sprintf("maximum LLM conversation turns of %d was exceeded for the request", c.MaxTurns))

			fns = functions.NewCallLoopPreventingProvider(nil)
		}

		err := c.createChatCompletion(ctx, reqNum, fns, conv, ch)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if _, ok := err.(*reconcileError); !ok {
			slog.Warn(err.Error())
		}
	}
}

func (c *LLM) createChatCompletion(ctx context.Context, reqNum int64, fns *functions.CallLoopPreventingProvider, conv *model.Conversation, ch chan<- ResponseChunk) error {
	if conv.RequestCounter() > reqNum {
		return nil // skip outdated request (user requested something else)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	conv.AddCancelFunc(cancel)

	functions, err := fns.Functions()
	if err != nil {
		return fmt.Errorf("get available tools: %w", err)
	}

	llmFunctions := make([]llms.FunctionDefinition, len(functions))
	for i, f := range functions {
		llmFunctions[i] = f.Definition()
	}

	messages := conv.Messages()

	printMessages(messages)

	var toolCalls []aiToolCall

	streamingFunc := func(_ context.Context, chunk []byte) error {
		c.emitResponseChunk(string(chunk), reqNum, ch)
		return nil
	}
	if len(llmFunctions) > 0 {
		streamingFunc = func(_ context.Context, chunk []byte) error {
			if chunkStr := string(chunk); !strings.HasPrefix(chunkStr, "[{") {
				c.emitResponseChunk(chunkStr, reqNum, ch)

				return nil
			}

			addToolCalls := make([]aiToolCall, 0, 1)

			err := json.Unmarshal(chunk, &addToolCalls)
			if err != nil {
				slog.Warn("failed to parse tool calls from chunk", "err", err, "chunk", chunk)
				c.emitResponseChunk(string(chunk), reqNum, ch)

				return nil
			}

			toolCalls = append(toolCalls, addToolCalls...)

			return nil
		}
	}

	streamingFuncWrapper := func(ctx context.Context, chunk []byte) error {
		if conv.RequestCounter() > reqNum {
			// Cancel response stream if request is outdated (user requested something else)
			cancel()
			return nil
		}

		if len(chunk) == 0 {
			return nil
		}

		slog.Debug(fmt.Sprintf("received chunk %q", string(chunk)))

		return streamingFunc(ctx, chunk)
	}

	_, err = c.llm.GenerateContent(ctx,
		messages,
		llms.WithStreamingFunc(streamingFuncWrapper),
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

	if len(toolCalls) > 0 {
		toolCalls = mergeToolCalls(toolCalls)

		for _, call := range toolCalls {
			if call.Type != "function" || call.FunctionCall == nil {
				slog.Warn(fmt.Sprintf("ignoring unsupported tool call: %#v", call))
				continue
			}

			if conv.RequestCounter() > reqNum {
				return nil // skip outdated request (user requested something else)
			}

			err := handleToolCall(ctx, call.ToolCall(), reqNum, fns, conv, ch)
			if err != nil {
				if IsResponseDelegated(err) {
					// TODO: support using multiple agents/request (one for each task, e.g. change volume AND research sth)
					return nil // toolified agent answered to the user directly
				}

				return fmt.Errorf("failed to call tool %q: %w", call.FunctionCall.Name, err)
			}
		}

		return &reconcileError{fmt.Errorf("needs reconciliation")}
	}

	return nil
}

func (c *LLM) emitResponseChunk(chunk string, reqNum int64, ch chan<- ResponseChunk) {
	ch <- ResponseChunk{
		Type:       model.MessageTypeChunk,
		RequestNum: reqNum,
		Text:       strings.TrimPrefix(chunk, c.StripResponsePrefix),
	}
}

type reconcileError struct {
	error
}

func handleToolCall(ctx context.Context, toolCall llms.ToolCall, reqNum int64, fns *functions.CallLoopPreventingProvider, conv *model.Conversation, ch chan<- ResponseChunk) error {
	call := toolCall.FunctionCall
	args := map[string]any{}

	if call.Arguments != "" {
		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Errorf("parse tool call arguments: %w", err)
		}
	}

	callAllowed, err := fns.IsFunctionCallAllowed(call.Name, args)
	if err != nil {
		return fmt.Errorf("deduplicate tool call: %w", err)
	}

	if !callAllowed {
		// Re-request chat completion without the now banned tool
		return fmt.Errorf("repeating tool call %q is not allowed", call.Name)
	}

	if rationale, ok := args["rationale"]; ok && rationale != "" {
		infos := splitIntoSentences(fmt.Sprintf("%v", rationale))

		if len(infos) > 1 {
			infos = infos[:1]
		}

		// TODO: let it explain its rationale again? [SAY]
		//infos = append(infos, fmt.Sprintf("Let me use my %q tool.", call.Name))
		infos = []string{fmt.Sprintf("Let me use my %q tool.", call.Name)}

		for _, sentence := range infos {
			ch <- ResponseChunk{
				Type:       model.MessageTypeChunk,
				RequestNum: reqNum,
				Text:       sentence,
				UserOnly:   true,
			}
		}
	}

	result, err := callTool(ctx, toolCall, args, fns)
	if err != nil {
		if IsResponseDelegated(err) {
			return err
		}

		msg := fmt.Sprintf("ERROR: failed to call tool %q: %s", call.Name, err)
		result = msg

		slog.Warn(msg)
	}

	conv.AddToolCallResponse(reqNum, toolCall, result)

	return nil
}

func callTool(ctx context.Context, call llms.ToolCall, args map[string]any, fns *functions.CallLoopPreventingProvider) (string, error) {
	slog.Debug(fmt.Sprintf("ai tool call %s of function %s with args %#v", call.ID, call.FunctionCall.Name, call.FunctionCall.Arguments))

	fnList, err := fns.Functions()
	if err != nil {
		return "", err
	}

	fn, err := functions.FindByName(call.FunctionCall.Name, fnList)
	if err != nil {
		return "", err
	}

	err = validateFunctionCallArgs(args, fn.Definition())
	if err != nil {
		return "", err
	}

	functionCallResult, err := fn.Call(ctx, args)
	if err != nil {
		return "", err
	}

	functionCallResult = strings.TrimSpace(functionCallResult)

	if functionCallResult == "" {
		return "", errors.New("tool call returned empty result")
	}

	result := ""
	if len(functionCallResult) > 0 {
		result = strings.ReplaceAll("\n"+functionCallResult, "\n", "\n\t")
	}

	slog.Debug(fmt.Sprintf("tool %s result: %s", call.FunctionCall.Name, result))

	return result, nil
}

var whitespaceRegex = regexp.MustCompile(`\s+`)

func printMessages(messages []llms.MessageContent) {
	msgs := make([]string, 0, len(messages))

	for i, m := range messages {
		content := model.FormatMessage(m)
		if len(content) > 140 {
			content = content[:140] + "..."
		}

		content = strings.ReplaceAll(content, "\n", " ")
		content = whitespaceRegex.ReplaceAllString(content, " ")

		msgs = append(msgs, fmt.Sprintf("\n\t%d. %s", i, content))
	}

	slog.Debug(fmt.Sprintf("requesting chat completion for message history: %s", strings.Join(msgs, "")))
}

func validateFunctionCallArgs(args map[string]any, paramDefinition llms.FunctionDefinition) error {
	if len(args) == 0 {
		return errors.New("function called with empty arguments")
	}

	// TODO: validate parameters

	return nil
}

func mergeToolCalls(calls []aiToolCall) []aiToolCall {
	callMap := make(map[string]aiToolCall, len(calls))
	ids := make([]string, 0, len(calls))
	result := make([]aiToolCall, 0, len(calls))

	for _, call := range calls {
		if call.FunctionCall == nil {
			continue
		}

		if lastCall, ok := callMap[call.ID]; ok {
			if lastCall.FunctionCall.Name != "" {
				call.FunctionCall.Name = lastCall.FunctionCall.Name
			}
			if lastCall.FunctionCall.Arguments != "" {
				call.FunctionCall.Arguments = lastCall.FunctionCall.Arguments
			}
		} else {
			ids = append(ids, call.ID)
		}

		callMap[call.ID] = call
	}

	for _, id := range ids {
		result = append(result, callMap[id])
	}

	return result
}

type aiToolCall struct {
	ID           string        `json:"id"`
	Type         string        `type:"type"`
	FunctionCall *functionCall `json:"function,omitempty"`
}

func (c *aiToolCall) ToolCall() llms.ToolCall {
	return llms.ToolCall{
		ID:   c.ID,
		Type: c.Type,
		FunctionCall: &llms.FunctionCall{
			Name:      c.FunctionCall.Name,
			Arguments: c.FunctionCall.Arguments,
		},
	}
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `Json:"arguments"`
}
