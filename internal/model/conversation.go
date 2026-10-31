package model

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/tmc/langchaingo/llms"
	"gopkg.in/yaml.v3"
)

type StructuredConversationState struct {
	Topics []string `json:"topics"`
	Facts  []string `json:"facts"`
	//Conclusions []string `json:"conclusions"`
	//PendingDiscussions []string `json:"pendingDiscussions"`
}

type ConversationState struct {
	RequestNum int64
	Messages   []llms.MessageContent
	StructuredConversationState
}

type Conversation struct {
	requestCounter int64
	cancelFuncs    []context.CancelFunc
	state          ConversationState
	mutex          sync.Mutex
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

func NewConversation(reqNum int64) *Conversation {
	messages := make([]llms.MessageContent, 0, 50)

	return &Conversation{
		state: ConversationState{
			RequestNum: reqNum,
			Messages:   messages,
		},
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

func (c *Conversation) State() ConversationState {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.state
}

func (c *Conversation) SetState(state ConversationState) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.state.RequestNum != state.RequestNum {
		slog.Debug("ignoring outdated conversation state")
		return
	}

	slog.Debug("update conversation state", "state", state)

	c.state = state
}

func (c *Conversation) AddUserRequest(msgContent llms.ContentPart) int64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.requestCounter++

	for _, cancel := range c.cancelFuncs {
		cancel()
	}

	c.cancelFuncs = nil

	msg := llms.MessageContent{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{msgContent},
	}
	msgStr := formatMessageParts(msg.Parts)

	slog.Info(fmt.Sprintf("user request: %s", strings.TrimSpace(msgStr)))

	c.addMessage(c.requestCounter, msg)

	return c.requestCounter
}

func (c *Conversation) AddAIResponse(requestNum int64, msg string) bool {
	if len(msg) == 0 {
		return false
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.addMessage(requestNum, llms.TextParts(llms.ChatMessageTypeAI, msg)) {
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

	c.state.Messages = append(c.state.Messages,
		llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{call},
		},
		llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{llms.ToolCallResponse{
				ToolCallID: call.ID,
				Name:       call.FunctionCall.Name,
				Content:    result,
			}},
		})
}

func (c *Conversation) addMessage(requestNum int64, msg llms.MessageContent) bool {
	if c.requestCounter > requestNum {
		// ignore response from an outdated request
		return false
	}

	messages := c.state.Messages

	if len(messages) > 0 && messages[len(messages)-1].Role == msg.Role {
		// TODO: add whitespace
		messages[len(messages)-1].Parts = append(messages[len(messages)-1].Parts, msg.Parts...)
	} else {
		messages = append(messages, msg)
	}

	c.state.Messages = messages

	return true
}

func toolCallID(msg llms.MessageContent) string {
	for _, part := range msg.Parts {
		if call, ok := part.(llms.ToolCall); ok && call.Type == "function" {
			return call.ID
		}
	}
	return ""
}

// Messages returns the message history prefixed with a system message
// built from the given systemPrompt and the current conversation state.
func (c *Conversation) Messages(systemPrompt string) []llms.MessageContent {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	msgs := c.state.Messages
	msgContents := make([]llms.MessageContent, len(msgs)+1)

	stateYAML, err := yaml.Marshal(c.state)
	if err != nil {
		panic(err)
	}

	systemMsg := fmt.Sprintf("%s\n%s", systemPrompt, stateYAML)
	msgContents[0] = llms.TextParts(llms.ChatMessageTypeSystem, systemMsg)

	for i, msg := range msgs {
		msgContents[i+1] = msg
	}

	return msgContents
}
