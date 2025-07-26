package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/mgoltzsche/ai-assistant-vui/internal/functions"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
	"github.com/tmc/langchaingo/llms"
)

var _ functions.FunctionProvider = &Functions{}

type Functions struct {
	FunctionDefinitions []config.FunctionDefinition
}

func (f *Functions) Functions() ([]functions.Function, error) {
	r := make([]functions.Function, len(f.FunctionDefinitions))
	for i, cfunc := range f.FunctionDefinitions {
		r[i] = &function{FunctionDefinition: cfunc}
	}
	return r, nil
}

type function struct {
	config.FunctionDefinition
}

func (f *function) Definition() llms.FunctionDefinition {
	return f.FunctionDefinition.FunctionDefinition
}

func (f *function) Call(ctx context.Context, params map[string]any) (string, error) {
	c := f.Container

	timeout := c.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("failed to run function %q: create docker client: %w", f.Name, err)
	}
	defer cli.Close()

	reader, err := cli.ImagePull(ctx, c.Image, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to run function %q: pull image: %w", f.Name, err)
	}

	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)

	env := make([]string, 0, len(c.Env))
	for k, v := range c.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	for k, v := range params {
		env = append(env, fmt.Sprintf("PARAMETER_%s=%v", strings.ToUpper(k), v))
	}

	cfg := &container.Config{
		Image: c.Image,
		Cmd:   c.Args,
		Env:   env,
	}

	if c.Command != "" {
		cfg.Entrypoint = []string{c.Command}
	}

	resp, err := cli.ContainerCreate(ctx, cfg, nil, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("failed to run function %q: failed to create container: %w", f.Name, err)
	}

	defer func() {
		err := cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to remove function container: %s", err))
		}
	}()

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to run function %q: failed to start container: %w", f.Name, err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return "", fmt.Errorf("failed to run function %q: %w%s", f.Name, err, errDetails(ctx, resp.ID, cli))
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", fmt.Errorf("failed to run function %q: exited with %d%s", f.Name, status.StatusCode, errDetails(ctx, resp.ID, cli))
		}
	}

	out, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", fmt.Errorf("failed to read the output of function %q: %w", f.Name, err)
	}

	defer out.Close()

	var stdout, stderr bytes.Buffer

	_, err = stdcopy.StdCopy(&stdout, &stderr, out)
	if err != nil {
		return "", fmt.Errorf("failed to read the output of function %q: %w", f.Name, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line != "" {
			slog.Warn(fmt.Sprintf("function %s: %s", f.Name, line))
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

func errDetails(ctx context.Context, containerID string, c *client.Client) string {
	suffix := ""
	out, e := c.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStderr: true})
	if e == nil {
		defer out.Close()
		var stdout, stderr bytes.Buffer
		_, _ = stdcopy.StdCopy(&stdout, &stderr, out)
		errLog := strings.TrimSpace(stderr.String())
		if errLog != "" {
			suffix = fmt.Sprintf(", stderr: %s", errLog)
		}
	}
	return suffix
}
