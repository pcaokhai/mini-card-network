package main

import (
	"fmt"
	"strings"
)

// ScenarioResult is one chaos scenario's outcome, sourced from ChaosRun (gateway-go's
// internal/chaos.ChaosRun over the wire) - counts and RCs only, never card data.
type ScenarioResult struct {
	ID                string
	Requested         int
	Approved          int
	Declined          int
	Reversed          int
	LedgerDiscrepancy int64
	Status            string
}

// RenderReport writes the fixed markdown table (docs/plans/MCN-407.md Ruling 2): one row per
// scenario, in the order given.
func RenderReport(results []ScenarioResult, safDeadCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Chaos suite report\n\n")
	fmt.Fprintf(&b, "SAF dead count: %d\n\n", safDeadCount)
	fmt.Fprintf(&b, "| Scenario | Requested | Approved | Declined | Reversed | LedgerDiscrepancy | Status |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %s |\n",
			r.ID, r.Requested, r.Approved, r.Declined, r.Reversed, r.LedgerDiscrepancy, r.Status)
	}
	return b.String()
}

// ReportFailed reports whether the suite should fail the build: any scenario with a non-zero
// ledger discrepancy or a non-PASSED status, or any SAF item left DEAD after drain (MCN-407-AC2).
func ReportFailed(results []ScenarioResult, safDeadCount int) bool {
	if safDeadCount > 0 {
		return true
	}
	for _, r := range results {
		if r.LedgerDiscrepancy != 0 || r.Status != "PASSED" {
			return true
		}
	}
	return false
}
