package seed

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixturePath is contracts/fixtures/cards.json relative to this package, so these tests fail if
// the plan drifts from the card states the fixture actually defines.
const fixturePath = "../../../contracts/fixtures/cards.json"

var planNow = time.Date(2026, 9, 21, 7, 30, 0, 0, time.UTC) // 14:30 in Vietnam

func buildTestPlan(t *testing.T, seed int64) []Txn {
	t.Helper()
	return Build(planNow, rand.New(rand.NewSource(seed))) //nolint:gosec // deterministic test data
}

func TestSeedPlan_todayIsAbout12PercentOverYesterdaysWindow__MCN_002(t *testing.T) {
	plan := buildTestPlan(t, 1)
	dayStart := planNow.Truncate(24 * time.Hour)

	var today, yesterday int
	for _, txn := range plan {
		switch {
		case !txn.At.Before(dayStart) && !txn.At.After(planNow):
			today++
		case !txn.At.Before(dayStart.Add(-24*time.Hour)) && !txn.At.After(planNow.Add(-24*time.Hour)):
			yesterday++
		default:
			t.Fatalf("transaction at %s falls outside both windows", txn.At)
		}
	}
	delta := float64(today-yesterday) / float64(yesterday)
	require.InDelta(t, 0.12, delta, 0.02)
}

func TestSeedPlan_everyLastHourBucketHasTransactions__MCN_002(t *testing.T) {
	plan := buildTestPlan(t, 2)

	var counts [bucketCount]int
	for _, txn := range plan {
		age := planNow.Sub(txn.At)
		if age < 0 || age >= time.Hour {
			continue
		}
		counts[23-int(age/(150*time.Second))]++ //nolint:gosec // G602: 0 <= age < 1h, so the index is 0..23
	}
	for i, c := range counts {
		require.Positive(t, c, "bucket %d is empty", i)
	}
}

func TestSeedPlan_outcomeMixMatchesCardStates__MCN_002(t *testing.T) {
	fixture, err := LoadFixture(fixturePath)
	require.NoError(t, err)
	plan := buildTestPlan(t, 3)

	outcomes := map[string]int{}
	for _, txn := range plan {
		card, ok := fixture.Card(txn.CardToken)
		require.True(t, ok, "unknown card %s", txn.CardToken)
		_, ok = fixture.Terminal(txn.TerminalID)
		require.True(t, ok, "unknown terminal %s", txn.TerminalID)
		outcomes[txn.Expect]++

		switch txn.Expect {
		case ExpectApproved:
			require.Equal(t, "ACTIVE", card.Status, "approval planned on %s", txn.CardToken)
			require.NotEqual(t, "tok_expired", txn.CardToken)
			require.NotEqual(t, "tok_low", txn.CardToken)
			if txn.CardToken == limitCardToken {
				require.LessOrEqual(t, txn.Amount, LimitPerTransaction)
			}
		case "51":
			require.Equal(t, "tok_low", txn.CardToken)
			require.Greater(t, txn.Amount, card.Balance)
		case "61":
			require.Equal(t, limitCardToken, txn.CardToken)
			require.Greater(t, txn.Amount, LimitPerTransaction)
		case "62":
			require.Equal(t, "BLOCKED", card.Status)
		case "54":
			require.Equal(t, "tok_expired", txn.CardToken)
		default:
			t.Fatalf("unexpected outcome %q", txn.Expect)
		}
		if txn.Cancel {
			require.Equal(t, ExpectApproved, txn.Expect, "only approvals can be cancelled")
		}
	}
	for _, rc := range []string{"51", "61", "62", "54"} {
		require.Positive(t, outcomes[rc], "no %s decline planned", rc)
	}
	approvalRate := float64(outcomes[ExpectApproved]) / float64(len(plan))
	require.InDelta(t, 0.94, approvalRate, 0.02)
}

func TestSeedPlan_spendStaysWithinEachApprovingCardsBalance__MCN_002(t *testing.T) {
	fixture, err := LoadFixture(fixturePath)
	require.NoError(t, err)

	for seed := int64(1); seed <= 20; seed++ {
		spend := map[string]int64{}
		for _, txn := range buildTestPlan(t, seed) {
			if txn.Expect == ExpectApproved {
				spend[txn.CardToken] += txn.Amount
			}
		}
		for token, total := range spend {
			card, _ := fixture.Card(token)
			require.LessOrEqual(t, total, card.Balance/3, "seed %d: %s spends %d of %d", seed, token, total, card.Balance)
		}
	}
}

func TestSeedPlan_plansAtLeastOneCancellationAndEveryMerchant__MCN_002(t *testing.T) {
	plan := buildTestPlan(t, 4)

	cancels := 0
	terminals := map[string]bool{}
	for _, txn := range plan {
		if txn.Cancel {
			cancels++
		}
		terminals[txn.TerminalID] = true
	}
	require.Positive(t, cancels)
	require.Len(t, terminals, 7)
}

func TestSeedPlan_isDeterministicForASeed__MCN_002(t *testing.T) {
	require.Equal(t, buildTestPlan(t, 7), buildTestPlan(t, 7))
}
