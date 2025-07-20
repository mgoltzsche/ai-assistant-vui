package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
)

type Publisher[E any] interface {
	Publish(evt E)
}

type Subscriber[E any] interface {
	Subscribe(ctx context.Context) Subscription[E]
}

type Subscription[E any] interface {
	ResultChan() <-chan E
	Stop()
}

type PubSub[E any] struct {
	mutex         sync.RWMutex
	subscriptions map[int64]*subscription[E]
	seq           int64
	stack         string
	stopped       bool
}

func New[E any]() *PubSub[E] {
	return &PubSub[E]{subscriptions: map[int64]*subscription[E]{}}
}

func (p *PubSub[E]) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.stopped = true

	for _, subscription := range p.subscriptions {
		subscription.cancel()
	}
}

func (p *PubSub[E]) Subscribe(ctx context.Context) Subscription[E] {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.stopped {
		return noopSubscription[E]("noop-subscription")
	}

	p.seq++

	buf := make([]byte, 1024)
	i := goruntime.Stack(buf, true)
	buf = buf[:i]
	ctx, cancel := context.WithCancel(ctx)
	s := &subscription[E]{
		id:     p.seq,
		cancel: cancel,
		pubsub: p,
		ch:     make(chan E, 10),
		stack:  string(buf),
	}
	p.subscriptions[s.id] = s

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	return s
}

func (p *PubSub[E]) Publish(evt E) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.stopped {
		return
	}

	for _, s := range p.subscriptions {
		select {
		case s.ch <- evt:
		case <-time.After(20 * time.Second):
			slog.Warn(fmt.Sprintf("kicking subscriber since it timed out accepting the event after 20s, subscriber stack trace:\n  %s", strings.ReplaceAll(s.stack, "\n", "\n  ")))
			go s.Stop()
		}
	}
}

type subscription[E any] struct {
	pubsub *PubSub[E]
	id     int64
	cancel context.CancelFunc
	ch     chan E
	stack  string
}

func (s *subscription[E]) Stop() {
	s.pubsub.mutex.Lock()
	delete(s.pubsub.subscriptions, s.id)
	ch := s.ch
	s.ch = nil
	s.pubsub.mutex.Unlock()
	if ch != nil {
		close(ch)
		s.cancel()
		for _ = range ch {
		}
	}
}

func (w *subscription[E]) ResultChan() <-chan E {
	return w.ch
}

type noopSubscription[E any] string

func (_ noopSubscription[E]) Stop() {}

func (_ noopSubscription[E]) ResultChan() <-chan E {
	ch := make(chan E, 0)
	close(ch)
	return ch
}
