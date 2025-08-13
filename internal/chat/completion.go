package chat

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type ChatCompletionRequest struct {
	RequestNum int64
}

type Completer struct {
	LLM         LLM
	IntroPrompt string
	Tools       functions.FunctionProvider
	Agents      []Agent
}

func (c *Completer) Run(ctx context.Context, requests <-chan ChatCompletionRequest, conv *model.Conversation) (<-chan ResponseChunk, error) {
	ch := make(chan ResponseChunk, 50)

	go func() {
		defer close(ch)

		if c.IntroPrompt != "" {
			wg := &sync.WaitGroup{}
			wg.Add(1)

			defer wg.Wait()

			go func() {
				defer wg.Done()
				prompt := fmt.Sprintf("%s\n%s", conv.Messages()[0].Parts[0], c.IntroPrompt)
				welcomeConv := model.NewConversation(prompt, 1)

				err := c.LLM.ChatCompletion(ctx, conv.RequestCounter(), nil, welcomeConv, ch)
				if err != nil {
					slog.Error("failed to generate greeting", "err", err)
				}

				ch <- ResponseChunk{
					Type:       model.MessageTypeEnd,
					RequestNum: conv.RequestCounter(),
				}
			}()
		}

		for req := range requests {
			tools, err := c.Tools.Functions()
			if err != nil {
				slog.Error("failed to load tools", "err", err)

				ch <- ResponseChunk{
					Type:       model.MessageTypeChunk,
					RequestNum: req.RequestNum,
					Text:       fmt.Sprintf("WARNING: cannot access tools: %s", err),
				}
			}

			for _, agent := range c.Agents {
				tools = append(tools, agent.AsTool(req.RequestNum, ch))
			}

			err = c.LLM.ChatCompletion(ctx, req.RequestNum, tools, conv, ch)
			if err != nil {
				slog.Error("chat completion failed", "err", err)

				ch <- ResponseChunk{
					Type:       model.MessageTypeChunk,
					RequestNum: req.RequestNum,
					Text:       fmt.Sprintf("ERROR: Failed to generate AI response: %s", err),
				}
			}

			slog.Debug("end of response")

			ch <- ResponseChunk{
				Type:       model.MessageTypeEnd,
				RequestNum: req.RequestNum,
			}
		}
	}()

	return ch, nil
}
