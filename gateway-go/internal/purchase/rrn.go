package purchase

import (
	"fmt"
	"time"
)

// BuildRRN implements docs/03 §5: last digit of year + 3-digit day-of-year + 2-digit UTC hour + STAN.
func BuildRRN(now time.Time, stan string) string {
	yearDigit := now.Year() % 10
	dayOfYear := now.YearDay()
	return fmt.Sprintf("%d%03d%02d%s", yearDigit, dayOfYear, now.Hour(), stan)
}
