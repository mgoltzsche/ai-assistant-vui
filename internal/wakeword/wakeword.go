package wakeword

import (
	"fmt"
	"log"
	"regexp"

	"github.com/mgoltzsche/ai-agent-vui/internal/model"
)

// TODO: use openwakeword or porcupine instead? See:
// * https://picovoice.ai/docs/quick-start/porcupine-go/
// * https://github.com/charithe/porcupine-go
// * https://github.com/dscripka/openWakeWord/
// * https://community.rhasspy.org/t/openwakeword-new-library-and-pre-trained-models-for-wakeword-and-phrase-detection/4162

type Request = model.Request

type Filter struct {
	WakeWord     string
	SystemPrompt string
}

func (f *Filter) FilterByWakeWord(requests <-chan Request) (<-chan Request, *model.ConversationContext) {
	regex := regexp.MustCompile(fmt.Sprintf("%[1]s|^%[1]s,.+|.+, ?%[1]s$|.+, ?%[1]s,.+", regexp.QuoteMeta(f.WakeWord)))

	var counter int64

	ch := make(chan Request, 5)

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

	return ch, model.NewConversationContext(&counter, f.SystemPrompt)
}
