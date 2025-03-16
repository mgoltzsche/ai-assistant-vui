package model

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/tmc/langchaingo/llms"
)

type Conversation struct {
	requestCounter int64
	cancel         context.CancelFunc
	messages       []conversationMessage
	mutex          sync.Mutex
}

type conversationMessage struct {
	RequestGeneration int64
	llms.MessageContent
}

func FormatMessage(m llms.MessageContent) string {
	parts := make([]string, len(m.Parts))
	for i, p := range m.Parts {
		switch part := p.(type) {
		case llms.TextContent:
			parts[i] = part.Text
		case llms.ToolCall:
			parts[i] = fmt.Sprintf("{%s %s(%s)}", part.ID[:5], part.FunctionCall.Name, part.FunctionCall.Arguments)
		case llms.ToolCallResponse:
			parts[i] = fmt.Sprintf("&%s %q", part.ToolCallID[:5], part.Content)
		case llms.BinaryContent:
			parts[i] = "[binary]"
		default:
			parts[i] = fmt.Sprintf("%T%v", p, p)
		}
	}
	return fmt.Sprintf("%s: %s", m.Role, strings.Join(parts, " "))
}

func NewConversation(systemPrompt string) *Conversation {
	messages := make([]conversationMessage, 1, 100)
	messages[0] = conversationMessage{
		RequestGeneration: 1,
		MessageContent:    llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
	}

	return &Conversation{
		messages:       messages,
		requestCounter: 1,
	}
}

func (c *Conversation) SetCancelFunc(fn context.CancelFunc) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cancel = fn
}

func (c *Conversation) RequestCounter() int64 {
	return c.requestCounter
}

func (c *Conversation) AddUserRequest(msg string) int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.requestCounter++

	if c.cancel != nil {
		c.cancel()
	}

	cmsg := conversationMessage{
		RequestGeneration: c.requestCounter,
		MessageContent:    llms.TextParts(llms.ChatMessageTypeHuman, msg),
	}

	log.Println("user request:", strings.TrimSpace(msg))

	c.dropPreviousToolCalls()
	c.addMessage(cmsg)

	return c.requestCounter
}

func (c *Conversation) AddAIResponse(generation int64, msg string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.addMessage(conversationMessage{
		RequestGeneration: generation,
		MessageContent:    llms.TextParts(llms.ChatMessageTypeAI, msg),
	}) {
		log.Println("assistant:", strings.TrimSpace(msg))
		return true
	}

	return false
}

func (c *Conversation) AddToolCall(generation int64, callID string, call llms.FunctionCall) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.addMessage(conversationMessage{
		RequestGeneration: generation,
		MessageContent: llms.MessageContent{
			Role: llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{llms.ToolCall{
				ID:           callID,
				Type:         "function",
				FunctionCall: &call,
			}},
		},
	}) {
		log.Printf("ai tool call %s of function %s with args %#v", callID, call.Name, call.Arguments)
		return true
	}

	return false
}

func (c *Conversation) AddToolResponse(generation int64, resp llms.ToolCallResponse) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.addMessage(conversationMessage{
		RequestGeneration: generation,
		MessageContent: llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{resp},
		},
	})
}

func (c *Conversation) addMessage(msg conversationMessage) bool {
	if c.requestCounter > msg.RequestGeneration {
		// ignore response from an outdated request
		return false
	}

	messages := c.messages

	if len(messages) > 0 && messages[len(messages)-1].Role == msg.Role {
		// TODO: add whitespace
		messages[len(messages)-1].Parts = append(messages[len(messages)-1].Parts, msg.Parts...)
		messages[len(messages)-1].RequestGeneration = msg.RequestGeneration
	} else {
		messages = append(messages, msg)
	}

	c.messages = messages

	return true
}

func (c *Conversation) dropPreviousToolCalls() {
	filtered := make([]conversationMessage, 0, len(c.messages)+1)

	for _, msg := range c.messages {
		if msg.RequestGeneration == c.requestCounter || msg.Role != llms.ChatMessageTypeTool && toolCallID(msg.MessageContent) == "" {
			filtered = append(filtered, msg)
		}
	}

	c.messages = filtered
}

func toolCallID(msg llms.MessageContent) string {
	for _, part := range msg.Parts {
		if call, ok := part.(llms.ToolCall); ok && call.Type == "function" {
			return call.ID
		}
	}
	return ""
}

func toolCallResponseID(msg llms.MessageContent) string {
	for _, part := range msg.Parts {
		if resp, ok := part.(llms.ToolCallResponse); ok {
			return resp.ToolCallID
		}
	}
	return ""
}

func (c *Conversation) Messages() []llms.MessageContent {
	msgs := make([]llms.MessageContent, len(c.messages))
	for i, msg := range c.messages {
		msgs[i] = msg.MessageContent
	}
	return msgs
}

func (c *Conversation) RequestMessages() []llms.MessageContent {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	msgs := make([]llms.MessageContent, 0, 10)
	for _, msg := range c.messages {
		if msg.RequestGeneration == c.requestCounter {
			msgs = append(msgs, msg.MessageContent)
		}
	}

	return msgs
}
