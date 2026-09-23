package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderReport_failsOnNonZeroDiscrepancyOrDeadSaf__MCN_407_AC2(t *testing.T) {
	results := []ScenarioResult{
		{ID: "SLOW_NETWORK", Requested: 500, Approved: 480, Declined: 20, Status: "PASSED"},
		{ID: "DROP_RESPONSE", Requested: 500, Reversed: 500, LedgerDiscrepancy: 0, Status: "PASSED"},
	}

	report := RenderReport(results, 0)

	require.True(t, strings.Contains(report, "SLOW_NETWORK"))
	require.True(t, strings.Contains(report, "| PASSED |"))
	require.False(t, ReportFailed(results, 0))
}

func TestReportFailed_onDiscrepancy__MCN_407_AC2(t *testing.T) {
	results := []ScenarioResult{{ID: "ISSUER_DOWN", LedgerDiscrepancy: 100, Status: "FAILED"}}
	require.True(t, ReportFailed(results, 0))
}

func TestReportFailed_onDeadSafItems__MCN_407_AC2(t *testing.T) {
	results := []ScenarioResult{{ID: "SLOW_NETWORK", LedgerDiscrepancy: 0, Status: "PASSED"}}
	require.True(t, ReportFailed(results, 3))
}
