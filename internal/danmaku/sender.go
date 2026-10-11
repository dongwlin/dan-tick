package danmaku

import (
	"context"
	"time"
)

// accountInterval is a conservative P0 default, not a verified Bilibili
// minimum. Revisit it after the platform's real sending rules are checked.
const accountInterval = 15 * time.Second

// sendingClient is the real HTTP side-effect boundary, replaced in tests.
type sendingClient interface {
	Send(ctx context.Context, roomID, message string) (Result, error)
}

// Sender is the single-account sending entry point. Share one instance across
// manual runs, scheduled runs and test sends; constructing separate instances
// would defeat the account-wide limit. D3 will add execution-level FIFO queues.
type Sender struct {
	client   sendingClient
	permit   chan struct{}
	interval time.Duration
	nextDue  time.Time // Only accessed while holding permit.
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

func NewSender(client *Client) *Sender {
	return newSender(client, accountInterval, time.Now, waitDelay)
}

func newSender(client sendingClient, interval time.Duration, now func() time.Time, wait func(context.Context, time.Duration) error) *Sender {
	s := &Sender{
		client:   client,
		permit:   make(chan struct{}, 1),
		interval: interval,
		now:      now,
		wait:     wait,
	}
	s.permit <- struct{}{}
	return s
}

// Send waits for the shared account slot, then makes one HTTP send. Every
// request, including one returning an uncertain outcome, counts against the
// interval; requests known not to have been sent do not consume a slot.
func (s *Sender) Send(ctx context.Context, roomID, message string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{Outcome: NotSent}, err
	}
	select {
	case <-ctx.Done():
		return Result{Outcome: NotSent}, ctx.Err()
	case <-s.permit:
	}
	defer func() { s.permit <- struct{}{} }()

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

	// Hold the permit throughout the HTTP call: later requests must not
	// overlap, and the next gap is measured from the actual send attempt.
	previousDue := s.nextDue
	s.nextDue = s.now().Add(s.interval)
	result, err := s.client.Send(ctx, roomID, message)
	if result.Outcome == NotSent {
		s.nextDue = previousDue
	}
	return result, err
}

// TestSend performs one real send through the very same account limiter. It
// does not create an execution instance or a send-attempt history row.
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
