package functions

import (
	"fmt"

	"github.com/tmc/langchaingo/llms"
)

type FunctionProvider interface {
	Functions() ([]Function, error)
}

type Function interface {
	Definition() llms.FunctionDefinition
	Call(params map[string]any) (string, error)
}

func FindByName(name string, functions []Function) (Function, error) {
	for _, f := range functions {
		if f.Definition().Name == name {
			return f, nil
		}
	}

	return nil, fmt.Errorf("function %q not found", name)
}

type noop string

func Noop() FunctionProvider {
	return noop("noop-function-provider")
}

func (_ noop) Functions() ([]Function, error) {
	return nil, nil
}
