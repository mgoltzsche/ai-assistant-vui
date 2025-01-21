package chat

import (
	"sync"
)

func MergeCompletionRequests(sources ...<-chan ChatCompletionRequest) <-chan ChatCompletionRequest {
	ch := make(chan ChatCompletionRequest, 10)
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
