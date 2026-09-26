package bizdate

import "time"

// ClockCalendar derives the business date from the clock and the configured cutover, until
// MCN-702 persists real cutovers (ADR-007 §1): the local date in the cutover zone, plus one day
// once the local time is at or past the cutover time.
type ClockCalendar struct {
	cutover time.Duration // since local midnight
	loc     *time.Location
}

// NewClockCalendar builds a ClockCalendar rolling at cutover (time since local midnight) in loc.
func NewClockCalendar(cutover time.Duration, loc *time.Location) ClockCalendar {
	return ClockCalendar{cutover: cutover, loc: loc}
}

// Current is the business date open at now.
func (c ClockCalendar) Current(now time.Time) time.Time {
	local := now.In(c.loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, c.loc)
	d := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if local.Sub(midnight) >= c.cutover {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// Previous is the business date before d.
func (c ClockCalendar) Previous(d time.Time) time.Time { return d.AddDate(0, 0, -1) }

// OpenedAt is the cutover instant on the local day before d.
func (c ClockCalendar) OpenedAt(d time.Time) time.Time {
	prev := d.AddDate(0, 0, -1)
	return time.Date(prev.Year(), prev.Month(), prev.Day(), 0, 0, 0, 0, c.loc).Add(c.cutover)
}
