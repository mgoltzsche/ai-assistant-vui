package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tmc/langchaingo/llms"
)

type FunctionCallChecker interface {
	IsFunctionCallAllowed(name string, args map[string]any) (bool, error)
}

type CallLoopPreventingProvider struct {
	tools       []Tool
	bannedNames map[string]struct{}
	calls       map[string]struct{}
}

func NewCallLoopPreventingProvider(fns []Tool) *CallLoopPreventingProvider {
	return &CallLoopPreventingProvider{
		tools:       fns,
		bannedNames: map[string]struct{}{},
		calls:       map[string]struct{}{},
	}
}

func (p *CallLoopPreventingProvider) IsToolCallAllowed(call *llms.FunctionCall) (bool, error) {
	callSignature := call.Name

	if _, alreadyCalled := p.calls[callSignature]; alreadyCalled {
		slog.Warn(fmt.Sprintf("disabling %s tool temporarily due to duplicate call", callSignature))
		p.bannedNames[call.Name] = struct{}{}

		return false, nil
	}

	p.calls[callSignature] = struct{}{}

	return true, nil
}

func (p *CallLoopPreventingProvider) Tools(_ context.Context) ([]Tool, error) {
	tools := p.tools
	filtered := make([]Tool, 0, len(tools))

	for _, f := range tools {
		if _, ok := p.bannedNames[f.Definition().Name]; !ok {
			filtered = append(filtered, f)
		}
	}

	return filtered, nil
}
