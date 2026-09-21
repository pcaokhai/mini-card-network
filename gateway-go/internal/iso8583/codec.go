package iso8583

import (
	"regexp"
	"strconv"
	"strings"
)

// FieldSpec describes one data element's wire encoding (contracts/iso8583/packager-spec.yaml).
type FieldSpec struct {
	Number int
	Type   string // n | an | ans | b
	Length int    // fixed length, or max length when Prefix is LL/LLL
	Prefix string // "", "LL", "LLL"
	Name   string
}

var hexRe = regexp.MustCompile(`^[0-9A-F]*$`)
var digitsRe = regexp.MustCompile(`^[0-9]*$`)

func dataLen(f FieldSpec, value string) int {
	if f.Type == "b" {
		return len(value) / 2
	}
	return len(value)
}

func checkValue(f FieldSpec, value string) error {
	if f.Type == "b" && (len(value)%2 != 0 || !hexRe.MatchString(value)) {
		return &CodecError{Code: "INVALID_VALUE", DE: f.Number, Detail: "must be uppercase hex"}
	}
	if f.Type == "n" && !digitsRe.MatchString(value) {
		return &CodecError{Code: "INVALID_VALUE", DE: f.Number, Detail: "must be numeric"}
	}
	size := dataLen(f, value)
	if f.Prefix == "" && size != f.Length {
		return &CodecError{Code: "INVALID_LENGTH", DE: f.Number, Detail: "fixed length " + itoa(f.Length) + ", got " + itoa(size)}
	}
	if f.Prefix != "" && size > f.Length {
		return &CodecError{Code: "INVALID_LENGTH", DE: f.Number, Detail: "max " + itoa(f.Length) + ", got " + itoa(size)}
	}
	return nil
}

// bitPositions returns which of bits 1..64 (offset by `offset`) are set, MSB first.
func bitPositions(value uint64, offset int) []int {
	var numbers []int
	for i := 0; i < 64; i++ {
		// i is always in [0, 63], so 63-i is always in [0, 63]: safe for the uint shift count.
		if value&(1<<uint(63-i)) != 0 { //nolint:gosec // bounded loop counter, never negative or >63
			numbers = append(numbers, offset+i+1)
		}
	}
	return numbers
}

func bitmapHex(numbers map[int]bool, page int) string {
	var value uint64
	for i := 0; i < 64; i++ {
		if numbers[page*64+i+1] {
			value |= 1 << uint(63-i) //nolint:gosec // bounded loop counter, never negative or >63
		}
	}
	return strings.ToUpper(pad16(strconv.FormatUint(value, 16)))
}

func pad16(s string) string {
	for len(s) < 16 {
		s = "0" + s
	}
	return s
}

// Pack builds a wire message for mti with the given DE number -> value map. DE 1 (secondary
// bitmap) is derived automatically and must not be set explicitly.
func Pack(mti string, fields map[int]string) (string, error) {
	if len(mti) != 4 || !digitsRe.MatchString(mti) {
		return "", &CodecError{Code: "INVALID_MTI", Detail: mti}
	}
	if _, has1 := fields[1]; has1 {
		return "", &CodecError{Code: "UNKNOWN_FIELD", Detail: "DE 1 is derived, do not set it"}
	}

	present, secondary := presenceOf(fields)
	numbers := make([]int, 0, len(fields))
	for n := range fields {
		numbers = append(numbers, n)
	}
	sortInts(numbers)

	var b strings.Builder
	b.WriteString(mti)
	b.WriteString(bitmapHex(present, 0))
	if secondary {
		b.WriteString(bitmapHex(present, 1))
	}
	for _, n := range numbers {
		packed, err := packField(n, fields[n])
		if err != nil {
			return "", err
		}
		b.WriteString(packed)
	}
	return b.String(), nil
}

func presenceOf(fields map[int]string) (present map[int]bool, secondary bool) {
	present = make(map[int]bool, len(fields))
	for n := range fields {
		present[n] = true
		if n > 64 {
			secondary = true
		}
	}
	if secondary {
		present[1] = true
	}
	return present, secondary
}

func packField(n int, v string) (string, error) {
	spec, ok := Fields[n]
	if !ok {
		return "", &CodecError{Code: "UNKNOWN_FIELD", DE: n}
	}
	if err := checkValue(spec, v); err != nil {
		return "", err
	}
	switch spec.Prefix {
	case "LL":
		return pad2(dataLen(spec, v)) + v, nil
	case "LLL":
		return pad3(dataLen(spec, v)) + v, nil
	default:
		return v, nil
	}
}

func pad2(n int) string { return padN(n, 2) }
func pad3(n int) string { return padN(n, 3) }
func padN(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

// taker reads fixed-width slices off a wire message, tracking position and reporting
// truncation; shared by Unpack's bitmap and per-field reads.
type taker struct {
	packed string
	pos    int
}

func (t *taker) take(count int, what string) (string, error) {
	if t.pos+count > len(t.packed) {
		return "", &CodecError{Code: "TRUNCATED", Detail: what + " needs " + itoa(count) + " chars at " + itoa(t.pos)}
	}
	s := t.packed[t.pos : t.pos+count]
	t.pos += count
	return s, nil
}

// Unpack parses a wire message into its MTI and DE number -> value map.
func Unpack(packed string) (string, map[int]string, error) {
	t := &taker{packed: packed}

	mti, err := t.take(4, "MTI")
	if err != nil {
		return "", nil, err
	}
	if !digitsRe.MatchString(mti) {
		return "", nil, &CodecError{Code: "INVALID_MTI", Detail: mti}
	}

	numbers, err := unpackBitmaps(t)
	if err != nil {
		return "", nil, err
	}

	fields := make(map[int]string, len(numbers))
	for _, n := range numbers {
		if n == 1 {
			continue
		}
		value, err := unpackField(t, n)
		if err != nil {
			return "", nil, err
		}
		fields[n] = value
	}
	if t.pos != len(t.packed) {
		return "", nil, &CodecError{Code: "TRAILING_DATA", Detail: itoa(len(t.packed)-t.pos) + " chars after last field"}
	}
	return mti, fields, nil
}

func unpackBitmaps(t *taker) ([]int, error) {
	primary, err := t.take(16, "primary bitmap")
	if err != nil {
		return nil, err
	}
	if !hexRe.MatchString(primary) {
		return nil, &CodecError{Code: "INVALID_BITMAP", Detail: primary}
	}
	bits, _ := strconv.ParseUint(primary, 16, 64)
	numbers := bitPositions(bits, 0)
	if len(numbers) == 0 || numbers[0] != 1 {
		return numbers, nil
	}

	secondary, err := t.take(16, "secondary bitmap")
	if err != nil {
		return nil, err
	}
	if !hexRe.MatchString(secondary) {
		return nil, &CodecError{Code: "INVALID_BITMAP", Detail: secondary}
	}
	sbits, _ := strconv.ParseUint(secondary, 16, 64)
	numbers = append(numbers, bitPositions(sbits, 64)...)
	return numbers, nil
}

func unpackField(t *taker, n int) (string, error) {
	spec, ok := Fields[n]
	if !ok {
		return "", &CodecError{Code: "UNKNOWN_FIELD", DE: n}
	}
	size := spec.Length
	if spec.Prefix != "" {
		digits := 2
		if spec.Prefix == "LLL" {
			digits = 3
		}
		rawLen, err := t.take(digits, "DE "+itoa(n)+" length")
		if err != nil {
			return "", err
		}
		if !digitsRe.MatchString(rawLen) {
			return "", &CodecError{Code: "INVALID_LENGTH", DE: n, Detail: "prefix '" + rawLen + "'"}
		}
		size, _ = strconv.Atoi(rawLen)
		if size > spec.Length {
			return "", &CodecError{Code: "INVALID_LENGTH", DE: n, Detail: "max " + itoa(spec.Length) + ", got " + itoa(size)}
		}
	}
	width := size
	if spec.Type == "b" {
		width = size * 2
	}
	value, err := t.take(width, "DE "+itoa(n))
	if err != nil {
		return "", err
	}
	if err := checkValue(spec, value); err != nil {
		return "", err
	}
	return value, nil
}
