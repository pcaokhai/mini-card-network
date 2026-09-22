package obs

import "github.com/prometheus/client_golang/prometheus"

// LinkUp is 1 when the named link is SIGNED_ON, 0 otherwise (docs/02 §7.6: mcn_link_up{endpoint}).
var LinkUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "mcn_link_up", Help: "1 if the link is signed on, else 0"}, []string{"endpoint"})

// LateResponseTotal counts responses that arrived after the caller's request already timed out
// (docs/03 §5; MCN-203-AC3 — detection only, reversal/SAF queuing is a later story).
var LateResponseTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "mcn_late_response_total",
	Help: "Responses that arrived after the caller's request already timed out",
})

func init() {
	prometheus.MustRegister(LinkUp, LateResponseTotal)
}
