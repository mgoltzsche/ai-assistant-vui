package chat

import (
	"bytes"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
)

// ChunksToSentences receives a stream of chunks and returns a stream of sentences.
func ChunksToSentences(chunks <-chan ResponseChunk) <-chan ResponseChunk {
	ch := make(chan ResponseChunk)

	go func() {
		defer close(ch)

		var buf bytes.Buffer

		for chunk := range chunks {
			switch chunk.Type {
			case model.MessageTypeChunk:
				if chunk.UserOnly {
					ch <- chunk
					continue
				}

				buf.WriteString(chunk.Text)

				if sentences := splitIntoSentences(buf.String()); len(sentences) > 1 {
					for _, sentence := range sentences[:len(sentences)-1] {
						ch <- ResponseChunk{
							Type:       model.MessageTypeChunk,
							RequestNum: chunk.RequestNum,
							Text:       sentence,
						}
					}

					buf.Reset()

					lastSentencePrefix := sentences[len(sentences)-1]
					if endsWithPunctuationMark(lastSentencePrefix) {
						ch <- ResponseChunk{
							Type:       model.MessageTypeChunk,
							RequestNum: chunk.RequestNum,
							Text:       lastSentencePrefix,
						}
					} else {
						buf.WriteString(lastSentencePrefix)
					}
				}
			case model.MessageTypeEnd:
				if buf.Len() > 0 {
					ch <- ResponseChunk{
						Type:       model.MessageTypeChunk,
						RequestNum: chunk.RequestNum,
						Text:       strings.TrimSuffix(buf.String(), "</s>"),
					}
				}

				buf.Reset()

				ch <- chunk
			default:
				slog.Warn(fmt.Sprintf("chunks to sentences: unsupported chunk type %q", chunk.Type))
			}
		}
	}()

	return ch
}

func endsWithPunctuationMark(s string) bool {
	r := []rune(s)
	if len(r) == 0 {
		return false
	}

	c := r[len(r)-1]

	return c == '.' || c == '?' || c == '!'
}

var endOfSentenceRegex = regexp.MustCompile(`\n\s*|(\.|\?|!)+(\s+|$)`)

// SplitIntoSentences splits the given message at punctuation marks.
// This is to make the response appear to be streamed when LocalAI doesn't return a streamed response.
// Processing the response sentence by sentence reduces the time to the first response and allows the user to interrupt the AI verbally between each spoken sentence.
// See https://github.com/mudler/LocalAI/issues/1187
func SplitIntoSentences(msg string) []string {
	sentences := splitIntoSentences(msg)
	result := make([]string, 0, len(sentences))

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence != "" {
			result = append(result, sentence)
		}
	}

	return result
}

// splitIntoSentences splits a given message at punctuation marks, preserving whitespaces.
func splitIntoSentences(msg string) []string {
	m := endOfSentenceRegex.FindAllStringIndex(msg, -1)
	sentences := make([]string, len(m))
	pos := 0

	for i, idx := range m {
		sentences[i] = msg[pos:idx[1]]
		pos = idx[1]
	}

	if pos < len(msg) && len(msg[pos:]) > 0 {
		sentences = append(sentences, msg[pos:])
	}

	return sentences
}
