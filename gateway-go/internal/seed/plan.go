package seed

import (
	"math/rand"
	"sort"
	"time"
)

// ExpectApproved is Txn.Expect for a purchase the issuer should approve; declines use their RC.
const ExpectApproved = "APPROVED"

// LimitPerTransaction is the per-transaction limit the seed sets on tok_limit through the
// issuer Card Admin API, so any purchase above it is declined RC 61 by the issuer's velocity rule.
const LimitPerTransaction int64 = 500_000

// Txn is one planned purchase: which card at which terminal, how much, the time its gateway
// records are moved to, and the outcome the card's fixture state should produce.
type Txn struct {
	CardToken  string
	TerminalID string
	Amount     int64
	At         time.Time
	Expect     string
	Cancel     bool
}

const (
	bucketCount        = 24
	bucketWidth        = 150 * time.Second
	lastHourCount      = 96
	earlierTodayCount  = 34
	yesterdayCount     = 116 // (96 + 34) / 116 - 1 ≈ +12 %, the canvas's day-over-day line
	cancellationsToday = 2
	amountStep         = 500

	// Per-run approval budgets stay under a third of each card's fixture balance, so three
	// consecutive runs never turn a planned approval into RC 51 (plan ruling 7).
	normalCardBudget = 1_500_000
	limitCardBudget  = 15_000_000
)

// lastHourShape is the Overview canvas's bar heights; the last hour's transactions follow it.
var lastHourShape = [bucketCount]int{12, 15, 14, 18, 22, 20, 19, 25, 28, 26, 31, 34, 30, 29, 33, 38, 35, 32, 30, 27, 29, 31, 28, 26}

type merchantProfile struct {
	terminalID string
	weight     float64
	min, max   int64
}

// Amount ranges per merchant type, in VND.
var merchants = []merchantProfile{
	{"00000042", 0.30, 35_000, 95_000},   // Cà phê Góc Phố
	{"00000043", 0.10, 85_000, 450_000},  // Nhà sách Ánh Dương
	{"00000044", 0.12, 150_000, 650_000}, // Siêu thị Hoa Sen
	{"00000045", 0.08, 200_000, 600_000}, // Trạm xăng Bến Nghé
	{"00000046", 0.15, 45_000, 85_000},   // Quán bún Cô Ba
	{"00000047", 0.15, 30_000, 150_000},  // Tiệm bánh Mây
	{"00000048", 0.10, 60_000, 380_000},  // Nhà thuốc Bình An
}

// Declines per window, keyed by RC. Their shares follow the canvas's decline chart without RC 55,
// which the stack cannot produce yet (risk R-12).
var (
	todayDeclines     = map[string]int{"51": 4, "61": 2, "62": 1, "54": 1}
	yesterdayDeclines = map[string]int{"51": 3, "61": 2, "62": 1, "54": 1}
)

// Build plans one seed run relative to now. It is pure: the same now and seed give the same plan.
func Build(now time.Time, rng *rand.Rand) []Txn {
	p := planner{rng: rng, spent: map[string]int64{}}
	dayStart := now.Truncate(24 * time.Hour)

	todayTimes := append(lastHourTimes(now, rng), earlierTodayTimes(now, dayStart, rng)...)
	yesterdayTimes := uniformTimes(dayStart.Add(-24*time.Hour), now.Add(-24*time.Hour), yesterdayCount, rng)

	today := p.assign(todayTimes, todayDeclines)
	p.markCancellations(today, cancellationsToday)
	plan := append(today, p.assign(yesterdayTimes, yesterdayDeclines)...)

	sort.SliceStable(plan, func(i, j int) bool { return plan[i].At.Before(plan[j].At) })
	return plan
}

// lastHourTimes spreads lastHourCount timestamps over the 24 chart buckets in lastHourShape's
// proportions (largest remainder), with at least one per bucket so no bar is empty.
func lastHourTimes(now time.Time, rng *rand.Rand) []time.Time {
	total := 0
	for _, v := range lastHourShape {
		total += v
	}
	counts := make([]int, bucketCount)
	remainders := make([]float64, bucketCount)
	assigned := 0
	for i, v := range lastHourShape {
		exact := float64(v*lastHourCount) / float64(total)
		counts[i] = max(1, int(exact))
		remainders[i] = exact - float64(int(exact))
		assigned += counts[i]
	}
	order := make([]int, bucketCount)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return remainders[order[a]] > remainders[order[b]] })
	for k := 0; assigned < lastHourCount; k = (k + 1) % bucketCount {
		counts[order[k]]++
		assigned++
	}

	var times []time.Time
	for i, c := range counts {
		bucketStart := now.Add(-time.Duration(bucketCount-i) * bucketWidth)
		for range c {
			// (bucketStart, bucketStart+width]: the same half-open bucket the Overview query uses.
			offset := time.Duration(rng.Int63n(int64(bucketWidth-time.Second))) + time.Second
			times = append(times, bucketStart.Add(offset))
		}
	}
	return times
}

// earlierTodayTimes places the rest of today's transactions between midnight UTC and the last
// hour, or inside the last hour when the day is younger than that.
func earlierTodayTimes(now, dayStart time.Time, rng *rand.Rand) []time.Time {
	end := now.Add(-time.Hour)
	if !end.After(dayStart) {
		return uniformTimes(now.Add(-time.Hour).Add(time.Second), now, earlierTodayCount, rng)
	}
	return uniformTimes(dayStart, end, earlierTodayCount, rng)
}

func uniformTimes(from, to time.Time, n int, rng *rand.Rand) []time.Time {
	span := to.Sub(from)
	times := make([]time.Time, n)
	for i := range times {
		if span <= 0 {
			times[i] = from
			continue
		}
		times[i] = from.Add(time.Duration(rng.Int63n(int64(span))))
	}
	return times
}

type planner struct {
	rng   *rand.Rand
	spent map[string]int64
}

// assign gives each timestamp an outcome (declines at random positions) and the card, terminal
// and amount that produce it.
func (p *planner) assign(times []time.Time, declines map[string]int) []Txn {
	outcomes := make([]string, len(times))
	for i := range outcomes {
		outcomes[i] = ExpectApproved
	}
	pos := p.rng.Perm(len(times))
	k := 0
	for _, rc := range []string{"51", "61", "62", "54"} {
		for range declines[rc] {
			outcomes[pos[k]] = rc
			k++
		}
	}

	txns := make([]Txn, len(times))
	for i, at := range times {
		txns[i] = p.txnFor(outcomes[i])
		txns[i].At = at
	}
	return txns
}

func (p *planner) txnFor(expect string) Txn {
	switch expect {
	case "51":
		m := merchants[1+p.rng.Intn(2)] // bookstore or supermarket
		return Txn{CardToken: "tok_low", TerminalID: m.terminalID, Amount: p.amount(150_000, 1_300_000), Expect: expect}
	case "61":
		m := merchants[2+p.rng.Intn(2)] // supermarket or fuel
		return Txn{CardToken: "tok_limit", TerminalID: m.terminalID, Amount: p.amount(LimitPerTransaction+20_000, 650_000), Expect: expect}
	case "62", "54":
		m := p.merchant()
		card := map[string]string{"62": "tok_blocked", "54": "tok_expired"}[expect]
		return Txn{CardToken: card, TerminalID: m.terminalID, Amount: p.amount(m.min, m.max), Expect: expect}
	}
	m := p.merchant()
	amount := p.amount(m.min, m.max)
	return Txn{CardToken: p.approvalCard(amount), TerminalID: m.terminalID, Amount: amount, Expect: ExpectApproved}
}

// approvalCard spreads approvals over the three cards that can approve, keeping the small-balance
// ones within their per-run budget; tok_second takes the rest.
func (p *planner) approvalCard(amount int64) string {
	switch {
	case amount <= 95_000 && p.spent["tok_normal"]+amount <= normalCardBudget && p.rng.Float64() < 0.5:
		p.spent["tok_normal"] += amount
		return "tok_normal"
	case amount <= LimitPerTransaction && p.spent["tok_limit"]+amount <= limitCardBudget && p.rng.Float64() < 0.25:
		p.spent["tok_limit"] += amount
		return "tok_limit"
	default:
		p.spent["tok_second"] += amount
		return "tok_second"
	}
}

// markCancellations flags n approvals for a customer cancellation (→ REVERSED), moving them onto
// tok_second so the reversal never depends on a small-balance card's budget.
func (p *planner) markCancellations(txns []Txn, n int) {
	for _, i := range p.rng.Perm(len(txns)) {
		if n == 0 {
			return
		}
		if txns[i].Expect != ExpectApproved {
			continue
		}
		if txns[i].CardToken != "tok_second" {
			p.spent[txns[i].CardToken] -= txns[i].Amount
			p.spent["tok_second"] += txns[i].Amount
			txns[i].CardToken = "tok_second"
		}
		txns[i].Cancel = true
		n--
	}
}

func (p *planner) merchant() merchantProfile {
	r := p.rng.Float64()
	for _, m := range merchants {
		if r < m.weight {
			return m
		}
		r -= m.weight
	}
	return merchants[0]
}

func (p *planner) amount(minAmount, maxAmount int64) int64 {
	return minAmount + p.rng.Int63n((maxAmount-minAmount)/amountStep+1)*amountStep
}
