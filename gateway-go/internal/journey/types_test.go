package journey

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func typedRow(tranType, processingCode string) Journey {
	row := sentRow(statusApproved, rcApproved, testAuthCode)
	row.Type, row.ProcessingCode = tranType, processingCode
	if tranType == tranTypeCompletion {
		row.OriginalRRN = "626514000100"
	}
	return BuildJourney(row, approvedHistory(), nil)
}

func TestBuildJourney_mtiPairFollowsTheTransactionType__JRN_G2(t *testing.T) {
	cases := []struct {
		tranType, processingCode, request, response, path string
	}{
		{purchase, "000000", "0200", "0210", "POST /v1/transactions/purchases"},
		{"PREAUTH", "000000", "0100", "0110", "POST /v1/transactions/pre-authorizations"},
		{tranTypeCompletion, "000000", "0220", "0230", "POST /v1/transactions/626514000100/completions"},
		{tranTypeRefund, "200000", "0200", "0210", "POST /v1/transactions/refunds"},
		{tranTypeBalance, "310000", "0200", "0210", "POST /v1/transactions/balance-inquiries"},
	}
	for _, tc := range cases {
		t.Run(tc.tranType, func(t *testing.T) {
			j := typedRow(tc.tranType, tc.processingCode)

			require.Contains(t, j.Steps[0].TechnicalText, tc.path)
			require.Equal(t, tc.request, j.Steps[1].Message.MTI)
			require.Contains(t, j.Steps[1].TechnicalText, tc.request)
			require.Equal(t, tc.processingCode, fieldValue(j.Steps[1].Message, "3"))
			require.Equal(t, tc.response, j.Steps[2].Message.MTI)
			require.Contains(t, j.Steps[2].TechnicalText, tc.response)
		})
	}
}

func TestBuildJourney_moneyFollowsTheTransactionType__JRN_G2(t *testing.T) {
	require.Equal(t, []MoneyRow{{Label: labelRefund, Delta: 600000, AtStep: 3}}, typedRow(tranTypeRefund, "200000").Money, "a refund credits the cardholder")
	require.Empty(t, typedRow(tranTypeBalance, "310000").Money, "a balance inquiry moves no money")
	require.Equal(t, int64(-600000), typedRow("PREAUTH", "000000").Money[0].Delta)
}

func TestBuildJourney_messagesCarryOnlyWhatTheTypeSends__JRN_G2(t *testing.T) {
	completion := typedRow(tranTypeCompletion, "000000")
	require.Equal(t, []string{deMTI, "3", "4", "7", "11", "37", "49"}, fieldDEs(completion.Steps[1].Message), "a 0220 carries no card-present data")
	require.Equal(t, "626514000100", fieldValue(completion.Steps[1].Message, "37"), "DE 37 references the pre-auth")

	balance := typedRow(tranTypeBalance, "310000")
	require.NotContains(t, fieldDEs(balance.Steps[1].Message), "4", "a balance inquiry requests no amount")
	require.NotContains(t, fieldDEs(balance.Steps[2].Message), "4")
}
