package tools

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
)

type ToolProvider interface {
	Tools(ctx context.Context) ([]Tool, error)
}

type Tool interface {
	Definition() llms.FunctionDefinition
	Call(ctx context.Context, params string) (string, error)
}

func FindByName(name string, tools []Tool) (Tool, error) {
	for _, f := range tools {
		if f.Definition().Name == name {
			return f, nil
		}
	}

	return nil, fmt.Errorf("tool %q not found", name)
}

type noop string

func Noop() ToolProvider {
	return noop("noop-tool-provider")
}

func (_ noop) Tools(_ context.Context) ([]Tool, error) {
	return nil, nil
}
