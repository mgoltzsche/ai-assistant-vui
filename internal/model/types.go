package model

type MessageType string

const (
	MessageTypeChunk MessageType = "chunk"
	MessageTypeEnd   MessageType = "end"
)

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
