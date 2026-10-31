package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromFileTopicFadeDuration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		yaml     string
		expected Duration
	}{
		{
			name:     "defaults to 30m when unset",
			yaml:     "serverURL: http://localhost:8080\n",
			expected: DefaultTopicFadeDuration,
		},
		{
			name:     "explicit 0s disables fading",
			yaml:     "topicFadeDuration: 0s\n",
			expected: 0,
		},
		{
			name:     "explicit duration string is used",
			yaml:     "topicFadeDuration: 1h\n",
			expected: Duration(time.Hour),
		},
		{
			name:     "nanoseconds number is accepted",
			yaml:     "topicFadeDuration: 60000000000\n",
			expected: Duration(time.Minute),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0600))

			cfg, err := FromFile(path)
			require.NoError(t, err)

			assert.Equal(t, tc.expected, cfg.TopicFadeDuration)
		})
	}
}
