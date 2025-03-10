package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func FromFile(path string) (Configuration, error) {
	var cfg Configuration

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	m := map[string]any{}

	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return cfg, fmt.Errorf("read config at %s: %w", path, err)
	}

	b, err = json.Marshal(m)
	if err != nil {
		return cfg, fmt.Errorf("load config: marshal config: %w", err)
	}

	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()

	err = d.Decode(&cfg)
	if err != nil {
		return cfg, fmt.Errorf("read config at %s: %w", path, err)
	}

	return cfg, nil
}
