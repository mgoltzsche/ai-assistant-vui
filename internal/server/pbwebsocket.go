package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/mgoltzsche/ai-assistant-vui/internal/channel"
	"github.com/mgoltzsche/ai-assistant-vui/internal/generated/api/chat"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"google.golang.org/protobuf/proto"
)

func WebsocketHandler(c *channel.Channel) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slog.Info("Accepting websocket connection")

		ws, err := websocket.Accept(w, req, nil)
		if err != nil {
			errMsg := fmt.Sprintf("accept websocket connection: %s", err)
			slog.Warn(errMsg)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		defer ws.CloseNow()

		go func() {
			err := readWebsocketMessage(req.Context(), ws, c)
			if err != nil {
				slog.Error("failed to read websocket message", "err", err)
				return
			}
		}()

		err = streamAgentEvents(req.Context(), c, ws)
		if err != nil {
			slog.Warn("failed to send websocket message", "err", err)
		}
	})
}

func readWebsocketMessage(ctx context.Context, ws *websocket.Conn, out channel.Publisher) error {
	for {
		ws.SetReadLimit(-1)
		msgType, reader, err := ws.Reader(ctx)
		if err != nil {
			return fmt.Errorf("read websocket message: %w", err)
		}

		if msgType != websocket.MessageBinary {
			return fmt.Errorf("invalid message type %q received", msgType)
		}

		b, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		pbMsg := chat.Message{}
		err = proto.Unmarshal(b, &pbMsg)
		if err != nil {
			return err
		}

		slog.Info("received pb message", "len", len(pbMsg.GetAudioMessage()), "text", pbMsg.GetTextMessage())

		out.Publish(model.AudioMessage{
			WaveData: pbMsg.GetAudioMessage(),
		})
	}
}

func streamAgentEvents(ctx context.Context, c channel.Subscriber, ws *websocket.Conn) error {
	s := c.Subscribe(ctx)
	defer s.Stop()

	ch := s.ResultChan()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				break
			}

			pbMsg := chat.Message{}
			pbMsg.SetRole(chat.Role_ASSISTANT)
			if msg.Text != "" {
				pbMsg.SetTextMessage(msg.Text)
			}
			if len(msg.WaveData) > 0 {
				pbMsg.SetAudioMessage(msg.WaveData)
			}

			b, err := proto.Marshal(&pbMsg)
			if err != nil {
				return err
			}

			err = ws.Write(ctx, websocket.MessageBinary, b)
			if err != nil {
				return err
			}
		}
	}

	if ctx.Err() != nil {
		slog.Debug("request was cancelled")
	}

	return nil
}
