// Package chaos toggles named failure scenarios against the real Toxiproxy instance and drives
// synthetic load runs that verify the ledger invariant holds under those failures (docs/06
// MCN-404).
package chaos

// ScenarioID mirrors contracts/openapi.yaml's ChaosScenarioId enum exactly.
type ScenarioID string

// The six scenarios (docs/06-user-stories.md MCN-404-AC1).
const (
	ScenarioSlowNetwork      ScenarioID = "SLOW_NETWORK"
	ScenarioConnectionCut    ScenarioID = "CONNECTION_CUT"
	ScenarioDropResponse     ScenarioID = "DROP_RESPONSE"
	ScenarioDuplicateRequest ScenarioID = "DUPLICATE_REQUEST"
	ScenarioIssuerDown       ScenarioID = "ISSUER_DOWN"
	ScenarioLateResponse     ScenarioID = "LATE_RESPONSE"
)

// AllScenarios is the fixed, ordered set of six scenario ids (matches web-next's
// CHAOS_SCENARIO_IDS ordering in chaos-client.ts so both sides render the same six cards).
var AllScenarios = []ScenarioID{
	ScenarioSlowNetwork,
	ScenarioConnectionCut,
	ScenarioDropResponse,
	ScenarioDuplicateRequest,
	ScenarioIssuerDown,
	ScenarioLateResponse,
}

// Scenario mirrors contracts/openapi.yaml's ChaosScenario schema.
type Scenario struct {
	ID            ScenarioID `json:"id"`
	Enabled       bool       `json:"enabled"`
	EasyText      string     `json:"easyText"`
	TechnicalText string     `json:"technicalText"`
}

// scenarioText is the fixed easy/technical copy per scenario id (docs/plans/MCN-404.md's Ruling).
var scenarioText = map[ScenarioID][2]string{
	ScenarioSlowNetwork:      {"The network gets slow.", "3000ms latency toxic on the issuer link."},
	ScenarioConnectionCut:    {"The connection drops mid-transaction.", "reset_peer toxic on the issuer link."},
	ScenarioDropResponse:     {"The issuer's reply never arrives.", "Requires a fake-issuer response-dropping mode (MCN-407, not yet built)."},
	ScenarioDuplicateRequest: {"The same payment is sent twice.", "The gateway sends the same 0200 twice with the same STAN."},
	ScenarioIssuerDown:       {"The issuer is completely unreachable.", "reset_peer toxic on the whole issuer link."},
	ScenarioLateResponse:     {"The reply arrives after we've given up.", "latency toxic exceeding the 30s request timeout."},
}
