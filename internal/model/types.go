package model

type Message struct {
	RequestNum int64
	Text       string
	UserOnly   bool
}

type AudioMessage struct {
	Message
	WaveData []byte
}
