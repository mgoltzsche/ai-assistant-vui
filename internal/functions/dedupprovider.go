package functions

import (
	"fmt"
	"log/slog"
)

type FunctionCallChecker interface {
	IsFunctionCallAllowed(name string, args map[string]any) (bool, error)
}

type CallLoopPreventingProvider struct {
	functions   []Function
	bannedNames map[string]struct{}
	calls       map[string]struct{}
}

func NewCallLoopPreventingProvider(fns []Function) *CallLoopPreventingProvider {
	return &CallLoopPreventingProvider{
		functions:   fns,
		bannedNames: map[string]struct{}{},
		calls:       map[string]struct{}{},
	}
}

func (p *CallLoopPreventingProvider) IsFunctionCallAllowed(name string, args map[string]any) (bool, error) {
	callSignature := name

	if _, alreadyCalled := p.calls[callSignature]; alreadyCalled {
		slog.Warn(fmt.Sprintf("disabling %s tool temporarily due to duplicate call", name))
		p.bannedNames[name] = struct{}{}

		return false, nil
	}

	p.calls[callSignature] = struct{}{}

	return true, nil
}

func (p *CallLoopPreventingProvider) Functions() ([]Function, error) {
	fns := p.functions
	filtered := make([]Function, 0, len(fns))

	for _, f := range fns {
		if _, ok := p.bannedNames[f.Definition().Name]; !ok {
			filtered = append(filtered, f)
		}
	}

	return filtered, nil
}
