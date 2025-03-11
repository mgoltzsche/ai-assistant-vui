package model

import (
	"sync"

	"github.com/tmc/langchaingo/llms"
)

type Message struct {
	RequestID int64
	Text      string
}

type AudioMessage struct {
	Message
	WaveData []byte
}

type Conversation struct {
	requestCounter int64
	messages       []llms.MessageContent
	mutex          sync.Mutex
}

func NewConversation(systemPrompt string) *Conversation {
	messages := make([]llms.MessageContent, 1, 100)
	messages[0] = llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt)

	return &Conversation{
		messages: messages,
	}
}

func (c *Conversation) RequestCounter() int64 {
	return c.requestCounter
}

func (c *Conversation) NewRequestID() int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.requestCounter++

	return c.requestCounter
}

func (c *Conversation) AddMessage(msg llms.MessageContent) []llms.MessageContent {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	messages := c.messages

	if messages[len(messages)-1].Role == msg.Role {
		// TODO: use original message without trimmed space here?!
		messages[len(messages)-1].Parts = append(messages[len(messages)-1].Parts, msg.Parts...)
	} else {
		messages = append(messages, msg)
	}

	c.messages = messages

	return messages
}

func (c *Conversation) Messages() []llms.MessageContent {
	return c.messages
}
