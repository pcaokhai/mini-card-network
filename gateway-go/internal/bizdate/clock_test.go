package bizdate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var hcm = mustLocation("Asia/Ho_Chi_Minh")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

const defaultCutover = 23*time.Hour + 59*time.Minute + 59*time.Second

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestClockCalendar_rollsAtTheCutoverInstant__OVW_G7(t *testing.T) {
	cal := NewClockCalendar(defaultCutover, hcm)
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"one second before cutover", time.Date(2026, 9, 25, 23, 59, 58, 999_000_000, hcm), day(2026, 9, 25)},
		{"at cutover", time.Date(2026, 9, 25, 23, 59, 59, 0, hcm), day(2026, 9, 26)},
		{"after local midnight", time.Date(2026, 9, 26, 0, 0, 1, 0, hcm), day(2026, 9, 26)},
		{"UTC is still the previous calendar day", time.Date(2026, 9, 25, 17, 30, 0, 0, time.UTC), day(2026, 9, 26)},
		{"UTC morning, local afternoon", time.Date(2026, 9, 25, 7, 0, 0, 0, time.UTC), day(2026, 9, 25)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, cal.Current(tc.now))
		})
	}
}

func TestClockCalendar_previousAndOpening__OVW_G7(t *testing.T) {
	cal := NewClockCalendar(defaultCutover, hcm)

	require.Equal(t, day(2026, 9, 25), cal.Previous(day(2026, 9, 26)))
	require.Equal(t, day(2026, 2, 28), cal.Previous(day(2026, 3, 1)))
	opened := cal.OpenedAt(day(2026, 9, 26))
	require.True(t, opened.Equal(time.Date(2026, 9, 25, 16, 59, 59, 0, time.UTC)), opened)
	require.Equal(t, day(2026, 9, 26), cal.Current(opened), "the day opens at the cutover that starts it")
}
