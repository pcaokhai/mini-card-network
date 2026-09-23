package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanPAN_flagsLuhnValidRunsOnly__MCN_506_AC1(t *testing.T) {
	findings := ScanPAN("card 4111111111111111 charged")
	require.Len(t, findings, 1)
	require.Equal(t, "411111******1111", findings[0].Masked)

	require.Empty(t, ScanPAN("rrn 626514000123 stan 000123"))

	require.Empty(t, ScanPAN("ref 4111111111111112 not a card"))
}

func TestScanPinBlock_flagsFormatZeroShapeOnly__MCN_506_AC1(t *testing.T) {
	findings := ScanPinBlock("block 0412FFFFFFFFFFFF captured")
	require.Len(t, findings, 1)
	require.Equal(t, "pinblock", findings[0].Detector)

	require.Empty(t, ScanPinBlock("hex value AABBCCDDEEFF0011 not a pin block"))
}

func TestScanTrack2_requiresStructuralMarkersAndLuhnPAN__MCN_506_AC1(t *testing.T) {
	findings := ScanTrack2(";4111111111111111=28112010000012345?")
	require.Len(t, findings, 1)

	require.Empty(t, ScanTrack2("key=4111111111111111 not track data"))
}
