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
	RequestNum int64
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
		RequestNum:     1,
		MessageContent: llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
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
		RequestNum:     c.requestCounter,
		MessageContent: llms.TextParts(llms.ChatMessageTypeHuman, msg),
	}

	log.Println("user request:", strings.TrimSpace(msg))

	c.dropPreviousMessages()
	c.addMessage(cmsg)

	return c.requestCounter
}

func (c *Conversation) AddAIResponse(requestNum int64, msg string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.addMessage(conversationMessage{
		RequestNum:     requestNum,
		MessageContent: llms.TextParts(llms.ChatMessageTypeAI, msg),
	}) {
		log.Println("assistant:", strings.TrimSpace(msg))
		return true
	}

	return false
}

func (c *Conversation) AddToolCall(requestNum int64, callID string, call llms.FunctionCall) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.addMessage(conversationMessage{
		RequestNum: requestNum,
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

func (c *Conversation) AddToolResponse(requestNum int64, resp llms.ToolCallResponse) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.addMessage(conversationMessage{
		RequestNum: requestNum,
		MessageContent: llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{resp},
		},
	})
}

func (c *Conversation) addMessage(msg conversationMessage) bool {
	if c.requestCounter > msg.RequestNum {
		// ignore response from an outdated request
		return false
	}

	messages := c.messages

	if len(messages) > 0 && messages[len(messages)-1].Role == msg.Role {
		// TODO: add whitespace
		messages[len(messages)-1].Parts = append(messages[len(messages)-1].Parts, msg.Parts...)
		messages[len(messages)-1].RequestNum = msg.RequestNum
	} else {
		messages = append(messages, msg)
	}

	c.messages = messages

	return true
}

func (c *Conversation) dropPreviousMessages() {
	filtered := make([]conversationMessage, 0, len(c.messages)+1)

	for _, msg := range c.messages {
		if msg.RequestNum < 2 || msg.RequestNum == c.requestCounter {
			filtered = append(filtered, msg)
		}
	}

	c.messages = filtered
}

func (c *Conversation) Messages() []llms.MessageContent {
	msgs := c.messages
	nsgContents := make([]llms.MessageContent, len(msgs))

	for i, msg := range msgs {
		nsgContents[i] = msg.MessageContent
	}

	return nsgContents
}

func (c *Conversation) RequestMessages() []llms.MessageContent {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	msgs := make([]llms.MessageContent, 0, 10)
	for _, msg := range c.messages {
		if msg.RequestNum == c.requestCounter {
			msgs = append(msgs, msg.MessageContent)
		}
	}

	return msgs
}
