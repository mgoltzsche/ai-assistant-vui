package model

type Message struct {
	RequestID int64
	Text      string
}

type AudioMessage struct {
	Message
	WaveData []byte
}
