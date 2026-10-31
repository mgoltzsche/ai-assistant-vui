package chat

import (
	"testing"
	"time"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeState(t *testing.T) {
	old := time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC)
	now := old.Add(time.Hour)
	topics := model.NewFadingTopics(0)
	topics.Merge([]string{"Gardening"}, old)
	state := model.StructuredConversationState{
		Topics: topics,
		Facts:  []string{"user lives in Berlin"},
	}
	delta := conversationStateDelta{
		Topics: []string{"Gardening", "Sailing"},
		Facts:  []string{"user lives in Berlin", "user owns a boat"},
	}

	merged := mergeState(state, delta, now)

	assert.Equal(t, []model.Topic{
		{Text: "Gardening", LastSeen: now},
		{Text: "Sailing", LastSeen: now},
	}, merged.Topics.Topics())
	assert.Equal(t, []string{"user lives in Berlin", "user owns a boat"}, merged.Facts)
}

func TestMergeStrings(t *testing.T) {
	merged := mergeStrings([]string{"a", "b", "a"}, []string{"b", "c"})

	assert.Equal(t, []string{"a", "b", "c"}, merged)
}

func TestParseStateDelta(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected conversationStateDelta
		errMsg   string
	}{
		{
			name:    "pure JSON",
			content: `{"topics": ["Germany"], "facts": ["user lives in Berlin"]}`,
			expected: conversationStateDelta{
				Topics: []string{"Germany"},
				Facts:  []string{"user lives in Berlin"},
			},
		},
		{
			name:    "plain text before JSON",
			content: "Why don't skeletons fight each other?  \n{ \"facts\": [\"Why don't skeletons fight each other? They don't have the guts.\"] , \"topics\": [\"Germany\", \"Capital cities\"] }",
			expected: conversationStateDelta{
				Topics: []string{"Germany", "Capital cities"},
				Facts:  []string{"Why don't skeletons fight each other? They don't have the guts."},
			},
		},
		{
			name:    "text after JSON",
			content: `{"topics": ["Sailing"]} hope that helps!`,
			expected: conversationStateDelta{
				Topics: []string{"Sailing"},
			},
		},
		{
			name:    "braces in leading text",
			content: `I like {braces} in text. {"facts": ["a"]}`,
			expected: conversationStateDelta{
				Facts: []string{"a"},
			},
		},
		{
			name:    "empty arrays",
			content: `{"topics": [], "facts": []}`,
			expected: conversationStateDelta{
				Topics: []string{},
				Facts:  []string{},
			},
		},
		{
			name:    "no JSON",
			content: "no JSON here",
			errMsg:  "no conversation state JSON object found",
		},
		{
			name:    "JSON without state fields",
			content: `{"joke": "They don't have the guts."}`,
			errMsg:  "no conversation state JSON object found",
		},
		{
			name:    "truncated JSON",
			content: `{"topics": ["Germany"`,
			errMsg:  "no conversation state JSON object found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta, err := parseStateDelta(tt.content)
			if tt.errMsg != "" {
				require.ErrorContains(t, err, tt.errMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, delta)
		})
	}
}
