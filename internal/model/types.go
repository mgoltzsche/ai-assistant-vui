package model

type MessageType string

const (
	MessageTypeChunk MessageType = "chunk"
	MessageTypeEnd   MessageType = "end"
)

type Role string

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

type Event struct {
	Role Role
	AudioMessage
}

type Message struct {
	Type       MessageType
	RequestNum int64
	Text       string
	UserOnly   bool
}

type AudioMessage struct {
	Message
	WaveData []byte
}
