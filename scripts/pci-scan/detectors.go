package main

import "regexp"

// Finding is a PCI-scan hit: never carries the raw matched value, only a masked preview.
type Finding struct {
	Detector string
	File     string
	Line     int
	Masked   string
}

var panRunRe = regexp.MustCompile(`\b\d{13,19}\b`)

// ScanPAN flags digit runs that are both length-plausible and Luhn-valid (Ruling 1).
func ScanPAN(text string) []Finding {
	var findings []Finding
	for _, m := range panRunRe.FindAllString(text, -1) {
		if !luhnValid(m) {
			continue
		}
		findings = append(findings, Finding{Detector: "pan", Masked: maskPAN(m)})
	}
	return findings
}

func luhnValid(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

func maskPAN(pan string) string {
	return pan[:6] + "******" + pan[len(pan)-4:]
}

var pinBlockRe = regexp.MustCompile(`\b0[4-9A-Ca-c][0-9A-Fa-f]{14}\b`)

// ScanPinBlock flags ISO 9564-1 format-0 PIN block shape only (Ruling 2: no checksum exists).
func ScanPinBlock(text string) []Finding {
	var findings []Finding
	for range pinBlockRe.FindAllString(text, -1) {
		findings = append(findings, Finding{Detector: "pinblock", Masked: "[redacted pin block]"})
	}
	return findings
}

var track2Re = regexp.MustCompile(`;(\d{13,19})=\d+\?`)

// ScanTrack2 requires the ISO 7813 structural markers plus an embedded Luhn-valid PAN (Ruling 2).
func ScanTrack2(text string) []Finding {
	var findings []Finding
	for _, m := range track2Re.FindAllStringSubmatch(text, -1) {
		pan := m[1]
		if !luhnValid(pan) {
			continue
		}
		findings = append(findings, Finding{Detector: "track2", Masked: maskPAN(pan)})
	}
	return findings
}
