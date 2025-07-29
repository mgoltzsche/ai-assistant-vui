package chat

import (
	"testing"

	"github.com/mgoltzsche/ai-assistant-vui/internal/model"
	"github.com/stretchr/testify/require"
)

func TestSplitIntoSentences(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: []string{},
		},
		{
			name:     "whitespace",
			input:    " ",
			expected: []string{" "},
		},
		{
			name:     "word",
			input:    "word",
			expected: []string{"word"},
		},
		{
			name:     "domain",
			input:    "example.org",
			expected: []string{"example.org"},
		},
		{
			name:     "sentence",
			input:    "a sentence.",
			expected: []string{"a sentence."},
		},
		{
			name:     "sentences",
			input:    "A sentence. a question? Another sentence!",
			expected: []string{"A sentence. ", "a question? ", "Another sentence!"},
		},
		{
			name:     "takes multiple punctuation marks into account",
			input:    "A sentence... a question?? Another statement!!",
			expected: []string{"A sentence... ", "a question?? ", "Another statement!!"},
		},

		{
			name:     "sentences without punctuation mark suffix",
			input:    "  A sentence. a question? Another sentence",
			expected: []string{"  A sentence. ", "a question? ", "Another sentence"},
		},
		{
			name:     "multi-line",
			input:    "  A\nsentence.",
			expected: []string{"  A\n", "sentence."},
		},
		{
			name:     "sentences with line breaks",
			input:    "  A sentence.\n\na question? Another sentence",
			expected: []string{"  A sentence.\n\n", "a question? ", "Another sentence"},
		},

		{
			name:     "preserves whitespace",
			input:    "  A sentence...   a question?  Another sentence!  ",
			expected: []string{"  A sentence...   ", "a question?  ", "Another sentence!  "},
		},
		{
			name:  "real-world use case",
			input: "How can I assist you today, user? Please say 'Computer' to address me.",
			expected: []string{
				"How can I assist you today, user? ",
				"Please say 'Computer' to address me.",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, splitIntoSentences(tc.input))
		})
	}
}

func TestChunksToSentences(t *testing.T) {
	for _, tc := range []struct {
		name              string
		input             []string
		expected          []string
		expectedBeforeEnd []string
	}{
		{
			name:     "no chunk",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "empty empty",
			input:    []string{""},
			expected: []string{},
		},
		{
			name:     "whitespace",
			input:    []string{" "},
			expected: []string{" "},
		},
		{
			name:     "word",
			input:    []string{"word"},
			expected: []string{"word"},
		},
		{
			name:     "domain",
			input:    []string{"example.org"},
			expected: []string{"example.org"},
		},
		{
			name:     "sentence",
			input:    []string{"a sentence."},
			expected: []string{"a sentence."},
		},
		{
			name:     "sentence in multiple chunks",
			input:    []string{"a", " sentence", "."},
			expected: []string{"a sentence."},
		},
		{
			name:     "sentences in one chunk",
			input:    []string{"A sentence... a question? Another sentence!"},
			expected: []string{"A sentence... ", "a question? ", "Another sentence!"},
		},
		{
			name:     "sentences in multiple chunks",
			input:    []string{"A sentence... a", " question? Another", " sentence!"},
			expected: []string{"A sentence... ", "a question? ", "Another sentence!"},
		},
		{
			name:     "sentences with whitespaces",
			input:    []string{"  A sentence...   a question?  Another sentence!  "},
			expected: []string{"  A sentence...   ", "a question?  ", "Another sentence!  "},
		},
		{
			name:     "sentences without punctuation mark suffix",
			input:    []string{"  A sentence. a question? Another sentence"},
			expected: []string{"  A sentence. ", "a question? ", "Another sentence"},
		},
		{
			name:     "sentences line break separated",
			input:    []string{"  A\nsentence."},
			expected: []string{"  A\n", "sentence."},
		},
		{
			name:              "sentences line break separated - emit immediately when ends with punctuation mark",
			input:             []string{"  A\nsentence."},
			expectedBeforeEnd: []string{"  A\n", "sentence."},
			expected:          []string{"  A\n", "sentence."},
		},
		{
			name:     "sentences with line breaks",
			input:    []string{"  A sentence.\n\na question? Another sentence"},
			expected: []string{"  A sentence.\n\n", "a question? ", "Another sentence"},
		},
		{
			name:              "sentences with line breaks - event emission delay if punctuation mark missing",
			input:             []string{"  A sentence.\n\na question? Another sentence"},
			expectedBeforeEnd: []string{"  A sentence.\n\n", "a question? "},
			expected:          []string{"  A sentence.\n\n", "a question? ", "Another sentence"},
		},
		{
			name: "real-world use case",
			input: []string{
				"How", " can", " I", " assist", " you", " today", ",",
				" user", "?", " Please", " say", " '", "Computer", "'", " to", " address", " me", ".",
			},
			expected: []string{
				"How can I assist you today, user? ",
				"Please say 'Computer' to address me.",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan ResponseChunk)
			wait := make(chan struct{})

			go func() {
				defer close(ch)

				for _, input := range tc.input {
					ch <- ResponseChunk{
						Type:       model.MessageTypeChunk,
						RequestNum: 1,
						Text:       input,
					}
				}

				ch <- ResponseChunk{
					Type:       model.MessageTypeChunk,
					RequestNum: 1,
					UserOnly:   true,
					Text:       "fake user-only message",
				}

				<-wait

				ch <- ResponseChunk{
					Type:       model.MessageTypeEnd,
					RequestNum: 1,
				}
			}()

			actual := make([]string, 0, 3)
			isLastMsg := false
			var userOnlyMessage *ResponseChunk

			for sentence := range ChunksToSentences(ch) {
				require.False(t, isLastMsg, "there shouldn't be an event emitted after the last message")
				require.Equal(t, int64(1), sentence.RequestNum, "chunk.requestNum")

				if sentence.UserOnly {
					s := sentence
					userOnlyMessage = &s

					if len(tc.expectedBeforeEnd) > 0 {
						require.Equal(t, tc.expectedBeforeEnd, actual, "should emit last chunk immediately when ends with punctuation mark")
					}

					wait <- struct{}{}
					continue
				}

				if sentence.Type == model.MessageTypeChunk {
					actual = append(actual, sentence.Text)
				} else {
					require.Equal(t, sentence.Type, model.MessageTypeEnd, "last message's type")
					isLastMsg = true
				}
			}

			require.Equal(t, tc.expected, actual)
			require.NotNil(t, userOnlyMessage, "emitted user-only message")
			require.Equal(t, "fake user-only message", userOnlyMessage.Text, "userOnlyMessage.Text")
		})
	}
}
