package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dongwlin/dan-tick/internal/autostart"
	"github.com/stretchr/testify/require"
)

type fakeAutostartManager struct {
	status autostart.Status
	err    error
}

func (m *fakeAutostartManager) Status(context.Context) (autostart.Status, error) {
	return m.status, m.err
}

func (m *fakeAutostartManager) SetEnabled(_ context.Context, enabled bool) (autostart.Status, error) {
	if m.err == nil {
		m.status.Enabled, m.status.NeedsRepair = enabled, false
	}
	return m.status, m.err
}

func TestAutostartBindingReturnsFactsAndNotifiesAfterChange(t *testing.T) {
	m := &fakeAutostartManager{status: autostart.Status{Supported: true, NeedsRepair: true}}
	var events []string
	s := &AutostartService{manager: m, changed: func(event string) { events = append(events, event) }}
	status, err := s.GetAutostartStatus(context.Background())
	require.NoError(t, err)
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	require.JSONEq(t, `{"enabled":false,"supported":true,"needsRepair":true}`, string(encoded))
	status, err = s.SetAutostartEnabled(context.Background(), true)
	require.NoError(t, err)
	require.Equal(t, AutostartStatus{Supported: true, Enabled: true}, status)
	status, err = s.SetAutostartEnabled(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, AutostartStatus{Supported: true}, status)
	require.Equal(t, []string{EventApplicationChanged, EventApplicationChanged}, events)
}

func TestAutostartBindingProtectsPlatformErrorsAndCancellation(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{fmt.Errorf("private path: %w", autostart.ErrUnavailable), "autostart_unavailable"},
		{autostart.ErrUnsupported, "autostart_unsupported"},
		{context.Canceled, "cancelled"},
	} {
		t.Run(test.code, func(t *testing.T) {
			events := 0
			s := &AutostartService{manager: &fakeAutostartManager{err: test.err}, changed: func(string) { events++ }}
			_, err := s.GetAutostartStatus(context.Background())
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private path")
			var reply safeError
			require.NoError(t, json.Unmarshal(MarshalError(err), &reply))
			require.Equal(t, test.code, reply.Code)
			_, err = s.SetAutostartEnabled(context.Background(), true)
			require.Error(t, err)
			require.Zero(t, events)
		})
	}
}
