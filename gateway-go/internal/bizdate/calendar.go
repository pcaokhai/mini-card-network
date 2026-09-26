// Package bizdate is the acquirer's business date (ADR-007): the day a transaction, DE 15, the
// totals and every "today" figure belong to. It rolls only at cutover, never at a calendar
// midnight.
package bizdate

import "time"

// Calendar is the BusinessCalendar port (ADR-007 §1). Dates are midnight UTC values that carry
// only the year, month and day.
type Calendar interface {
	// Current is the business date open at now.
	Current(now time.Time) time.Time
	// Previous is the business date before d.
	Previous(d time.Time) time.Time
	// OpenedAt is the instant d opened: the cutover that closed the previous business date.
	OpenedAt(d time.Time) time.Time
}

// MMDD formats d as DE 15 (docs/03 §3: n 4 MMDD).
func MMDD(d time.Time) string { return d.Format("0102") }

// Format formats d as the API's business date (docs/04 §2: YYYY-MM-DD).
func Format(d time.Time) string { return d.Format(time.DateOnly) }
