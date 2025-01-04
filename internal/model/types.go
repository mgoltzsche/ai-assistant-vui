package model

type Request struct {
	ID   int64
	Text string
}

type ConversationContext struct {
	requestCounter *int64
}

func NewConversationContext(requestCounter *int64) *ConversationContext {
	return &ConversationContext{
		requestCounter: requestCounter,
	}
}

func (c *ConversationContext) RequestCounter() int64 {
	return *c.requestCounter
}
