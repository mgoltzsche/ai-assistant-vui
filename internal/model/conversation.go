package model

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/tmc/langchaingo/llms"
)

type Conversation struct {
	requestCounter int64
	cancelFuncs    []context.CancelFunc
	messages       []conversationMessage
	mutex          sync.Mutex
}

type conversationMessage struct {
	RequestNum int64
	llms.MessageContent
}

func FormatMessage(m llms.MessageContent) string {
	return fmt.Sprintf("%s: %s", m.Role, formatMessageParts(m.Parts))
}

func formatMessageParts(parts []llms.ContentPart) string {
	strs := make([]string, len(parts))

	for i, p := range parts {
		switch part := p.(type) {
		case llms.TextContent:
			strs[i] = part.Text
		case llms.ToolCall:
			strs[i] = fmt.Sprintf("{%s %s(%s)}", part.ID[:5], part.FunctionCall.Name, part.FunctionCall.Arguments)
		case llms.ToolCallResponse:
			strs[i] = fmt.Sprintf("&%s %q", part.ToolCallID[:5], part.Content)
		case llms.BinaryContent:
			strs[i] = "[binary]"
		default:
			strs[i] = fmt.Sprintf("%T%v", p, p)
		}
	}

	return strings.Join(strs, "")
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

func (c *Conversation) AddCancelFunc(fn context.CancelFunc) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cancelFuncs = append(c.cancelFuncs, fn)
}

func (c *Conversation) RequestCounter() int64 {
	return c.requestCounter
}

func (c *Conversation) AddUserRequest(msgContent llms.ContentPart) int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.requestCounter++

	for _, cancel := range c.cancelFuncs {
		cancel()
	}

	c.cancelFuncs = nil

	cmsg := conversationMessage{
		RequestNum: c.requestCounter,
		MessageContent: llms.MessageContent{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{msgContent},
		},
	}
	msgStr := formatMessageParts(cmsg.MessageContent.Parts)

	slog.Info(fmt.Sprintf("user request: %s", strings.TrimSpace(msgStr)))

	c.dropPreviousMessages()
	c.addMessage(cmsg)

	return c.requestCounter
}

func (c *Conversation) AddAIResponse(requestNum int64, msg string) bool {
	if len(msg) == 0 {
		return false
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.addMessage(conversationMessage{
		RequestNum:     requestNum,
		MessageContent: llms.TextParts(llms.ChatMessageTypeAI, msg),
	}) {
		slog.Info(fmt.Sprintf("assistant: %s", strings.TrimSpace(msg)))
		return true
	}

	return false
}

func (c *Conversation) AddToolCallResponse(requestNum int64, call llms.ToolCall, result string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.requestCounter > requestNum {
		// ignore response from an outdated request
		return
	}

	c.messages = append(c.messages,
		conversationMessage{
			RequestNum: requestNum,
			MessageContent: llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{call},
			},
		},
		conversationMessage{
			RequestNum: requestNum,
			MessageContent: llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{llms.ToolCallResponse{
					ToolCallID: call.ID,
					Name:       call.FunctionCall.Name,
					Content:    result,
				}},
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

	for i, msg := range c.messages {
		if i == 0 || msg.RequestNum == c.requestCounter {
			filtered = append(filtered, msg)
		}
	}

	c.messages = filtered
}

func (c *Conversation) Messages() []llms.MessageContent {
	msgs := c.messages
	msgContents := make([]llms.MessageContent, len(msgs))

	for i, msg := range msgs {
		msgContents[i] = msg.MessageContent
	}

	return msgContents
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
