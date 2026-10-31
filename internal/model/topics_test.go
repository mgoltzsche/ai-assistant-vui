package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFadingTopicsPrune(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		maxAge   time.Duration
		topics   []Topic
		expected []string
	}{
		{
			name:   "fading disabled when maxAge is not positive",
			maxAge: 0,
			topics: []Topic{
				{Text: "old", LastSeen: now.Add(-365 * 24 * time.Hour)},
			},
			expected: []string{"old"},
		},
		{
			name:   "removes topics not seen within maxAge",
			maxAge: time.Hour,
			topics: []Topic{
				{Text: "recent", LastSeen: now.Add(-30 * time.Minute)},
				{Text: "boundary", LastSeen: now.Add(-time.Hour)},
				{Text: "stale", LastSeen: now.Add(-time.Hour - time.Second)},
			},
			expected: []string{"recent", "boundary"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			topics := NewFadingTopics(tc.maxAge)
			topics.items = tc.topics

			topics.Prune(now)

			texts := make([]string, len(topics.Topics()))
			for i, topic := range topics.Topics() {
				texts[i] = topic.Text
			}
			assert.Equal(t, tc.expected, texts)
		})
	}
}

func TestFadingTopicsMerge(t *testing.T) {
	old := time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC)
	now := old.Add(time.Hour)
	topics := NewFadingTopics(0)
	topics.Merge([]string{"Gardening", "gardening", "Car repair"}, old)
	require.Equal(t, []Topic{
		{Text: "Gardening", LastSeen: old},
		{Text: "Car repair", LastSeen: old},
	}, topics.Topics())

	topics.Merge([]string{"gardening", "  ", "Cooking", "cooking"}, now)

	assert.Equal(t, []Topic{
		{Text: "Gardening", LastSeen: now}, // re-discussed case-insensitively: refreshed, original wording kept
		{Text: "Car repair", LastSeen: old},
		{Text: "Cooking", LastSeen: now}, // newly discussed topic appended once
	}, topics.Topics())
}

func TestFadingTopicsMarshalYAML(t *testing.T) {
	topics := NewFadingTopics(time.Hour)
	topics.Merge([]string{"garden"}, time.Now())
	state := StructuredConversationState{
		Topics: topics,
		Facts:  []string{"user likes tomatoes"},
	}

	b, err := yaml.Marshal(state)
	require.NoError(t, err)

	assert.Contains(t, string(b), "garden")
	assert.NotContains(t, string(b), "lastSeen")
	assert.NotContains(t, string(b), "LastSeen")
}
