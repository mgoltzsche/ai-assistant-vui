package wakeword

import (
	"fmt"
	"log"
	"regexp"

	"github.com/mgoltzsche/ai-agent-vui/internal/model"
)

type Filter struct {
	WakeWord string
}

func (f *Filter) FilterByWakeWord(requests <-chan model.Request) (<-chan model.Request, *model.ConversationContext) {
	regex := regexp.MustCompile(fmt.Sprintf("%[1]s|^%[1]s,.+|.+, ?%[1]s$|.+, ?%[1]s,.+", regexp.QuoteMeta(f.WakeWord)))

	var counter int64

	ch := make(chan model.Request, 5)

	go func() {
		defer close(ch)

		for req := range requests {
			if regex.MatchString(req.Text) {
				counter++
				req.ID = counter

				ch <- req
			} else {
				log.Println("user:", req.Text)
			}
		}
	}()

	return ch, model.NewConversationContext(&counter)
}
