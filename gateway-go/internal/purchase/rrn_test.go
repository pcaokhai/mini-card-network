package purchase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildRRN_matchesSpecFormat__MCN_303_AC2(t *testing.T) {
	// 2026-09-22 14:00 UTC is day-of-year 265, year digit 6, hour 14
	now := time.Date(2026, 9, 22, 14, 0, 0, 0, time.UTC)
	rrn := BuildRRN(now, "000123")
	require.Len(t, rrn, 12)
	require.Equal(t, "6"+"265"+"14"+"000123", rrn)
}
