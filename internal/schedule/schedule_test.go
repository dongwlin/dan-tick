package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func at(t *testing.T, rfc3339 string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, rfc3339)
	require.NoError(t, err)
	return value
}

func requireNext(t *testing.T, rule Rule, after, want string) {
	t.Helper()
	next, ok, err := NextAfter(rule, at(t, after))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, at(t, want), next)
	require.Equal(t, time.UTC, next.Location())
	require.Zero(t, next.Nanosecond())
	require.Equal(t, at(t, want).UnixMilli(), next.UnixMilli())
}

func TestOnceFuturePastAndExactBoundary(t *testing.T) {
	rule := Rule{Kind: Once, Timezone: "Asia/Shanghai", LocalDate: "2026-10-15", LocalTime: "22:00:00"}
	requireNext(t, rule, "2026-10-15T13:59:59Z", "2026-10-15T14:00:00Z")

	for _, after := range []string{"2026-10-15T14:00:00Z", "2026-10-16T00:00:00Z"} {
		next, ok, err := NextAfter(rule, at(t, after))
		require.NoError(t, err)
		require.False(t, ok)
		require.True(t, next.IsZero())
	}
}

func TestDailyAcrossMidnightAndExactBoundary(t *testing.T) {
	rule := Rule{Kind: Daily, Timezone: "Asia/Shanghai", LocalTime: "00:15:00"}
	requireNext(t, rule, "2026-10-10T16:14:59Z", "2026-10-10T16:15:00Z")
	requireNext(t, rule, "2026-10-10T16:15:00Z", "2026-10-11T16:15:00Z")
	requireNext(t, rule, "2026-10-10T16:15:01Z", "2026-10-11T16:15:00Z")
}

func TestWeeklyUsesMondayBasedWeekdayMask(t *testing.T) {
	// Monday bit 0, Wednesday bit 2, Friday bit 4; Sunday bit 6.
	rule := Rule{Kind: Weekly, Timezone: "Asia/Shanghai", LocalTime: "22:00:00", Weekdays: 1 | 4 | 16}
	requireNext(t, rule, "2026-10-11T12:00:00Z", "2026-10-12T14:00:00Z")
	requireNext(t, rule, "2026-10-12T14:00:00Z", "2026-10-14T14:00:00Z")
	requireNext(t, rule, "2026-10-16T15:00:00Z", "2026-10-19T14:00:00Z")

	sunday := Rule{Kind: Weekly, Timezone: "UTC", LocalTime: "23:45:00", Weekdays: 64}
	requireNext(t, sunday, "2026-12-27T23:45:00Z", "2027-01-03T23:45:00Z")
}

func TestTaskTimezoneIndependentOfHostTimezone(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("different host zone", -7*3600)
	t.Cleanup(func() { time.Local = oldLocal })

	rule := Rule{Kind: Daily, Timezone: "Asia/Shanghai", LocalTime: "09:00:00"}
	requireNext(t, rule, "2026-10-11T00:00:00Z", "2026-10-11T01:00:00Z")
}

func TestNonexistentDSTTimeSkipped(t *testing.T) {
	// New York jumps from 01:59:59 to 03:00 on 2026-03-08.
	daily := Rule{Kind: Daily, Timezone: "America/New_York", LocalTime: "02:30:00"}
	requireNext(t, daily, "2026-03-07T07:29:59Z", "2026-03-07T07:30:00Z")
	requireNext(t, daily, "2026-03-07T07:30:00Z", "2026-03-09T06:30:00Z")

	sunday := Rule{Kind: Weekly, Timezone: "America/New_York", LocalTime: "02:30:00", Weekdays: 64}
	requireNext(t, sunday, "2026-03-01T07:30:00Z", "2026-03-15T06:30:00Z")

	once := Rule{Kind: Once, Timezone: "America/New_York", LocalDate: "2026-03-08", LocalTime: "02:30:00"}
	next, ok, err := NextAfter(once, at(t, "2026-03-01T00:00:00Z"))
	require.NoError(t, err)
	require.False(t, ok, "a nonexistent one-time local time has no plan instant")
	require.True(t, next.IsZero())
}

func TestRepeatedDSTTimeOccursOnlyOnce(t *testing.T) {
	// At fall-back, 01:30 occurs at both 05:30Z (EDT) and 06:30Z (EST).
	daily := Rule{Kind: Daily, Timezone: "America/New_York", LocalTime: "01:30:00"}
	requireNext(t, daily, "2026-11-01T05:29:59Z", "2026-11-01T05:30:00Z")
	requireNext(t, daily, "2026-11-01T05:30:00Z", "2026-11-02T06:30:00Z")
	requireNext(t, daily, "2026-11-01T06:00:00Z", "2026-11-02T06:30:00Z")

	sunday := Rule{Kind: Weekly, Timezone: "America/New_York", LocalTime: "01:30:00", Weekdays: 64}
	requireNext(t, sunday, "2026-11-01T05:30:00Z", "2026-11-08T06:30:00Z")

	once := Rule{Kind: Once, Timezone: "America/New_York", LocalDate: "2026-11-01", LocalTime: "01:30:00"}
	requireNext(t, once, "2026-11-01T05:00:00Z", "2026-11-01T05:30:00Z")
	next, ok, err := NextAfter(once, at(t, "2026-11-01T05:30:00Z"))
	require.NoError(t, err)
	require.False(t, ok, "the second copy of the local hour must not run")
	require.True(t, next.IsZero())
}

func TestUnusualTimezoneTransitions(t *testing.T) {
	// Lord Howe falls back 30 minutes, repeating 01:30-01:59.
	lordHowe := Rule{Kind: Daily, Timezone: "Australia/Lord_Howe", LocalTime: "01:45:00"}
	requireNext(t, lordHowe, "2026-04-04T14:44:59Z", "2026-04-04T14:45:00Z")
	requireNext(t, lordHowe, "2026-04-04T14:45:00Z", "2026-04-05T15:15:00Z")

	// Apia skipped the entire local date 2011-12-30 when changing offset.
	apia := Rule{Kind: Daily, Timezone: "Pacific/Apia", LocalTime: "09:00:00"}
	requireNext(t, apia, "2011-12-29T19:00:00Z", "2011-12-30T19:00:00Z")
	skipped := Rule{Kind: Once, Timezone: "Pacific/Apia", LocalDate: "2011-12-30", LocalTime: "09:00:00"}
	next, ok, err := NextAfter(skipped, at(t, "2011-12-28T00:00:00Z"))
	require.NoError(t, err)
	require.False(t, ok)
	require.True(t, next.IsZero())
}

func TestInvalidRules(t *testing.T) {
	base := Rule{Kind: Daily, Timezone: "Asia/Shanghai", LocalTime: "09:00:00"}
	tests := []struct {
		name string
		rule Rule
		err  string
	}{
		{"no timezone", Rule{Kind: Daily, LocalTime: base.LocalTime}, "timezone"},
		{"host local timezone", Rule{Kind: Daily, Timezone: "Local", LocalTime: base.LocalTime}, "timezone"},
		{"unknown timezone", Rule{Kind: Daily, Timezone: "Mars/Olympus", LocalTime: base.LocalTime}, "timezone"},
		{"bad time", Rule{Kind: Daily, Timezone: base.Timezone, LocalTime: "9:00"}, "local_time"},
		{"fractional seconds", Rule{Kind: Daily, Timezone: base.Timezone, LocalTime: "09:00:00.1"}, "local_time"},
		{"bad date", Rule{Kind: Once, Timezone: base.Timezone, LocalDate: "2026-02-30", LocalTime: base.LocalTime}, "local_date"},
		{"missing once date", Rule{Kind: Once, Timezone: base.Timezone, LocalTime: base.LocalTime}, "local_date"},
		{"date on recurring", Rule{Kind: Daily, Timezone: base.Timezone, LocalDate: "2026-10-11", LocalTime: base.LocalTime}, "local_date"},
		{"once weekdays", Rule{Kind: Once, Timezone: base.Timezone, LocalDate: "2026-10-11", LocalTime: base.LocalTime, Weekdays: 1}, "weekdays"},
		{"daily weekdays", Rule{Kind: Daily, Timezone: base.Timezone, LocalTime: base.LocalTime, Weekdays: 1}, "weekdays"},
		{"weekly no days", Rule{Kind: Weekly, Timezone: base.Timezone, LocalTime: base.LocalTime}, "weekdays"},
		{"weekly out of range", Rule{Kind: Weekly, Timezone: base.Timezone, LocalTime: base.LocalTime, Weekdays: 128}, "weekdays"},
		{"unsupported kind", Rule{Kind: "monthly", Timezone: base.Timezone, LocalTime: base.LocalTime}, "schedule_kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok, err := NextAfter(tt.rule, at(t, "2026-10-11T00:00:00Z"))
			require.ErrorContains(t, err, tt.err)
			require.False(t, ok)
		})
	}
}
