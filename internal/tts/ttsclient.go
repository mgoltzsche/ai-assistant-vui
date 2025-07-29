package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	URL    string
	Model  string
	Client *http.Client
	APIKey string
}

func (c *Client) GenerateAudio(ctx context.Context, msg string) (io.ReadCloser, error) {
	params := map[string]interface{}{
		"input": msg,
		"model": c.Model,
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal speech generation params: %w", err)
	}

	req, err := http.NewRequest("POST", c.URL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build speech generation request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("generate speech: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("generate speech: server responded with %d", resp.StatusCode)
	}

	return resp.Body, nil
}
