package model

type Message struct {
	RequestID int64
	Text      string
	UserOnly  bool
}

type AudioMessage struct {
	Message
	WaveData []byte
}
