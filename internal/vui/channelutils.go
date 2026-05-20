package vui

import (
	"sync"
)

func mergeChannels[T any](sources ...<-chan T) <-chan T {
	ch := make(chan T, 10)
	wg := &sync.WaitGroup{}

	wg.Add(len(sources))

	for _, src := range sources {
		go func() {
			for req := range src {
				ch <- req
			}

			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	return ch
}

func splitChannel[T any](src <-chan T, cond func(T) bool) (<-chan T, <-chan T) {
	ch1 := make(chan T, 10)
	ch2 := make(chan T, 10)

	go func() {
		defer close(ch1)
		defer close(ch2)

		for msg := range src {
			if cond(msg) {
				ch1 <- msg
			} else {
				ch2 <- msg
			}
		}
	}()

	return ch1, ch2
}
