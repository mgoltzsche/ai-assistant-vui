package chat

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tools"
	"github.com/tmc/langchaingo/llms"
)

type ChatCompletionRequest struct {
	RequestNum int64
}

type Completer struct {
	LLM         LLM
	Compaction  *ConversationCompaction
	IntroPrompt string
	Tools       tools.ToolProvider
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

				origPrompt := conv.SystemPrompt()

				conv.SetSystemPrompt(fmt.Sprintf("%s\n%s", origPrompt, c.IntroPrompt))
				conv.AddUserRequest(llms.TextPart("Hi")) // LocalAI 4 requires user message

				err := c.LLM.ChatCompletion(ctx, conv.RequestCounter(), nil, conv, ch)
				if err != nil {
					slog.Error("failed to generate greeting", "err", err)
				}

				ch <- ResponseChunk{
					Type:       model.MessageTypeEnd,
					RequestNum: conv.RequestCounter(),
				}

				conv.SetSystemPrompt(origPrompt)
			}()
		}

		for req := range requests {
			toolDefs, err := c.Tools.Tools(ctx)
			if err != nil {
				slog.Error("failed to load tools", "err", err)

				ch <- ResponseChunk{
					Type:       model.MessageTypeChunk,
					RequestNum: req.RequestNum,
					Text:       fmt.Sprintf("WARNING: cannot access tools: %s", err),
				}
			}

			for _, agent := range c.Agents {
				toolDefs = append(toolDefs, agent.AsTool(req.RequestNum, ch))
			}

			err = c.LLM.ChatCompletion(ctx, req.RequestNum, toolDefs, conv, ch)
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

			err = c.Compaction.RunCompaction(ctx, conv, func(ctx context.Context, warmupConv *model.Conversation) error {
				return c.LLM.WarmupChatCompletion(ctx, req.RequestNum, toolDefs, conv)
			})
			if err != nil {
				slog.Error("conversation compaction failed", "err", err)
			}
		}
	}()

	return ch, nil
}
