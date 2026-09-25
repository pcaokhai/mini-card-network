package saf

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

// goldenOriginal is the purchase contracts/iso8583/vectors/0420-reversal-timeout.json reverses.
func goldenOriginal() store.TranLogRow {
	sentAt := time.Date(2026, 9, 21, 7, 32, 44, 0, time.UTC)
	return store.TranLogRow{
		RRN: "626514000124", Amount: 600000, Currency: "704", TerminalID: "00000042", MerchantID: "GOCPHO000000001",
		NetworkSTAN: "000124", ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sentAt, CardToken: testCardToken,
	}
}

func TestReversalAdvice_matchesTheGoldenVectorFieldSet__MCN_401(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/iso8583/vectors/0420-reversal-timeout.json")
	require.NoError(t, err)
	var vector struct {
		Fields map[string]string `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(raw, &vector))

	adv, err := reversalAdvice(goldenOriginal(), "68")
	require.NoError(t, err)

	// DE 2 (PAN), 7 and 11 (this advice's own STAN and time) and 128 (MAC) are added per send.
	perSend := map[string]bool{"2": true, "7": true, "11": true, "128": true}
	for de, want := range vector.Fields {
		if perSend[de] {
			continue
		}
		n, err := strconv.Atoi(de)
		require.NoError(t, err)
		require.Equal(t, want, adv.Fields[n], "DE %s", de)
	}
	require.Len(t, adv.Fields, len(vector.Fields)-len(perSend))
	require.Equal(t, testCardToken, adv.CardToken)
}

func TestReversalAdvice_refusesATransactionThatWasNeverSent__MCN_401(t *testing.T) {
	neverSent := goldenOriginal()
	neverSent.SentAt = nil
	_, err := reversalAdvice(neverSent, "17")
	require.Error(t, err)

	noStan := goldenOriginal()
	noStan.NetworkSTAN = ""
	_, err = reversalAdvice(noStan, "17")
	require.Error(t, err)
}

func TestReversalAdvice_de90NamesTheOriginalMTI__POS_G4(t *testing.T) {
	preAuth := goldenOriginal()
	preAuth.MTI = "0100"

	adv, err := reversalAdvice(preAuth, "68")

	require.NoError(t, err)
	require.Equal(t, "0100", adv.Fields[90][:4])

	legacy, err := reversalAdvice(goldenOriginal(), "68")
	require.NoError(t, err)
	require.Equal(t, "0200", legacy.Fields[90][:4], "rows logged before tran_log.mti was written are purchases")
}
