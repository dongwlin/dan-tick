package danmaku

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	delays []time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(delay time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delay)
}

func (c *fakeClock) Wait(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.delays = append(c.delays, delay)
	c.now = c.now.Add(delay)
	c.mu.Unlock()
	return ctx.Err()
}

type sendFunc func(context.Context, string, string) (Result, error)

func (f sendFunc) Send(ctx context.Context, roomID, message string) (Result, error) {
	return f(ctx, roomID, message)
}

type credentialLoadFunc func() (credential.Credentials, error)

func (f credentialLoadFunc) Load() (credential.Credentials, error) { return f() }

func TestSenderPreservesHTTPIntervalAfterSlowCredentialRead(t *testing.T) {
	for _, readDelay := range []time.Duration{14 * time.Second, 30 * time.Second} {
		t.Run(readDelay.String(), func(t *testing.T) {
			clock := &fakeClock{now: time.Unix(1000, 0)}
			started := make(chan time.Time, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- clock.Now()
				_, _ = w.Write([]byte(`{"code":0}`))
			}))
			defer server.Close()

			loads := 0
			store := credentialLoadFunc(func() (credential.Credentials, error) {
				loads++
				if loads == 1 {
					clock.Advance(readDelay)
				}
				return credential.Credentials{SESSDATA: sessionSecret, BiliJCT: csrfSecret}, nil
			})
			client := NewClient(store, server.Client())
			client.endpoint = server.URL
			sender := newSender(client, accountInterval, clock.Now, clock.Wait)

			for _, send := range []func(context.Context, string, string) (Result, error){sender.Send, sender.TestSend} {
				result, err := send(context.Background(), "42", "example")
				require.NoError(t, err)
				require.Equal(t, Accepted, result.Outcome)
			}
			first, second := <-started, <-started
			require.Equal(t, accountInterval, second.Sub(first))
			require.Equal(t, []time.Duration{accountInterval}, clock.delays)
			require.Equal(t, 2, loads)
		})
	}
}

func TestSendAndTestSendShareAccountIntervalWithRealHTTP(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1760123456, 0)}
	var started []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "42", r.Form.Get("roomid"))
		require.Equal(t, "example", r.Form.Get("msg"))
		started = append(started, clock.Now())
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	store := &fakeStore{creds: credential.Credentials{SESSDATA: sessionSecret, BiliJCT: csrfSecret}}
	client := testClient(store, server)
	sender := newSender(client, accountInterval, clock.Now, clock.Wait)

	for _, call := range []func(context.Context, string, string) (Result, error){
		sender.TestSend, sender.Send, sender.TestSend,
	} {
		result, err := call(context.Background(), "42", "example")
		require.NoError(t, err)
		require.Equal(t, Accepted, result.Outcome)
		require.Equal(t, 0, *result.Code)
		safe(t, result, err)
	}
	require.Equal(t, 3, store.loads)
	require.Equal(t, []time.Duration{accountInterval, accountInterval}, clock.delays)
	require.Len(t, started, 3)
	require.Equal(t, accountInterval, started[1].Sub(started[0]))
	require.Equal(t, accountInterval, started[2].Sub(started[1]))
}

func TestTestSendReturnsActualHTTPRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":10031,"message":"too frequent"}`))
	}))
	defer server.Close()

	result, err := NewSender(testClient(storedCredentials(), server)).TestSend(context.Background(), "42", "example")
	require.ErrorIs(t, err, ErrRateLimited)
	require.Equal(t, Rejected, result.Outcome)
	require.Equal(t, 10031, *result.Code)
	safe(t, result, err)
}

func TestSenderSerializesCallsAndCancelsWhileWaitingForPermit(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	sender := newSender(sendFunc(func(ctx context.Context, roomID, message string) (Result, error) {
		started <- message
		if message == "first" {
			<-release
		}
		return Result{Outcome: Accepted}, nil
	}), accountInterval, clock.Now, clock.Wait)

	firstDone := make(chan error, 1)
	go func() {
		_, err := sender.Send(context.Background(), "42", "first")
		firstDone <- err
	}()
	require.Equal(t, "first", <-started)

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, err := sender.TestSend(ctx, "42", "cancelled")
		secondDone <- struct {
			result Result
			err    error
		}{result, err}
	}()
	cancel()
	second := <-secondDone
	require.Equal(t, NotSent, second.result.Outcome)
	require.ErrorIs(t, second.err, context.Canceled)
	select {
	case unexpected := <-started:
		t.Fatalf("cancelled call started an HTTP send: %s", unexpected)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-firstDone)

	result, err := sender.Send(context.Background(), "42", "next")
	require.NoError(t, err)
	require.Equal(t, Accepted, result.Outcome)
	require.Equal(t, "next", <-started)
	require.Equal(t, []time.Duration{accountInterval}, clock.delays)
}

func TestSenderCancelsWaitWithoutConsumingSlot(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	calls := 0
	waiting := make(chan time.Duration, 1)
	sender := newSender(sendFunc(func(ctx context.Context, roomID, message string) (Result, error) {
		calls++
		return Result{Outcome: Accepted}, nil
	}), accountInterval, clock.Now, func(ctx context.Context, delay time.Duration) error {
		waiting <- delay
		<-ctx.Done()
		return ctx.Err()
	})

	result, err := sender.Send(context.Background(), "42", "first")
	require.NoError(t, err)
	require.Equal(t, Accepted, result.Outcome)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, err := sender.TestSend(ctx, "42", "cancelled")
		done <- struct {
			result Result
			err    error
		}{result, err}
	}()
	require.Equal(t, accountInterval, <-waiting)
	cancel()
	cancelled := <-done
	require.Equal(t, NotSent, cancelled.result.Outcome)
	require.ErrorIs(t, cancelled.err, context.Canceled)
	require.Equal(t, 1, calls)

	// The canceled wait did not reserve a new time slot.
	clock.Advance(accountInterval)
	sender.wait = func(context.Context, time.Duration) error {
		return errors.New("unexpected wait after cancellation")
	}
	result, err = sender.Send(context.Background(), "42", "next")
	require.NoError(t, err)
	require.Equal(t, Accepted, result.Outcome)
	require.Equal(t, 2, calls)
}

func TestSenderPropagatesErrorsAndPreservesIntervals(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	responses := []struct {
		result Result
		err    error
	}{
		{Result{Outcome: Rejected, Code: intPointer(10031)}, ErrRateLimited},
		{Result{Outcome: Unknown}, ErrUncertain},
		{Result{Outcome: Accepted, Code: intPointer(0)}, nil},
	}
	calls := 0
	sender := newSender(sendFunc(func(ctx context.Context, roomID, message string) (Result, error) {
		answer := responses[calls]
		calls++
		clock.Advance(10 * time.Second)
		return answer.result, answer.err
	}), accountInterval, clock.Now, clock.Wait)

	for i, response := range responses {
		result, err := sender.TestSend(context.Background(), "42", fmt.Sprintf("attempt %d", i))
		require.Equal(t, response.result, result)
		require.ErrorIs(t, err, response.err)
	}
	require.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{accountInterval, accountInterval}, clock.delays)
}

func TestSenderDoesNotLimitKnownUnsentFailures(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	calls := 0
	sender := newSender(sendFunc(func(ctx context.Context, roomID, message string) (Result, error) {
		calls++
		if calls == 1 {
			return Result{Outcome: NotSent}, credential.ErrNotConfigured
		}
		return Result{Outcome: Accepted}, nil
	}), accountInterval, clock.Now, clock.Wait)

	result, err := sender.TestSend(context.Background(), "42", "first")
	require.Equal(t, NotSent, result.Outcome)
	require.ErrorIs(t, err, credential.ErrNotConfigured)
	result, err = sender.Send(context.Background(), "42", "second")
	require.NoError(t, err)
	require.Equal(t, Accepted, result.Outcome)
	require.Equal(t, 2, calls)
	require.Empty(t, clock.delays)
}

func TestWaitDelayCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitDelay(ctx, time.Hour), context.Canceled)
	require.NoError(t, waitDelay(context.Background(), time.Millisecond))
}

func intPointer(value int) *int { return &value }
