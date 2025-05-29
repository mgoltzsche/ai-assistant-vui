package chat

import (
	"bytes"
	"context"
	"log"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

type responseParser struct {
	Cancel              context.CancelFunc
	ReqNum              int64
	Conversation        *model.Conversation
	StripResponsePrefix string
	Ch                  chan<- ResponseChunk

	buf          bytes.Buffer
	lastContent  string
	lastSentence string
	thinking     bool
}

func (p *responseParser) ConsumeChunk(ctx context.Context, chunk []byte) error {
	if p.Conversation.RequestCounter() > p.ReqNum {
		// Cancel response stream if request is outdated (user requested something else)
		p.Cancel()
		return nil
	}

	content := string(chunk)
	buf := &p.buf

	buf.WriteString(content)

	// TODO: don't emit separate event for numbered list items, e.g. 3. ?!
	if buf.Len() > len(content)+1 && (content == "\n" || content == " " && (p.lastContent == "." || p.lastContent == "!" || p.lastContent == "?")) {
		sentence := buf.String()

		defer buf.Reset()

		p.parseSentence(sentence)
	}

	p.lastContent = content

	return nil
}

func (p *responseParser) parseSentence(sentence string) {
	trimmed := strings.TrimSpace(sentence)

	if trimmed == "" {
		return
	}

	if sentence == p.lastSentence {
		log.Println("WARNING: Cancelling response stream since last sentence was repeated")
		p.Cancel()
		return
	}

	p.lastSentence = sentence

	if trimmed == "<think>" {
		p.thinking = true
		return
	}

	if p.thinking {
		pos := strings.Index(sentence, "</think>")
		thought := trimmed
		thoughtEnd := pos > -1

		if thoughtEnd {
			p.thinking = false
			thought = sentence[:pos]

			if len(sentence) > pos+8 {
				sentence = sentence[pos+8:]
			} else {
				sentence = ""
			}
		}

		for _, ts := range splitIntoSentences(thought) {
			log.Println("assistant (thinking):", ts)
		}

		if !thoughtEnd {
			return
		}
	}

	sentence = p.sanitizeMessage(sentence)

	for _, sentence := range splitIntoSentences(sentence) {
		p.Ch <- ResponseChunk{
			RequestNum: p.ReqNum,
			Text:       sentence,
		}
	}
}

func (p *responseParser) Complete() {
	p.parseSentence(strings.TrimSuffix(p.buf.String(), "</s>"))
}

func (p *responseParser) sanitizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.TrimPrefix(msg, p.StripResponsePrefix)
	return msg
}

// splitIntoSentences splits the given message at punctuation marks.
// This is to make the response appear to be streamed when LocalAI doesn't return a streamed response.
// Processing the response sentence by sentence reduces the time to the first response and allows the user to interrupt the AI verbally between each spoken sentence.
// See https://github.com/mudler/LocalAI/issues/1187
func splitIntoSentences(msg string) []string {
	m := endOfSentenceRegex.FindAllStringIndex(msg, -1)
	sentences := make([]string, len(m))
	pos := 0

	for i, idx := range m {
		sentences[i] = strings.TrimSpace(msg[pos:idx[1]]) + " "
		pos = idx[1]
	}

	if pos < len(msg) && len(strings.TrimSpace(msg[pos:])) > 0 {
		sentences = append(sentences, msg[pos:])
	}

	if len(sentences) > 0 {
		sentences[len(sentences)-1] = strings.TrimSpace(sentences[len(sentences)-1])
	}

	return sentences
}
