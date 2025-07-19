package channel

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-audio/wav"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/pubsub"
	"github.com/mgoltzsche/ai-assistant-vui/internal/vui"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

type AudioMessage = model.AudioMessage
type Subscriber = pubsub.Subscriber[AudioMessage]
type Publisher = pubsub.Publisher[AudioMessage]

type Channel struct {
	input  chan<- AudioMessage
	output *pubsub.PubSub[AudioMessage]
	cancel context.CancelFunc
}

func newChannel(ctx context.Context, cfg config.Configuration, client *http.Client) (*Channel, error) {
	ctx, cancel := context.WithCancel(ctx)
	input := make(chan AudioMessage, 5)
	c := &Channel{
		input:  input,
		output: pubsub.New[AudioMessage](),
		cancel: cancel,
	}

	output, conversation, err := vui.AudioPipeline(ctx, cfg, input)
	if err != nil {
		return nil, fmt.Errorf("start conversation: %w", err)
	}

	go func() {
		defer cancel()
		defer c.output.Stop()

		for m := range output {
			if m.RequestNum < conversation.RequestCounter() {
				continue
			}

			duration, err := audioDuration(m.WaveData)
			if err != nil {
				log.Println("ERROR:", err)
				continue
			}

			if m.UserOnly || conversation.AddAIResponse(m.RequestNum, m.Text) {
				if m.UserOnly {
					log.Println("assistant:", m.Text)
				}

				c.output.Publish(m)
				time.Sleep(duration)
			}
		}
	}()

	return c, nil
}

func (c *Channel) Stop() {
	c.cancel()
	c.output.Stop()
	close(c.input)
}

func (c *Channel) Publish(msg AudioMessage) {
	c.input <- msg
}

func (c *Channel) Subscribe(ctx context.Context) pubsub.Subscription[AudioMessage] {
	return c.output.Subscribe(ctx)
}

func audioDuration(wave []byte) (time.Duration, error) {
	decoder := wav.NewDecoder(bytes.NewReader(wave))
	decoder.ReadInfo()
	if err := decoder.Err(); err != nil {
		return 0, fmt.Errorf("read wave file headers: %w", err)
	}

	audioDuration, err := decoder.Duration()
	if err != nil {
		return 0, fmt.Errorf("get audio duration from wave headers: %w", err)
	}

	return audioDuration, err
}
