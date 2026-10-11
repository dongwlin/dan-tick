package danmaku

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func waitForQueue(t *testing.T, sender *Sender, want int) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		sender.mu.Lock()
		got := len(sender.queue)
		sender.mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-timeout.C:
			t.Fatalf("FIFO waiters: got %d, want %d", got, want)
		case <-tick.C:
		}
	}
}

func TestSenderFIFOIncludesTestSendAndHonorsAccountInterval(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	started := make(chan string, 3)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	sender := newSender(sendFunc(func(ctx context.Context, room, message string) (Result, error) {
		started <- message
		if message == "first" {
			<-release
		}
		return Result{Outcome: Accepted}, nil
	}), accountInterval, clock.Now, clock.Wait)
	done := make(chan error, 3)
	go func() {
		_, err := sender.Send(context.Background(), "42", "first")
		done <- err
	}()
	require.Equal(t, "first", <-started)

	go func() {
		_, err := sender.TestSend(context.Background(), "42", "test-send")
		done <- err
	}()
	waitForQueue(t, sender, 1)
	go func() {
		_, err := sender.Send(context.Background(), "42", "third")
		done <- err
	}()
	waitForQueue(t, sender, 2)
	once.Do(func() { close(release) })

	require.Equal(t, "test-send", <-started)
	require.Equal(t, "third", <-started)
	for range 3 {
		require.NoError(t, <-done)
	}
	require.Equal(t, []time.Duration{accountInterval, accountInterval}, clock.delays)
}

func TestSenderFIFOUnlinksCanceledMiddleWaiter(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	started := make(chan string, 3)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	sender := newSender(sendFunc(func(ctx context.Context, room, message string) (Result, error) {
		started <- message
		if message == "first" {
			<-release
		}
		return Result{Outcome: Accepted}, nil
	}), accountInterval, clock.Now, clock.Wait)
	done := make(chan error, 2)
	go func() {
		_, err := sender.Send(context.Background(), "42", "first")
		done <- err
	}()
	require.Equal(t, "first", <-started)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelled := make(chan Result, 1)
	go func() {
		result, _ := sender.TestSend(ctx, "42", "cancelled")
		cancelled <- result
	}()
	waitForQueue(t, sender, 1)
	go func() {
		_, err := sender.Send(context.Background(), "42", "third")
		done <- err
	}()
	waitForQueue(t, sender, 2)
	cancel()
	require.Equal(t, NotSent, (<-cancelled).Outcome)
	waitForQueue(t, sender, 1)
	once.Do(func() { close(release) })
	require.Equal(t, "third", <-started)
	for range 2 {
		require.NoError(t, <-done)
	}
	require.Equal(t, []time.Duration{accountInterval}, clock.delays)
	select {
	case unexpected := <-started:
		t.Fatalf("cancelled request used the slot: %s", unexpected)
	default:
	}
}

func TestSenderAdmissionFailureDoesNotSendOrConsumeSlot(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	calls := 0
	sender := newSender(sendFunc(func(ctx context.Context, room, message string) (Result, error) {
		calls++
		return Result{Outcome: Accepted}, nil
	}), accountInterval, clock.Now, clock.Wait)
	failure := errors.New("persistence failed")
	result, err := sender.SendWithAdmission(context.Background(), "42", "not sent", func(context.Context) error {
		return failure
	})
	require.ErrorIs(t, err, failure)
	require.Equal(t, NotSent, result.Outcome)
	require.Zero(t, calls)
	result, err = sender.TestSend(context.Background(), "42", "sent")
	require.NoError(t, err)
	require.Equal(t, Accepted, result.Outcome)
	require.Equal(t, 1, calls)
	require.Empty(t, clock.delays)
}
