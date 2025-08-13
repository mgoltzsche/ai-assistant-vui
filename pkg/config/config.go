package config

import (
	"time"

	"github.com/tmc/langchaingo/llms"
)

type Configuration struct {
	ServerURL    string            `json:"serverURL"`
	APIKey       string            `json:"apiKey"`
	InputDevice  string            `json:"inputDevice,omitempty"`
	OutputDevice string            `json:"outputDevice,omitempty"`
	MinVolume    int               `json:"minVolume,omitempty"`
	VADEnabled   bool              `json:"vadEnabled,omitempty"`
	VADModelPath string            `json:"vadModelPath,omitempty"`
	STTModel     string            `json:"sttModel,omitempty"`
	TTSModel     string            `json:"ttsModel,omitempty"`
	ChatModel    string            `json:"chatModel,omitempty"`
	Temperature  float64           `json:"temperature,omitempty"`
	WakeWord     string            `json:"wakeWord,omitempty"`
	IntroPrompt  string            `json:"introPrompt,omitempty"`
	Agents       []AgentDefinition `json:"agents,omitempty"`
	AgentDefinition
}

type FunctionDefinition struct {
	llms.FunctionDefinition
	Container
}

type Container struct {
	Image   string            `json:"image"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout time.Duration     `json:"timeout,omitempty"`
}

type AgentDefinition struct {
	Name        string               `json:"name,omitempty"`
	Description string               `json:"description,omitempty"`
	Prompt      []string             `json:"prompt,omitempty"`
	Functions   []FunctionDefinition `json:"tools,omitempty"`
}
