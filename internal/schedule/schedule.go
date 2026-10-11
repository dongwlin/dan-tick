// Package schedule calculates task plan instants independently of the scheduler
// and persistence layer.
package schedule

import (
	"fmt"
	"time"
	_ "time/tzdata" // Include IANA rules in packaged Windows/Linux binaries.
)

// Kind is a task's calendar recurrence, independent of its sending mode.
type Kind string

const (
	Once   Kind = "once"
	Daily  Kind = "daily"
	Weekly Kind = "weekly"
)

// Rule uses the same local date, time, timezone and weekday encoding as tasks
// in SQLite. Weekdays uses Monday bit 0 through Sunday bit 6.
// LocalDate must be YYYY-MM-DD for Once; LocalTime must be HH:MM:SS.
// Weekdays is zero for Once/Daily and nonzero for Weekly.
type Rule struct {
	Kind      Kind
	Timezone  string
	LocalDate string
	LocalTime string
	Weekdays  int
}

// NextAfter returns the first UTC plan instant strictly after the given time.
// A one-time plan in the past or at a nonexistent local time has no next instant.
// Repeated local times use their first occurrence; nonexistent days are skipped
// for recurring rules. The caller owns persistence of the next_due_at cursor:
// recomputing after a system clock rollback must not rewind a saved cursor.
func NextAfter(rule Rule, after time.Time) (time.Time, bool, error) {
	if rule.Timezone == "" || rule.Timezone == "Local" {
		return time.Time{}, false, fmt.Errorf("timezone: explicit IANA location required")
	}
	location, err := time.LoadLocation(rule.Timezone)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("timezone %q: %w", rule.Timezone, err)
	}

	localTime, err := time.Parse(time.TimeOnly, rule.LocalTime)
	if err != nil || localTime.Format(time.TimeOnly) != rule.LocalTime {
		return time.Time{}, false, fmt.Errorf("local_time: expected HH:MM:SS")
	}

	switch rule.Kind {
	case Once:
		if rule.Weekdays != 0 {
			return time.Time{}, false, fmt.Errorf("weekdays: not allowed for once")
		}
		date, err := time.Parse(time.DateOnly, rule.LocalDate)
		if err != nil || date.Format(time.DateOnly) != rule.LocalDate {
			return time.Time{}, false, fmt.Errorf("local_date: expected a valid YYYY-MM-DD")
		}
		candidate, valid := resolveLocal(date, localTime, location)
		if valid && candidate.After(after) {
			return candidate, true, nil
		}
		return time.Time{}, false, nil
	case Daily, Weekly:
		if rule.LocalDate != "" {
			return time.Time{}, false, fmt.Errorf("local_date: only allowed for once")
		}
		if rule.Kind == Daily && rule.Weekdays != 0 {
			return time.Time{}, false, fmt.Errorf("weekdays: only allowed for weekly")
		}
		if rule.Kind == Weekly && (rule.Weekdays < 1 || rule.Weekdays > 127) {
			return time.Time{}, false, fmt.Errorf("weekdays: expected Monday-to-Sunday bitmask (1-127)")
		}
	default:
		return time.Time{}, false, fmt.Errorf("schedule_kind: unsupported %q", rule.Kind)
	}

	y, m, d := after.In(location).Date()
	// Iterate calendar dates in UTC, not 24-hour increments in the task zone:
	// daylight saving transitions can make a local day 23 or 25 hours long.
	startDate := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	// Two weeks covers a skipped weekly occurrence (e.g. a spring DST gap).
	for day := range 15 {
		date := startDate.AddDate(0, 0, day)
		if rule.Kind == Weekly {
			mondayBasedDay := (int(date.Weekday()) + 6) % 7
			if rule.Weekdays&(1<<mondayBasedDay) == 0 {
				continue
			}
		}
		candidate, valid := resolveLocal(date, localTime, location)
		if valid && candidate.After(after) {
			return candidate, true, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("no valid occurrence within two weeks")
}

// resolveLocal maps a wall-clock date and time to its earliest actual instant.
// time.Date with a Location normalizes missing hours and can choose either
// side of an ambiguous hour. Instead, inspect the zone offsets across the
// relevant UTC window, retaining only exact local wall-time round trips.
func resolveLocal(date, clock time.Time, location *time.Location) (time.Time, bool) {
	y, m, d := date.Date()
	h, minute, second := clock.Clock()
	wallUTC := time.Date(y, m, d, h, minute, second, 0, time.UTC)
	from := wallUTC.Add(-48 * time.Hour)
	until := wallUTC.Add(48 * time.Hour)
	var earliest time.Time

	for cursor := from; cursor.Before(until); {
		inZone := cursor.In(location)
		_, offset := inZone.Zone()
		candidate := wallUTC.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(location)
		cy, cm, cd := local.Date()
		ch, cminute, cs := local.Clock()
		if cy == y && cm == m && cd == d &&
			ch == h && cminute == minute && cs == second &&
			(earliest.IsZero() || candidate.Before(earliest)) {
			earliest = candidate
		}

		_, end := inZone.ZoneBounds()
		if end.IsZero() || !end.After(cursor) || !end.Before(until) {
			break
		}
		cursor = end
	}
	return earliest, !earliest.IsZero()
}
