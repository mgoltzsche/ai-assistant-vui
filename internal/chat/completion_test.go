package chat

import (
	"testing"

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
			expected: []string{},
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
			input:    "A sentence... a question? Another sentence!",
			expected: []string{"A sentence... ", "a question? ", "Another sentence!"},
		},
		{
			name:     "sentences with whitespaces",
			input:    "  A sentence...   a question?  Another sentence!  ",
			expected: []string{"A sentence... ", "a question? ", "Another sentence!"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, splitIntoSentences(tc.input))
		})
	}
}
