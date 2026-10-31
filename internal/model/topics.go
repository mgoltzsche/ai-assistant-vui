package model

import (
	"strings"
	"time"
)

// Topic is a conversation topic with the time it was last actively discussed.
type Topic struct {
	Text     string    `json:"text"`
	LastSeen time.Time `json:"lastSeen"`
}

// FadingTopics is a set of unique topics that drops entries
// that were not actively discussed within MaxAge.
// The zero value is an empty set with fading disabled.
type FadingTopics struct {
	// MaxAge removes topics that were not actively discussed within the given duration. Disabled when not positive.
	MaxAge time.Duration
	items  []Topic
}

func NewFadingTopics(maxAge time.Duration) FadingTopics {
	return FadingTopics{MaxAge: maxAge}
}

// Topics returns the topics in insertion order.
func (t FadingTopics) Topics() []Topic {
	return t.items
}

// Prune removes topics that were not actively discussed within MaxAge.
func (t *FadingTopics) Prune(now time.Time) {
	if t.MaxAge <= 0 {
		return
	}

	kept := t.items[:0]
	for _, topic := range t.items {
		if now.Sub(topic.LastSeen) <= t.MaxAge {
			kept = append(kept, topic)
		}
	}
	t.items = kept
}

// Merge refreshes the LastSeen timestamp of topics that match the given
// discussed texts case-insensitively and appends the remaining ones.
func (t *FadingTopics) Merge(discussed []string, now time.Time) {
	merged := make([]Topic, 0, len(t.items)+len(discussed))
	index := make(map[string]int, len(t.items)+len(discussed))
	for _, topic := range t.items {
		key := strings.ToLower(topic.Text)
		if _, seen := index[key]; seen {
			continue
		}
		index[key] = len(merged)
		merged = append(merged, topic)
	}
	for _, text := range discussed {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		key := strings.ToLower(text)
		if i, ok := index[key]; ok {
			merged[i].LastSeen = now
			continue
		}
		index[key] = len(merged)
		merged = append(merged, Topic{Text: text, LastSeen: now})
	}
	t.items = merged
}

// MarshalYAML renders the topic texts as plain strings so that timestamps stay out of LLM prompts.
func (t FadingTopics) MarshalYAML() (interface{}, error) {
	texts := make([]string, 0, len(t.items))
	for _, topic := range t.items {
		texts = append(texts, topic.Text)
	}
	return texts, nil
}
