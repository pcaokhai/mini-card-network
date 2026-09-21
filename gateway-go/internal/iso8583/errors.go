package iso8583

import "errors"

// CodecError is a typed pack/unpack failure carrying the offending DE number (0 = MTI/bitmap level).
type CodecError struct {
	Code   string // INVALID_MTI, INVALID_BITMAP, UNKNOWN_FIELD, INVALID_LENGTH, INVALID_VALUE, TRUNCATED, TRAILING_DATA
	DE     int
	Detail string
}

func (e *CodecError) Error() string {
	if e.DE > 0 {
		return e.Code + ": DE " + itoa(e.DE) + ": " + e.Detail
	}
	return e.Code + ": " + e.Detail
}

// AsCodecError is a thin errors.As wrapper so callers don't need to import this package's error type name twice.
func AsCodecError(err error, target **CodecError) bool {
	return errors.As(err, target)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
