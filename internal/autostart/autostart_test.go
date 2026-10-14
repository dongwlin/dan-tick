package autostart

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeRegistration struct {
	target string
	err    error
	calls  int
}

func (r *fakeRegistration) status(target string) (Status, error) {
	r.calls++
	if r.err != nil {
		return Status{}, r.err
	}
	return Status{Enabled: r.target == target, NeedsRepair: r.target != "" && r.target != target}, nil
}

func (r *fakeRegistration) enable(target string) error {
	r.calls++
	if r.err == nil {
		r.target = target
	}
	return r.err
}

func (r *fakeRegistration) disable() error {
	r.calls++
	if r.err == nil {
		r.target = ""
	}
	return r.err
}

func testManager(r registration) *Manager {
	return &Manager{platform: r, executable: func() (string, error) { return "/current/dan-tick", nil }}
}

func TestManagerReturnsPlatformFactsAndCleansPreviousPath(t *testing.T) {
	ctx := context.Background()
	r := &fakeRegistration{target: "/previous/dan-tick"}
	m := testManager(r)
	status, err := m.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, Status{Supported: true, NeedsRepair: true}, status)
	status, err = m.SetEnabled(ctx, false)
	require.NoError(t, err)
	require.Equal(t, Status{Supported: true}, status)
	status, err = m.SetEnabled(ctx, true)
	require.NoError(t, err)
	require.Equal(t, Status{Supported: true, Enabled: true}, status)
	// An external change must be reflected immediately, without a cached flag.
	r.target = ""
	status, err = m.Status(ctx)
	require.NoError(t, err)
	require.False(t, status.Enabled)
}

func TestManagerCancelledCallsHaveNoPlatformSideEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &fakeRegistration{}
	m := testManager(r)
	_, err := m.Status(ctx)
	require.ErrorIs(t, err, context.Canceled)
	_, err = m.SetEnabled(ctx, true)
	require.ErrorIs(t, err, context.Canceled)
	_, err = m.SetEnabled(ctx, false)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, r.calls)
}

func TestManagerCancelledExecutableLookupDoesNotCreateEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &fakeRegistration{}
	m := testManager(r)
	m.executable = func() (string, error) {
		cancel()
		return "/current/dan-tick", nil
	}
	_, err := m.SetEnabled(ctx, true)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, r.calls)
}

func TestManagerFailuresRemainClassifiable(t *testing.T) {
	underlying := errors.New("private platform error")
	m := testManager(&fakeRegistration{err: underlying})
	_, err := m.Status(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	require.ErrorIs(t, err, underlying)
	_, err = m.SetEnabled(context.Background(), true)
	require.ErrorIs(t, err, ErrUnavailable)
	_, err = m.SetEnabled(context.Background(), false)
	require.ErrorIs(t, err, ErrUnavailable)
	unsupported := testManager(nil)
	status, err := unsupported.Status(context.Background())
	require.NoError(t, err)
	require.False(t, status.Supported)
	_, err = unsupported.SetEnabled(context.Background(), true)
	require.ErrorIs(t, err, ErrUnsupported)
}

func TestManagerSerializesQueriesAndChanges(t *testing.T) {
	m := testManager(&fakeRegistration{})
	var wg sync.WaitGroup
	errors := make(chan error, 60)
	for i := range 30 {
		wg.Go(func() {
			_, err := m.SetEnabled(context.Background(), i%2 == 0)
			errors <- err
			_, err = m.Status(context.Background())
			errors <- err
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	status, err := m.SetEnabled(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, Status{Supported: true}, status)
}
