package chat

import (
	"testing"
	"time"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/stretchr/testify/assert"
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
