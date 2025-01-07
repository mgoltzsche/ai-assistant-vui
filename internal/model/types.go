package model

import (
	"sync"

	"github.com/sashabaranov/go-openai"
)

type Request struct {
	ID   int64
	Text string
}

type ResponseChunk struct {
	RequestID int64
	Text      string
}

type ConversationContext struct {
	requestCounter *int64
	messages       []openai.ChatCompletionMessage
	mutex          sync.Mutex
}

func NewConversationContext(requestCounter *int64, systemPrompt string) *ConversationContext {
	messages := make([]openai.ChatCompletionMessage, 1, 100)
	messages[0] = openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}

	return &ConversationContext{
		requestCounter: requestCounter,
		messages:       messages,
	}
}

func (c *ConversationContext) RequestCounter() int64 {
	return *c.requestCounter
}

func (c *ConversationContext) AddMessage(msg openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	messages := c.messages

	if messages[len(messages)-1].Role == msg.Role {
		// TODO: use original message without trimmed space here?!
		messages[len(messages)-1].Content += " " + msg.Content
	} else {
		messages = append(messages, msg)
	}

	c.messages = messages

	return messages
}

func (c *ConversationContext) Messages() []openai.ChatCompletionMessage {
	return c.messages
}
