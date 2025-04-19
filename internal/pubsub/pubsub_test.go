package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeEvent struct {
	Value string
}

func TestPubSub(t *testing.T) {
	testee := New[fakeEvent]()
	s := testee.Subscribe(context.Background())
	defer s.Stop()

	eventCount := 3

	go func() {
		for i := 0; i < eventCount; i++ {
			testee.Publish(fakeEvent{Value: fmt.Sprintf("fake value %d", i)})
		}
	}()

	time.Sleep(100 * time.Millisecond)

	go func() {
		time.Sleep(time.Second)
		s.Stop()
		testee.Publish(fakeEvent{Value: "event sent after stop"})
	}()

	expected := []string{"fake value 0", "fake value 1", "fake value 2"}
	actual := make([]string, 0, 3)

	for evt := range s.ResultChan() {
		require.NotNil(t, evt, "event")

		actual = append(actual, evt.Value)
	}

	require.Equal(t, expected, actual, "received events")
}
