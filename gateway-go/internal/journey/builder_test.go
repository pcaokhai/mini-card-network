package journey

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

func TestBuildJourney_approvedPurchaseHasPosAcquirerIssuerSteps__MCN_304_AC2(t *testing.T) {
	txn := store.TranLogRow{RRN: "x", Type: "PURCHASE", Status: "APPROVED", ResponseCode: "00", Amount: 10000, Currency: "704", CreatedAt: time.Now()}
	history := []store.StateTransition{
		{FromStatus: "CREATED", ToStatus: "SENT", At: txn.CreatedAt},
		{FromStatus: "SENT", ToStatus: "APPROVED", At: txn.CreatedAt.Add(50 * time.Millisecond)},
	}

	j := BuildJourney(txn, history)

	require.Len(t, j.Steps, 2)
	require.Equal(t, "POS", string(j.Steps[0].Actor))
	require.Equal(t, "ISSUER", string(j.Steps[1].Actor))
	require.Contains(t, j.Steps[1].EasyText, "Approved")
	require.Equal(t, "OK", string(j.Steps[1].Kind))
	require.Len(t, j.Money, 1)
	require.Equal(t, int64(-10000), j.Money[0].Delta)
	require.Equal(t, 2, j.Money[0].AtStep)
}

func TestBuildJourney_declinedBlockedCardHasEasyText__MCN_304_AC2(t *testing.T) {
	txn := store.TranLogRow{RRN: "y", Type: "PURCHASE", Status: "DECLINED", ResponseCode: "62", Amount: 5000, Currency: "704", CreatedAt: time.Now()}
	history := []store.StateTransition{{FromStatus: "SENT", ToStatus: "DECLINED", At: txn.CreatedAt}}

	j := BuildJourney(txn, history)

	require.Contains(t, j.Steps[0].EasyText, "blocked")
	require.Equal(t, "BAD", string(j.Steps[0].Kind))
}

func TestBuildJourney_offsetMsIsRelativeToFirstStep__MCN_304_AC2(t *testing.T) {
	base := time.Now()
	txn := store.TranLogRow{RRN: "z", Type: "PURCHASE", Status: "APPROVED", ResponseCode: "00", Amount: 100, Currency: "704", CreatedAt: base}
	history := []store.StateTransition{
		{FromStatus: "CREATED", ToStatus: "SENT", At: base},
		{FromStatus: "SENT", ToStatus: "APPROVED", At: base.Add(120 * time.Millisecond)},
	}

	j := BuildJourney(txn, history)

	require.Equal(t, 0, j.Steps[0].OffsetMs)
	require.Equal(t, 120, j.Steps[1].OffsetMs)
}

func TestEasyTextForRC_coversEveryDocumentedCode(t *testing.T) {
	for _, rc := range []string{"00", "05", "06", "10", "12", "13", "14", "17", "30", "51", "54", "55", "57", "61", "62", "65", "68", "75", "91", "94", "95", "96"} {
		require.NotEmpty(t, EasyTextForRC(rc), "missing easy text for RC %s", rc)
	}
}
