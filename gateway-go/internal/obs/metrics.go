package obs

import "github.com/prometheus/client_golang/prometheus"

// LinkUp is 1 when the named link is SIGNED_ON, 0 otherwise (docs/02 §7.6: mcn_link_up{endpoint}).
var LinkUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "mcn_link_up", Help: "1 if the link is signed on, else 0"}, []string{"endpoint"})

func init() {
	prometheus.MustRegister(LinkUp)
}
