package functions

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

type FunctionCallChecker interface {
	IsFunctionCallAllowed(name string, args map[string]any) (bool, error)
}

type CallLoopPreventingProvider struct {
	delegate    FunctionProvider
	bannedNames map[string]struct{}
	calls       map[string]struct{}
}

func NewCallLoopPreventingProvider(p FunctionProvider) *CallLoopPreventingProvider {
	return &CallLoopPreventingProvider{
		delegate:    p,
		bannedNames: map[string]struct{}{},
		calls:       map[string]struct{}{},
	}
}

func (p *CallLoopPreventingProvider) IsFunctionCallAllowed(name string, args map[string]any) (bool, error) {
	argsCopy := make(map[string]any, len(args))
	for k, v := range args {
		if k != "rationale" {
			argsCopy[k] = v
		}
	}
	b, err := json.Marshal(argsCopy)
	if err != nil {
		return false, fmt.Errorf("marshal %s function call args: %w", name, err)
	}

	callSignature := fmt.Sprintf("%s(%s)", name, string(b))

	if _, alreadyCalled := p.calls[callSignature]; alreadyCalled {
		slog.Warn(fmt.Sprintf("disabling %s tool temporarily due to duplicate call", name))
		p.bannedNames[name] = struct{}{}

		return false, nil
	}

	p.calls[callSignature] = struct{}{}

	return true, nil
}

func (p *CallLoopPreventingProvider) Functions() ([]Function, error) {
	fns, err := p.delegate.Functions()
	if err != nil {
		return nil, err
	}

	filtered := make([]Function, 0, len(fns))

	for _, f := range fns {
		if _, ok := p.bannedNames[f.Definition().Name]; !ok {
			filtered = append(filtered, f)
		}
	}

	return filtered, nil
}
