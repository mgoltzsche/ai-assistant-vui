package wakeword

import (
	"fmt"
	"log/slog"
	"regexp"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

// TODO: use openwakeword or porcupine instead? (to avoid sending audio to whisper every time somebody talks) See:
// * https://picovoice.ai/docs/quick-start/porcupine-go/
// * https://github.com/charithe/porcupine-go
// * https://github.com/dscripka/openWakeWord/
// * https://community.rhasspy.org/t/openwakeword-new-library-and-pre-trained-models-for-wakeword-and-phrase-detection/4162

type Message = model.Message

type Filter struct {
	WakeWord string
}

func (f *Filter) FilterByWakeWord(requests <-chan Message) <-chan Message {
	regex := regexp.MustCompile(fmt.Sprintf(`(?i)(^|[^\w])%[1]s($|[^\w])`, regexp.QuoteMeta(f.WakeWord)))

	ch := make(chan Message, 5)

	go func() {
		defer close(ch)

		for req := range requests {
			if regex.MatchString(req.Text) {
				ch <- req
			} else {
				slog.Info(fmt.Sprintf("user: %s", req.Text))
			}
		}
	}()

	return ch
}
