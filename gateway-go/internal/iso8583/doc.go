// Package iso8583 packs and unpacks MCN-87A messages (docs/03-iso8583-interface-spec.md).
// It is pure: no I/O, no globals beyond the generated field table. See ADR-003.
package iso8583

//go:generate go run ./gen
