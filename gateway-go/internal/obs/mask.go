// Package obs holds logging, masking and tracing setup shared by the gateway binaries.
package obs

import (
	"regexp"
	"strings"
)

// panLike matches a standalone run of 13–19 digits (PAN lengths). Longer or shorter runs are left alone.
var panLike = regexp.MustCompile(`\b\d{13,19}\b`)

// MaskPAN keeps the first 6 and last 4 digits of anything that looks like a PAN (PCI DSS 3.4).
func MaskPAN(s string) string {
	return panLike.ReplaceAllStringFunc(s, func(m string) string {
		return m[:6] + strings.Repeat("*", len(m)-10) + m[len(m)-4:]
	})
}
