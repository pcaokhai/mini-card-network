// Package hsm simulates a hardware security module: keys are wrapped under an LMK and never
// held in the clear (root CLAUDE.md §6 rule 2). PIN translate and MAC are added by MCN-502.
package hsm

// Module wraps/unwraps keys under the LMK and computes their KCV. Clear key material is always
// []byte, never string (never accidentally logged via %s or held in an immutable Go string).
type Module interface {
	WrapUnderLMK(clearKey []byte) ([]byte, error)
	Unwrap(keyUnderLMK []byte) ([]byte, error)
	ComputeKCV(clearKey []byte) (string, error)
}
