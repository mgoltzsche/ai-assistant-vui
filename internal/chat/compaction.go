package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"gopkg.in/yaml.v3"
)

var conversationStateSchema = conversationStateJSONSchema()

// conversationStateDelta is the LLM-facing shape of the conversation state:
// plain strings without timestamps, timestamps are managed in code.
type conversationStateDelta struct {
	Topics []string `json:"topics"`
	Facts  []string `json:"facts"`
}

type ConversationCompaction struct {
	LLMConfig

	// Now returns the current time. Defaults to time.Now, override in tests.
	Now func() time.Time

	llm llms.LLM
}

func (c *ConversationCompaction) RunCompaction(ctx context.Context, conv *model.Conversation, warmup func(context.Context, *model.Conversation) error) error {
	if c.llm == nil {
		llm, err := openai.New(
			openai.WithHTTPClient(c.HTTPClient),
			openai.WithBaseURL(c.ServerURL+"/v1"),
			openai.WithToken(c.APIKey),
			openai.WithModel(c.Model),
			openai.WithResponseFormat(&openai.ResponseFormat{
				Type: "json_schema",
				JSONSchema: &openai.ResponseFormatJSONSchema{
					Name:   "state",
					Strict: true,
					Schema: conversationStateSchema,
				},
			}),
		)
		if err != nil {
			return err
		}

		c.llm = llm
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conv.AddCancelFunc(cancel)

	state := conv.State()

	if len(state.Messages) < 3 {
		return nil // nothing to do
	}

	b, err := yaml.Marshal(state.StructuredConversationState)
	if err != nil {
		return fmt.Errorf("marshal conversation state: %w", err)
	}

	systemPrompt := fmt.Sprintf("Extract the topics and facts that are actively discussed in the chat history following these instructions.\nOnly include topics and facts which that chat history actively mentions or discusses. Never copy entries from the existing state below unless they are discussed in the chat history.\nWhen an entry refers to the same topic or fact as an existing entry reuse its exact wording.\nExisting state:\n%s", string(b))

	msgs := append(
		[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt)},
		state.Messages...,
	)

	resp, err := c.llm.GenerateContent(ctx, msgs, llms.WithJSONMode())
	if err != nil {
		return fmt.Errorf("conversation compaction: %w", err)
	}

	if len(resp.Choices) == 0 {
		return errors.New("llm did not return any response")
	}

	if strings.TrimSpace(resp.Choices[0].Content) == "" {
		return errors.New("llm returned empty response")
	}

	newStateDelta, err := parseStateDelta(resp.Choices[0].Content)
	if err != nil {
		slog.Debug("unexpected conversation compaction request response", "response", resp.Choices[0].Content)
		return fmt.Errorf("parse llm response into conversation state: %w", err)
	}

	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}

	state.Topics.Prune(now)
	state.StructuredConversationState = mergeState(state.StructuredConversationState, newStateDelta, now)
	// TODO: persist extracted facts into long-term memory (move old state entries into long-term memory)
	// TODO: remove old facts and topics from state

	// Remove all messages but the last one (AI message).
	// This is to be able to correlate the user response, in case the AI asked a question.
	offset := len(state.Messages)
	if offset > 0 {
		offset -= 1
	}
	state.Messages = state.Messages[offset:]

	// Create a new warmup conversation from scratch in order to warm up the cache for the compacted state.
	warmupConv := model.NewConversation(state.RequestNum, 0) // The topic fade duration is carried by the state.
	warmupConv.SetState(state)
	err = warmup(ctx, warmupConv)
	if err != nil {
		return fmt.Errorf("warmup: %w", err)
	}

	conv.SetState(state)

	return nil
}

// parseStateDelta extracts the conversation state from the LLM response.
// Some LLMs stream a plain-text answer before the JSON object even when a
// JSON response format is requested, so the JSON object is searched for
// instead of assuming the content is pure JSON.
func parseStateDelta(content string) (conversationStateDelta, error) {
	for i, c := range content {
		if c != '{' {
			continue
		}
		// Decode one JSON value starting at the opening brace,
		// ignoring any text that follows it.
		var delta struct {
			Topics *[]string `json:"topics"`
			Facts  *[]string `json:"facts"`
		}
		err := json.NewDecoder(strings.NewReader(content[i:])).Decode(&delta)
		if err != nil {
			continue
		}
		if delta.Topics == nil && delta.Facts == nil {
			continue // not the state object, look for the next one
		}
		result := conversationStateDelta{}
		if delta.Topics != nil {
			result.Topics = *delta.Topics
		}
		if delta.Facts != nil {
			result.Facts = *delta.Facts
		}
		return result, nil
	}
	return conversationStateDelta{}, errors.New("no conversation state JSON object found in llm response")
}

func mergeState(s model.StructuredConversationState, d conversationStateDelta, now time.Time) model.StructuredConversationState {
	s.Topics.Merge(d.Topics, now)
	return model.StructuredConversationState{
		Topics: s.Topics,
		Facts:  mergeStrings(s.Facts, d.Facts),
	}
}

func mergeStrings(l1, l2 []string) []string {
	m := make(map[string]struct{}, len(l1)+len(l2))
	result := make([]string, 0, len(l1)+len(l2))
	for _, list := range [][]string{l1, l2} {
		for _, item := range list {
			if _, seen := m[item]; seen {
				continue
			}
			m[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func conversationStateJSONSchema() *openai.ResponseFormatJSONSchemaProperty {
	jsonSchema, err := jsonschema.For[conversationStateDelta](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Errorf("generate jsonschema from struct: %w", err))
	}
	b, err := jsonSchema.MarshalJSON()
	if err != nil {
		panic(fmt.Errorf("marshal jsonschema: %w", err))
	}
	schema := &openai.ResponseFormatJSONSchemaProperty{}
	err = json.Unmarshal(b, schema)
	if err != nil {
		panic(fmt.Errorf("unmarshal jsonschema: %w", err))
	}
	return schema
}
