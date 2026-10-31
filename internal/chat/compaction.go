package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"gopkg.in/yaml.v3"
)

var conversationStateSchema = conversationStateJSONSchema()

type ConversationCompaction struct {
	LLMConfig

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

	systemPrompt := fmt.Sprintf("Extract the contained topics, facts, conclusions and pending discussions from the chat history and add each to the corresponding string list within the given YAML object, if not contained already, and return the whole object with all new entries:\n\n%s", string(b))

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

	newStateDelta := model.StructuredConversationState{}
	err = json.Unmarshal([]byte(resp.Choices[0].Content), &newStateDelta)
	if err != nil {
		slog.Debug("unexpected conversation compaction request response", "response", resp.Choices[0].Content)
		return fmt.Errorf("unmarshal llm response into conversation state: %w", err)
	}

	state.StructuredConversationState = mergeState(state.StructuredConversationState, newStateDelta)
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
	warmupConv := model.NewConversation(state.RequestNum)
	warmupConv.SetState(state)
	err = warmup(ctx, warmupConv)
	if err != nil {
		return fmt.Errorf("warmup: %w", err)
	}

	conv.SetState(state)

	return nil
}

func mergeState(s1, s2 model.StructuredConversationState) model.StructuredConversationState {
	return model.StructuredConversationState{
		Topics: mergeStrings(s1.Topics, s2.Topics),
		Facts:  mergeStrings(s1.Facts, s2.Facts),
	}
}

func mergeStrings(l1, l2 []string) []string {
	m := make(map[string]struct{}, len(l1)+len(l2))
	result := make([]string, 0, len(l1)+len(l2))
	for _, item := range l1 {
		if _, seen := m[item]; seen {
			continue
		}
		m[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func conversationStateJSONSchema() *openai.ResponseFormatJSONSchemaProperty {
	jsonSchema, err := jsonschema.For[model.StructuredConversationState](&jsonschema.ForOptions{})
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
