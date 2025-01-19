package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	openai "github.com/sashabaranov/go-openai"
)

type Request = model.Request

type Completer struct {
	ServerURL           string
	Model               string
	Temperature         float32
	FrequencyPenalty    float32
	MaxTokens           int
	StripResponsePrefix string
	HTTPClient          openai.HTTPDoer
	client              *openai.Client
}

func (c *Completer) GenerateResponseText(ctx context.Context, requests <-chan Request, conv *model.Conversation) <-chan model.ResponseChunk {
	c.client = openai.NewClientWithConfig(openai.ClientConfig{
		BaseURL:            c.ServerURL + "/v1",
		HTTPClient:         c.HTTPClient,
		EmptyMessagesLimit: 10,
	})

	ch := make(chan model.ResponseChunk, 50)

	go func() {
		defer close(ch)

		err := c.createOpenAIChatCompletion(ctx, conv.Messages(), conv.RequestCounter(), conv, ch)
		if err != nil {
			log.Println("ERROR: chat completion:", err)
		}

		for request := range requests {
			if conv.RequestCounter() > request.ID {
				continue // skip outdated request (user requested something else)
			}

			log.Println("user request:", request.Text)

			messages := conv.AddMessage(openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: request.Text,
			})

			err := c.createOpenAIChatCompletion(ctx, messages, request.ID, conv, ch)
			if err != nil {
				log.Println("ERROR: chat completion:", err)
			}
		}
	}()

	return ch
}

func (c *Completer) createOpenAIChatCompletion(ctx context.Context, msgs []openai.ChatCompletionMessage, reqID int64, conv *model.Conversation, ch chan<- model.ResponseChunk) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:            c.Model,
		Temperature:      c.Temperature,
		FrequencyPenalty: c.FrequencyPenalty,
		MaxTokens:        c.MaxTokens,
		Messages:         msgs,
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return err
	}
	defer stream.Close()

	var buf bytes.Buffer
	lastContent := ""
	lastSentence := ""

	for {
		response, e := stream.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			if !errors.Is(e, context.Canceled) {
				err = fmt.Errorf("streaming chat completion response: %w", e)
			}
			break
		}

		if conv.RequestCounter() > reqID {
			// Cancel response stream if request is outdated (user requested something else)
			cancel()
			continue
		}

		content := response.Choices[0].Delta.Content

		buf.WriteString(content)

		// TODO: don't emit separate event for numbered list items, e.g. 3. ?!
		if buf.Len() > len(content)+1 && (content == "\n" || content == " " && (lastContent == "." || lastContent == "!" || lastContent == "?")) {
			sentence := buf.String()

			if strings.TrimSpace(sentence) != "" {
				if sentence == lastSentence {
					// Cancel response stream if last sentence was repeated
					cancel()
					continue
				}

				lastSentence = sentence

				ch <- model.ResponseChunk{
					RequestID: reqID,
					Text:      sentence,
				}
			}

			buf.Reset()
		}

		lastContent = content
	}

	if buf.Len() > 0 {
		ch <- model.ResponseChunk{
			RequestID: reqID,
			Text:      strings.TrimSuffix(buf.String(), "</s>"),
		}
	}

	return err
}

func (c *Completer) AddResponsesToConversation(sentences <-chan model.ResponseChunk, conv *model.Conversation) <-chan struct{} {
	ch := make(chan struct{})

	go func() {
		defer close(ch)

		for sentence := range sentences {
			log.Println("assistant:", strings.TrimSpace(sentence.Text))

			conv.AddMessage(openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: strings.TrimSpace(strings.TrimPrefix(sentence.Text, c.StripResponsePrefix)),
			})
		}
	}()

	return ch
}
