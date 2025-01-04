package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type Response struct {
	Text string `json:"text"`
}

type Client struct {
	URL    string
	Model  string
	Client *http.Client
}

func (c *Client) Transcribe(ctx context.Context, wavData []byte) (Transcription, error) {
	var b bytes.Buffer
	multipartWriter := multipart.NewWriter(&b)

	part, err := multipartWriter.CreateFormFile("file", "input.wav")
	if err != nil {
		return Transcription{}, fmt.Errorf("creating multipart form file: %w", err)
	}

	_, err = part.Write(wavData)
	if err != nil {
		return Transcription{}, fmt.Errorf("write data to multipart writer: %w", err)
	}

	err = multipartWriter.WriteField("model", c.Model)
	if err != nil {
		return Transcription{}, fmt.Errorf("write multipart request field: %w", err)
	}

	err = multipartWriter.Close()
	if err != nil {
		return Transcription{}, fmt.Errorf("multipart writer close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.URL+"/v1/audio/transcriptions", &b)
	if err != nil {
		return Transcription{}, fmt.Errorf("new transcription request: %w", err)
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())

	return c.send(req)
}

func (c *Client) send(request *http.Request) (Transcription, error) {
	resp, err := c.Client.Do(request)
	if err != nil {
		return Transcription{}, err // Handle the error appropriately
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Transcription{}, fmt.Errorf("server responded with status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Transcription{}, fmt.Errorf("read body: %w", err)
	}

	var result Response
	err = json.Unmarshal(body, &result)
	if err != nil {
		return Transcription{}, fmt.Errorf("unmarshal body: %w", err)
	}

	return Transcription{
		Text: result.Text,
	}, nil
}
