package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// DefaultTopicFadeDuration is the topic fade duration applied by FromFile when the config file does not set topicFadeDuration.
const DefaultTopicFadeDuration = Duration(30 * time.Minute)

// Duration is a time.Duration that can be parsed from a string such as "30m" as well as from a number of nanoseconds.
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		*d = Duration(parsed)
		return nil
	}
	var ns int64
	if err := json.Unmarshal(b, &ns); err != nil {
		return fmt.Errorf("parse duration: %w", err)
	}
	*d = Duration(ns)
	return nil
}

func (d Duration) String() string {
	return time.Duration(d).String()
}

type Configuration struct {
	ServerURL         string               `json:"serverURL"`
	APIKey            string               `json:"apiKey"`
	InputDevice       string               `json:"inputDevice,omitempty"`
	OutputDevice      string               `json:"outputDevice,omitempty"`
	MinVolume         int                  `json:"minVolume,omitempty"`
	VADEnabled        bool                 `json:"vadEnabled,omitempty"`
	VADModelPath      string               `json:"vadModelPath,omitempty"`
	STTModel          string               `json:"sttModel,omitempty"`
	TTSModel          string               `json:"ttsModel,omitempty"`
	TTSVoice          string               `json:"ttsVoice,omitempty"`
	ChatModel         string               `json:"chatModel,omitempty"`
	Temperature       float64              `json:"temperature,omitempty"`
	WakeWord          string               `json:"wakeWord,omitempty"`
	IntroPrompt       string               `json:"introPrompt,omitempty"`
	TopicFadeDuration Duration             `json:"topicFadeDuration,omitempty"` // Defaults to DefaultTopicFadeDuration when unset in the config file, 0 disables topic fading.
	MCPServers        map[string]MCPServer `json:"mcpServers,omitempty"`
	Agents            []AgentDefinition    `json:"agents,omitempty"`
	AgentDefinition
}

type MCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
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
	Name        string              `json:"name,omitempty"`
	Description string              `json:"description,omitempty"`
	Prompt      []string            `json:"prompt,omitempty"`
	Tools       []MCPToolsReference `json:"tools,omitempty"`
}

type MCPToolsReference struct {
	MCPServer  string   `json:"mcpServer"`
	AllowTools []string `json:"allow"`
}
