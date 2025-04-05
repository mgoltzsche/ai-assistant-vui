package chat

import (
	"sync"
)

func MergeChannels[T any](sources ...<-chan T) <-chan T {
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
