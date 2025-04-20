package server

import (
	"context"
	"io"

	"github.com/coder/websocket"
)

var _ io.Writer = &websocketWriter{}

type websocketWriter struct {
	Ctx       context.Context
	Websocket *websocket.Conn
}

func (w *websocketWriter) Write(b []byte) (int, error) {
	err := w.Websocket.Write(w.Ctx, websocket.MessageBinary, b)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}
