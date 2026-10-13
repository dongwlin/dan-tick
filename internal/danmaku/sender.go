package danmaku

import (
	"context"
	"sync"
	"time"
)

// accountInterval is a conservative local policy, not a universal Bilibili
// minimum or a guarantee against server-side rate limits. See docs/B 站 API 对接参考.md.
const accountInterval = 15 * time.Second

// sendingClient is the real HTTP side-effect boundary, replaced in tests.
type sendingClient interface {
	Send(ctx context.Context, roomID, message string) (Result, error)
}

// Sender is the single-account sending entry point. Share one instance across
// manual runs, scheduled runs and test sends; constructing separate instances
// would defeat the account-wide limit. Send and TestSend share the same FIFO
// admission order, with no durable queue table.
type Sender struct {
	client   sendingClient
	interval time.Duration
	nextDue  time.Time // Only accessed while holding an admitted slot.
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
	mu       sync.Mutex
	active   bool
	queue    []*waiter
}

type waiter struct {
	ready   chan struct{}
	granted bool // Under Sender.mu.
}

func NewSender(client *Client) *Sender {
	return newSender(client, accountInterval, time.Now, waitDelay)
}

func newSender(client sendingClient, interval time.Duration, now func() time.Time, wait func(context.Context, time.Duration) error) *Sender {
	s := &Sender{
		client:   client,
		interval: interval,
		now:      now,
		wait:     wait,
	}
	return s
}

// Send waits for the shared account slot, then makes one HTTP send. Every
// request, including one returning an uncertain outcome, counts against the
// interval; requests known not to have been sent do not consume a slot.
func (s *Sender) Send(ctx context.Context, roomID, message string) (Result, error) {
	return s.SendWithAdmission(ctx, roomID, message, nil)
}

// SendWithAdmission lets a task executor commit its queued -> running state
// after FIFO admission but before the HTTP call. A failed callback prevents
// sending and does not consume a rate-limit slot.
func (s *Sender) SendWithAdmission(ctx context.Context, roomID, message string, admitted func(context.Context) error) (Result, error) {
	release, err := s.acquire(ctx)
	if err != nil {
		return Result{Outcome: NotSent}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return Result{Outcome: NotSent}, err
	}
	if delay := s.nextDue.Sub(s.now()); delay > 0 {
		if err := s.wait(ctx, delay); err != nil {
			return Result{Outcome: NotSent}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{Outcome: NotSent}, err
	}
	if admitted != nil {
		if err := admitted(ctx); err != nil {
			return Result{Outcome: NotSent}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{Outcome: NotSent}, err
	}

	// Hold the permit throughout the client call, including credential reads.
	// Start the next gap after the attempt returns so preflight or HTTP latency
	// cannot shorten the interval between actual requests.
	result, err := s.client.Send(ctx, roomID, message)
	if result.Outcome != NotSent {
		s.nextDue = s.now().Add(s.interval)
	}
	return result, err
}

// acquire maintains FIFO order for send requests that reach this Sender,
// including test sends. A canceled waiter is removed without stealing the
// next turn; a cancellation racing with admission hands the slot forward.
func (s *Sender) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w := &waiter{ready: make(chan struct{})}
	s.mu.Lock()
	if !s.active {
		s.active = true
		w.granted = true
		close(w.ready)
	} else {
		s.queue = append(s.queue, w)
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.mu.Lock()
		if w.granted {
			s.releaseLocked()
		} else {
			for i, item := range s.queue {
				if item == w {
					copy(s.queue[i:], s.queue[i+1:])
					s.queue[len(s.queue)-1] = nil
					s.queue = s.queue[:len(s.queue)-1]
					break
				}
			}
		}
		s.mu.Unlock()
		return nil, ctx.Err()
	case <-w.ready:
		if err := ctx.Err(); err != nil {
			s.release()
			return nil, err
		}
		return s.release, nil
	}
}

func (s *Sender) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked()
}

func (s *Sender) releaseLocked() {
	if len(s.queue) == 0 {
		s.active = false
		return
	}
	next := s.queue[0]
	s.queue[0] = nil
	s.queue = s.queue[1:]
	next.granted = true
	close(next.ready)
}

// TestSend performs one real send through the very same account limiter. It
// only sends: execution and attempt persistence belongs to the executor, so
// user-visible test sends must go through executor.Service.TestSend.
func (s *Sender) TestSend(ctx context.Context, roomID, message string) (Result, error) {
	return s.Send(ctx, roomID, message)
}

func waitDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
