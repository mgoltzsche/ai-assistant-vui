package channel

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/mgoltzsche/ai-assistant-vui/internal/tools/mcp"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

type Channels struct {
	ctx        context.Context
	channels   map[string]*Channel
	cfg        config.Configuration
	mcpServers mcp.Servers
	httpClient *http.Client
	mutex      *sync.Mutex
}

func NewChannels(ctx context.Context, cfg config.Configuration, mcpServers mcp.Servers) *Channels {
	return &Channels{
		channels:   map[string]*Channel{},
		httpClient: &http.Client{Timeout: 90 * time.Second},
		mutex:      &sync.Mutex{},
		cfg:        cfg,
		mcpServers: mcpServers,
		ctx:        ctx,
	}
}

func (r *Channels) GetOrCreate(id string) (*Channel, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	c, ok := r.channels[id]
	if !ok {
		c, err := newChannel(r.ctx, r.cfg, r.mcpServers, r.httpClient)
		if err != nil {
			return nil, err
		}

		r.channels[id] = c
		return c, nil
	}

	return c, nil
}
