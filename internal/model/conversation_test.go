package model

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConversationMessagesPrunesFadedTopics(t *testing.T) {
	now := time.Now()
	topics := NewFadingTopics(time.Hour)
	topics.Merge([]string{"fresh-topic"}, now.Add(-time.Minute))
	topics.Merge([]string{"stale-topic"}, now.Add(-2*time.Hour))
	conv := NewConversation(1, time.Hour)
	conv.SetState(ConversationState{
		RequestNum: 1,
		StructuredConversationState: StructuredConversationState{
			Topics: topics,
		},
	})

	msgs := conv.Messages("system prompt")
	require.NotEmpty(t, msgs)
	systemMsg := FormatMessage(msgs[0])

	assert.Contains(t, systemMsg, "fresh-topic")
	assert.NotContains(t, systemMsg, "stale-topic")
	assert.NotContains(t, systemMsg, "lastSeen")

	// Pruned topics stay out of the state.
	state := conv.State()
	require.Len(t, state.Topics.Topics(), 1)
	assert.Equal(t, "fresh-topic", state.Topics.Topics()[0].Text)
	assert.False(t, strings.Contains(mustMarshal(t, state), "stale-topic"))
}

func mustMarshal(t *testing.T, v interface{}) string {
	b, err := yaml.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
