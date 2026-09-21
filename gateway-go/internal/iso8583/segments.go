package iso8583

import "strconv"

// Segment is one consumed slice of the wire message, in read order, for UI highlighting (MCN-104).
type Segment struct {
	Key  string `json:"key"`  // "mti", "primaryBitmap", "secondaryBitmap", or the DE number as a string
	Text string `json:"text"` // the exact on-wire text for this slice, including any length prefix
}

// UnpackSegmented behaves exactly like Unpack but additionally returns the ordered list of
// raw slices consumed, keyed by what each slice is — used by the Lab API to highlight bytes.
func UnpackSegmented(packed string) (string, map[int]string, []Segment, error) {
	t := &taker{packed: packed}
	var segments []Segment

	mti, err := t.take(4, "MTI")
	if err != nil {
		return "", nil, nil, err
	}
	if !digitsRe.MatchString(mti) {
		return "", nil, nil, &CodecError{Code: "INVALID_MTI", Detail: mti}
	}
	segments = append(segments, Segment{Key: "mti", Text: mti})

	numbers, bitmapSegments, err := unpackBitmapsSegmented(t)
	if err != nil {
		return "", nil, nil, err
	}
	segments = append(segments, bitmapSegments...)

	fields := make(map[int]string, len(numbers))
	for _, n := range numbers {
		if n == 1 {
			continue
		}
		start := t.pos
		value, err := unpackField(t, n)
		if err != nil {
			return "", nil, nil, err
		}
		fields[n] = value
		segments = append(segments, Segment{Key: strconv.Itoa(n), Text: t.packed[start:t.pos]})
	}
	if t.pos != len(t.packed) {
		return "", nil, nil, &CodecError{Code: "TRAILING_DATA", Detail: itoa(len(t.packed)-t.pos) + " chars after last field"}
	}
	return mti, fields, segments, nil
}

func unpackBitmapsSegmented(t *taker) ([]int, []Segment, error) {
	start := t.pos
	numbers, err := unpackBitmaps(t)
	if err != nil {
		return nil, nil, err
	}
	var segments []Segment
	segments = append(segments, Segment{Key: "primaryBitmap", Text: t.packed[start : start+16]})
	if len(numbers) > 0 && numbers[0] == 1 {
		segments = append(segments, Segment{Key: "secondaryBitmap", Text: t.packed[start+16 : start+32]})
	}
	return numbers, segments, nil
}
