package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/mgoltzsche/ai-agent-vui/internal/model"
	openai "github.com/sashabaranov/go-openai"
)

type CompletionChunk struct {
	RequestID int64
	Text      string
}

type Completer struct {
	ServerURL           string
	Model               string
	Temperature         float32
	FrequencyPenalty    float32
	MaxTokens           int
	SystemPrompt        string
	StripResponsePrefix string
	HTTPClient          openai.HTTPDoer
	client              *openai.Client
}

func (c *Completer) GenerateResponseText(ctx context.Context, requests <-chan model.Request, conv *model.ConversationContext, responses <-chan string) <-chan CompletionChunk {
	ch := make(chan CompletionChunk, 50)

	go func() {
		defer close(ch)

		messages := make([]openai.ChatCompletionMessage, 1, 100)
		messages[0] = openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: c.SystemPrompt,
		}

		for {
			select {
			case request, ok := <-requests:
				if !ok {
					return // terminate
				}

				if conv.RequestCounter() > request.ID {
					continue // skip outdated request (user requested something else)
				}

				log.Println("user request:", request.Text)

				messages = append(messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: request.Text,
				})
				err := c.createOpenAIChatCompletion(ctx, messages, request.ID, conv, ch)
				if err != nil {
					log.Println("ERROR: chat completion:", err)
				}
			case spokenSentence := <-responses:
				spokenSentence = strings.TrimSpace(strings.TrimPrefix(spokenSentence, c.StripResponsePrefix))

				log.Println("assistant:", spokenSentence)

				if messages[len(messages)-1].Role == openai.ChatMessageRoleAssistant {
					// TODO: use original message without trimmed space here
					messages[len(messages)-1].Content += " " + spokenSentence
				} else {
					messages = append(messages, openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: spokenSentence,
					})
				}
			}
		}
	}()

	return ch
}

func (c *Completer) createOpenAIChatCompletion(ctx context.Context, msgs []openai.ChatCompletionMessage, reqID int64, conv *model.ConversationContext, ch chan<- CompletionChunk) error {
	if c.client == nil {
		c.client = openai.NewClientWithConfig(openai.ClientConfig{
			BaseURL:            c.ServerURL + "/v1",
			HTTPClient:         c.HTTPClient,
			EmptyMessagesLimit: 10,
		})
	}

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

				ch <- CompletionChunk{
					RequestID: reqID,
					Text:      sentence,
				}
			}

			buf.Reset()
		}

		lastContent = content
	}

	if buf.Len() > 0 {
		ch <- CompletionChunk{
			RequestID: reqID,
			Text:      strings.TrimSuffix(buf.String(), "</s>"),
		}
	}

	return err
}
