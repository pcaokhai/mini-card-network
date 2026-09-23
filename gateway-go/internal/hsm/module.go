// Package hsm simulates a hardware security module: keys are wrapped under an LMK and never
// held in the clear (root CLAUDE.md §6 rule 2). PIN translate and MAC are added by MCN-502.
package hsm

// Module wraps/unwraps keys under the LMK and computes their KCV. Clear key material is always
// []byte, never string (never accidentally logged via %s or held in an immutable Go string).
type Module interface {
	WrapUnderLMK(clearKey []byte) ([]byte, error)
	Unwrap(keyUnderLMK []byte) ([]byte, error)
	ComputeKCV(clearKey []byte) (string, error)

	// ComputeMAC returns the 8-byte ISO 9797-1 algorithm 3 (Retail MAC / X9.19) over
	// packedMessageExcludingMACField, under zak (MCN-502-AC1/AC2).
	ComputeMAC(packedMessageExcludingMACField []byte, zak []byte) ([]byte, error)

	// TranslatePIN decrypts pinBlockUnderTPK under tpk and re-encrypts it under zpk. The clear
	// PIN block exists only inside this call's body and is never returned (MCN-502-AC1).
	TranslatePIN(pinBlockUnderTPK []byte, tpk, zpk []byte) ([]byte, error)
}
